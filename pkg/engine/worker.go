package engine

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/Lethal-Aggression/turbodl/pkg/bond"
)

// ChunkStatus represents current state of a file block.
type ChunkStatus string

const (
	ChunkPending     ChunkStatus = "pending"
	ChunkDownloading ChunkStatus = "downloading"
	ChunkDone        ChunkStatus = "done"
)

// Chunk represents a contiguous byte range of the target file.
type Chunk struct {
	mu         sync.Mutex
	Index      int         `json:"index"`
	Start      int64       `json:"start"`
	End        int64       `json:"end"`
	Downloaded int64       `json:"downloaded"`
	Status     ChunkStatus `json:"status"`
	AdapterID  string      `json:"adapter_id"`
}

// Length returns total bytes in chunk.
func (c *Chunk) Length() int64 {
	return c.End - c.Start + 1
}

// Remaining returns unread bytes in chunk.
func (c *Chunk) Remaining() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Length() - c.Downloaded
}

// MarkProgress updates downloaded counter.
func (c *Chunk) MarkProgress(n int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Downloaded += n
}

// Worker downloads chunks over an adapter-bound socket.
type Worker struct {
	ID        int
	Adapter   *bond.Adapter
	Client    *http.Client
	Storage   *Storage
	Stats     *SpeedTracker
	URL       string
	workQueue chan *Chunk
	retryChan chan *Chunk
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewWorker initializes a worker bound to an adapter.
func NewWorker(id int, adapter *bond.Adapter, rawURL string, storage *Storage, stats *SpeedTracker, workQueue, retryChan chan *Chunk, parentCtx context.Context) *Worker {
	ctx, cancel := context.WithCancel(parentCtx)
	client := bond.NewBoundHTTPClient(adapter, 30*time.Second)

	return &Worker{
		ID:        id,
		Adapter:   adapter,
		Client:    client,
		Storage:   storage,
		Stats:     stats,
		URL:       rawURL,
		workQueue: workQueue,
		retryChan: retryChan,
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start begins processing chunks from the queue until cancelled.
func (w *Worker) Start(wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			return
		case chunk, ok := <-w.workQueue:
			if !ok {
				return
			}

			chunk.mu.Lock()
			if chunk.Status == ChunkDone {
				chunk.mu.Unlock()
				continue
			}
			chunk.Status = ChunkDownloading
			chunk.AdapterID = w.Adapter.ID
			chunk.mu.Unlock()

			err := w.downloadChunk(chunk)
			if err != nil {
				// Requeue incomplete chunk for other workers
				chunk.mu.Lock()
				if chunk.Downloaded < chunk.Length() {
					chunk.Status = ChunkPending
					chunk.AdapterID = ""
					chunk.mu.Unlock()
					select {
					case w.retryChan <- chunk:
					case <-w.ctx.Done():
						return
					}
				} else {
					chunk.Status = ChunkDone
					chunk.mu.Unlock()
				}
			} else {
				chunk.mu.Lock()
				chunk.Status = ChunkDone
				chunk.mu.Unlock()
			}
		}
	}
}

func (w *Worker) downloadChunk(c *Chunk) error {
	c.mu.Lock()
	currentStart := c.Start + c.Downloaded
	targetEnd := c.End
	c.mu.Unlock()

	if currentStart > targetEnd {
		return nil
	}

	req, err := http.NewRequestWithContext(w.ctx, "GET", w.URL, nil)
	if err != nil {
		return err
	}

	rangeHeader := fmt.Sprintf("bytes=%d-%d", currentStart, targetEnd)
	req.Header.Set("Range", rangeHeader)
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := w.Client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status code: %d", resp.StatusCode)
	}

	// 64 KB streaming buffer for high throughput
	buf := make([]byte, 64*1024)
	writeOffset := currentStart

	for {
		select {
		case <-w.ctx.Done():
			return w.ctx.Err()
		default:
		}

		n, rErr := resp.Body.Read(buf)
		if n > 0 {
			_, wErr := w.Storage.WriteAt(buf[:n], writeOffset)
			if wErr != nil {
				return wErr
			}
			writeOffset += int64(n)
			c.MarkProgress(int64(n))
			w.Stats.AddBytes(w.Adapter.ID, int64(n))
		}

		if rErr != nil {
			if rErr == io.EOF {
				break
			}
			return rErr
		}
	}

	return nil
}

// Stop halts the worker gracefully.
func (w *Worker) Stop() {
	w.cancel()
}
