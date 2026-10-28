package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"

	"github.com/Lethal-Aggression/turbodl/pkg/bond"
	"github.com/Lethal-Aggression/turbodl/pkg/config"
	"github.com/Lethal-Aggression/turbodl/pkg/engine"
)

//go:embed static/*
var staticFS embed.FS

// Server hosts the web dashboard and REST API.
type Server struct {
	mu         sync.RWMutex
	cfg        *config.AppConfig
	pool       *bond.AdapterPool
	monitor    *bond.Monitor
	downloader *engine.Downloader
	port       int
}

// NewServer creates a web server.
func NewServer(cfg *config.AppConfig, pool *bond.AdapterPool, monitor *bond.Monitor) *Server {
	return &Server{
		cfg:     cfg,
		pool:    pool,
		monitor: monitor,
		port:    cfg.WebPort,
	}
}

// SetDownloader attaches an active downloader.
func (s *Server) SetDownloader(d *engine.Downloader) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.downloader = d
}

// Start launches the HTTP server.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Static frontend assets
	subFS, err := fs.Sub(staticFS, "static")
	if err != nil {
		return fmt.Errorf("failed to open embedded static assets: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(subFS)))

	// API endpoints
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/adapters", s.handleAdapters)
	mux.HandleFunc("/api/adapters/toggle", s.handleToggleAdapter)
	mux.HandleFunc("/api/adapters/remember", s.handleRememberAdapter)
	mux.HandleFunc("/api/download", s.handleStartDownload)
	mux.HandleFunc("/api/pause", s.handlePauseDownload)
	mux.HandleFunc("/api/resume", s.handleResumeDownload)

	addr := fmt.Sprintf("0.0.0.0:%d", s.port)
	log.Printf("[*] TurboDL Web Dashboard running at http://localhost:%d", s.port)
	return http.ListenAndServe(addr, mux)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	d := s.downloader
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if d == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status": "idle",
		})
		return
	}

	snapshot := d.Snapshot()
	json.NewEncoder(w).Encode(snapshot)
}

func (s *Server) handleAdapters(w http.ResponseWriter, r *http.Request) {
	adapters := s.pool.GetAll()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(adapters)
}

func (s *Server) handleToggleAdapter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	success := s.pool.SetEnabled(req.ID, req.Enabled)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": success,
	})
}

func (s *Server) handleRememberAdapter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		MAC   string `json:"mac"`
		Label string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.cfg.RememberPhone(req.MAC, req.Label, "")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
	})
}

func (s *Server) handleStartDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	d := engine.NewDownloader(req.URL, s.cfg.DownloadDir, req.Filename, s.cfg, s.pool, s.monitor)
	if err := d.Prepare(r.Context()); err != nil {
		http.Error(w, fmt.Sprintf("Failed to prepare download: %v", err), http.StatusInternalServerError)
		return
	}

	if err := d.Start(r.Context()); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start download: %v", err), http.StatusInternalServerError)
		return
	}

	s.SetDownloader(d)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":  true,
		"filename": d.Filename,
		"size":     d.TotalSize,
	})
}

func (s *Server) handlePauseDownload(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	d := s.downloader
	s.mu.RUnlock()

	if d != nil {
		d.Pause()
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}

func (s *Server) handleResumeDownload(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	d := s.downloader
	s.mu.RUnlock()

	if d != nil {
		_ = d.Start(r.Context())
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"success": true})
}
