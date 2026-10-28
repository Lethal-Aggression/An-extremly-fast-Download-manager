package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Lethal-Aggression/turbodl/pkg/bond"
	"github.com/Lethal-Aggression/turbodl/pkg/config"
)

// DownloadStatus represents state of the overall download.
type DownloadStatus string

const (
	StatusIdle        DownloadStatus = "idle"
	StatusConnecting  DownloadStatus = "connecting"
	StatusDownloading DownloadStatus = "downloading"
	StatusPaused      DownloadStatus = "paused"
	StatusCompleted   DownloadStatus = "completed"
	StatusFailed      DownloadStatus = "failed"
)

// Downloader manages the concurrent multi-adapter download session.
type Downloader struct {
	mu           sync.RWMutex
	cfg          *config.AppConfig
	pool         *bond.AdapterPool
	monitor      *bond.Monitor
	storage      *Storage
	stats        *SpeedTracker

	URL          string
	Filename     string
	DestPath     string
	TotalSize    int64
	AcceptRanges bool
	Status       DownloadStatus
	ErrorMessage string

	chunks       []*Chunk
	workQueue    chan *Chunk
	retryChan    chan *Chunk

	workerGroups map[string][]*Worker // adapterID -> workers
	workersWg    sync.WaitGroup
	ctx          context.Context
	cancel       context.CancelFunc
	running      bool
}

// NewDownloader creates a multi-interface bonded downloader instance.
func NewDownloader(rawURL, destDir, filename string, cfg *config.AppConfig, pool *bond.AdapterPool, monitor *bond.Monitor) *Downloader {
	d := &Downloader{
		cfg:          cfg,
		pool:         pool,
		monitor:      monitor,
		stats:        NewSpeedTracker(),
		URL:          rawURL,
		Filename:     filename,
		DestPath:     destDir,
		Status:       StatusIdle,
		workerGroups: make(map[string][]*Worker),
	}

	if monitor != nil {
		monitor.AddListener(d)
	}

	return d
}

// Prepare probes the URL to get Content-Length, filename, and Range support.
func (d *Downloader) Prepare(ctx context.Context) error {
	d.mu.Lock()
	d.Status = StatusConnecting
	d.mu.Unlock()

	req, err := http.NewRequestWithContext(ctx, "HEAD", d.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0 Safari/537.36")

	// Use default HTTP client for initial HEAD request
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode >= 400 {
		// Fallback to GET with Range: bytes=0-0 if HEAD is disallowed
		req2, err2 := http.NewRequestWithContext(ctx, "GET", d.URL, nil)
		if err2 != nil {
			return err
		}
		req2.Header.Set("Range", "bytes=0-0")
		req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) Chrome/120.0.0.0 Safari/537.36")
		resp2, err3 := client.Do(req2)
		if err3 != nil {
			return fmt.Errorf("failed to reach server: %w", err3)
		}
		resp = resp2
	}
	defer resp.Body.Close()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Parse filename if not provided
	if d.Filename == "" {
		d.Filename = extractFilename(resp, d.URL)
	}
	d.DestPath = filepath.Join(d.DestPath, d.Filename)

	// Check Range support
	if resp.Header.Get("Accept-Ranges") == "bytes" || resp.StatusCode == http.StatusPartialContent {
		d.AcceptRanges = true
	}

	// Content-Length
	if resp.ContentLength > 0 {
		d.TotalSize = resp.ContentLength
	} else if cr := resp.Header.Get("Content-Range"); cr != "" {
		// e.g. bytes 0-0/104857600
		parts := strings.Split(cr, "/")
		if len(parts) == 2 {
			var total int64
			_, _ = fmt.Sscanf(parts[1], "%d", &total)
			if total > 0 {
				d.TotalSize = total
				d.AcceptRanges = true
			}
		}
	}

	return nil
}

// Start begins the bonded download.
func (d *Downloader) Start(parentCtx context.Context) error {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return errors.New("download already running")
	}

	d.ctx, d.cancel = context.WithCancel(parentCtx)
	d.running = true
	d.Status = StatusDownloading

	// Initialize storage
	st, err := NewStorage(d.DestPath, d.TotalSize)
	if err != nil {
		d.Status = StatusFailed
		d.ErrorMessage = err.Error()
		d.mu.Unlock()
		return err
	}
	d.storage = st

	// Partition chunks
	chunkSize := int64(d.cfg.ChunkSizeMB) * 1024 * 1024
	if chunkSize <= 0 {
		chunkSize = 8 * 1024 * 1024
	}

	if d.TotalSize > 0 && d.AcceptRanges {
		numChunks := int((d.TotalSize + chunkSize - 1) / chunkSize)
		d.chunks = make([]*Chunk, numChunks)
		for i := 0; i < numChunks; i++ {
			start := int64(i) * chunkSize
			end := start + chunkSize - 1
			if end >= d.TotalSize {
				end = d.TotalSize - 1
			}
			d.chunks[i] = &Chunk{
				Index:      i,
				Start:      start,
				End:        end,
				Downloaded: 0,
				Status:     ChunkPending,
			}
		}
	} else {
		// Single chunk if ranges not supported
		d.chunks = []*Chunk{{
			Index:      0,
			Start:      0,
			End:        d.TotalSize - 1,
			Downloaded: 0,
			Status:     ChunkPending,
		}}
	}

	d.workQueue = make(chan *Chunk, len(d.chunks)+10)
	d.retryChan = make(chan *Chunk, len(d.chunks)+10)

	// Enqueue all chunks
	for _, c := range d.chunks {
		d.workQueue <- c
	}

	d.mu.Unlock()

	// Start queue coordinator
	go d.queueCoordinator()

	// Start speed ticker
	go d.speedTickerLoop()

	// Spawn initial workers on all enabled adapters
	adapters := d.pool.GetEnabled()
	if len(adapters) == 0 {
		// Fallback to active system adapters
		var err error
		adapters, err = bond.GetActiveAdapters()
		if err != nil || len(adapters) == 0 {
			return errors.New("no active network interfaces found")
		}
	}

	log.Printf("[*] Starting download '%s' across %d network interfaces...", d.Filename, len(adapters))
	for _, a := range adapters {
		d.spawnWorkersForAdapter(a)
	}

	return nil
}

func (d *Downloader) queueCoordinator() {
	for {
		select {
		case <-d.ctx.Done():
			return
		case chunk := <-d.retryChan:
			// Put back into workQueue for any worker to steal
			select {
			case d.workQueue <- chunk:
			case <-d.ctx.Done():
				return
			}
		}
	}
}

func (d *Downloader) speedTickerLoop() {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.stats.Tick()
			d.checkCompletion()
		}
	}
}

func (d *Downloader) checkCompletion() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.Status != StatusDownloading {
		return
	}

	doneCount := 0
	var downloadedBytes int64

	for _, c := range d.chunks {
		c.mu.Lock()
		if c.Status == ChunkDone {
			doneCount++
		}
		downloadedBytes += c.Downloaded
		c.mu.Unlock()
	}

	if doneCount == len(d.chunks) && len(d.chunks) > 0 {
		d.Status = StatusCompleted
		d.running = false
		if d.storage != nil {
			d.storage.RemoveMeta()
			_ = d.storage.Close()
		}
		log.Printf("[DONE] Download '%s' completed successfully! [Total Size: %d bytes]", d.Filename, d.TotalSize)
		if d.cancel != nil {
			d.cancel()
		}
	}
}

func (d *Downloader) spawnWorkersForAdapter(adapter *bond.Adapter) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running || d.ctx.Err() != nil {
		return
	}

	// Don't spawn duplicates
	if _, exists := d.workerGroups[adapter.ID]; exists {
		return
	}

	threads := d.cfg.ThreadsPerAdapter
	if threads <= 0 {
		threads = 4
	}

	log.Printf("[+] Bonding Adapter '%s' (%s) -> Spawning %d worker threads", adapter.DisplayName, adapter.IP, threads)

	var workers []*Worker
	for i := 0; i < threads; i++ {
		w := NewWorker(i, adapter, d.URL, d.storage, d.stats, d.workQueue, d.retryChan, d.ctx)
		workers = append(workers, w)
		d.workersWg.Add(1)
		go w.Start(&d.workersWg)
	}

	d.workerGroups[adapter.ID] = workers
}

func (d *Downloader) stopWorkersForAdapter(adapterID string) {
	d.mu.Lock()
	defer d.mu.Unlock()

	workers, exists := d.workerGroups[adapterID]
	if !exists {
		return
	}

	log.Printf("[-] Unbonding Adapter '%s' -> Cancelling %d worker threads", adapterID, len(workers))
	for _, w := range workers {
		w.Stop()
	}
	delete(d.workerGroups, adapterID)
}

// OnAdapterConnected implements bond.MonitorListener (Hot-Plug)
func (d *Downloader) OnAdapterConnected(adapter *bond.Adapter) {
	d.mu.RLock()
	running := d.running
	d.mu.RUnlock()

	if !running {
		return
	}

	// Hot-plug auto-bonding: If phone or auto-bond is enabled, add it right now!
	if d.cfg.AutoBondAll || adapter.IsPhone || d.cfg.IsPhoneRemembered(adapter.MAC) {
		log.Printf("[+] [HOT-PLUG ACTIVE] Instantly joining adapter '%s' to active download pool!", adapter.DisplayName)
		d.spawnWorkersForAdapter(adapter)
	}
}

// OnAdapterDisconnected implements bond.MonitorListener (Hot-Unplug)
func (d *Downloader) OnAdapterDisconnected(adapter *AdapterDisconnectedEvent) {
	d.stopWorkersForAdapter(adapter.ID)
}

// AdapterDisconnectedEvent adapter helper
type AdapterDisconnectedEvent = bond.Adapter

// Pause pauses the download.
func (d *Downloader) Pause() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if !d.running {
		return
	}

	d.Status = StatusPaused
	d.running = false
	if d.cancel != nil {
		d.cancel()
	}
	if d.storage != nil {
		_ = d.storage.Close()
	}
}

// GetSnapshot returns safe copy of current progress for UI.
type ProgressSnapshot struct {
	URL           string           `json:"url"`
	Filename      string           `json:"filename"`
	DestPath      string           `json:"dest_path"`
	TotalSize     int64            `json:"total_size"`
	Downloaded    int64            `json:"downloaded"`
	Percentage    float64          `json:"percentage"`
	TotalSpeedBps int64            `json:"total_speed_bps"`
	ETASeconds    int64            `json:"eta_seconds"`
	Status        DownloadStatus   `json:"status"`
	AdapterSpeeds map[string]int64 `json:"adapter_speeds"`
	ActiveAdapters []string        `json:"active_adapters"`
	TotalChunks   int              `json:"total_chunks"`
	DoneChunks    int              `json:"done_chunks"`
}

// Snapshot calculates real-time progress.
func (d *Downloader) Snapshot() ProgressSnapshot {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var downloaded int64
	doneChunks := 0
	for _, c := range d.chunks {
		c.mu.Lock()
		downloaded += c.Downloaded
		if c.Status == ChunkDone {
			doneChunks++
		}
		c.mu.Unlock()
	}

	pct := 0.0
	if d.TotalSize > 0 {
		pct = (float64(downloaded) / float64(d.TotalSize)) * 100.0
	}

	speed := d.stats.GetOverallSpeed()
	var eta int64
	if speed > 0 && d.TotalSize > downloaded {
		eta = (d.TotalSize - downloaded) / speed
	}

	var activeAdapters []string
	for id := range d.workerGroups {
		activeAdapters = append(activeAdapters, id)
	}

	return ProgressSnapshot{
		URL:           d.URL,
		Filename:      d.Filename,
		DestPath:      d.DestPath,
		TotalSize:     d.TotalSize,
		Downloaded:    downloaded,
		Percentage:    pct,
		TotalSpeedBps: speed,
		ETASeconds:    eta,
		Status:        d.Status,
		AdapterSpeeds: d.stats.GetAllAdapterSpeeds(),
		ActiveAdapters: activeAdapters,
		TotalChunks:   len(d.chunks),
		DoneChunks:    doneChunks,
	}
}

// Helper to extract clean filename
func extractFilename(resp *http.Response, rawURL string) string {
	cd := resp.Header.Get("Content-Disposition")
	if cd != "" {
		if _, params, err := mime.ParseMediaType(cd); err == nil {
			if fn, ok := params["filename"]; ok && fn != "" {
				return fn
			}
			if fn, ok := params["filename*"]; ok && fn != "" {
				return fn
			}
		}
	}

	u, err := url.Parse(rawURL)
	if err == nil {
		base := path.Base(u.Path)
		if base != "" && base != "/" && base != "." {
			return base
		}
	}

	return "downloaded_file.bin"
}
