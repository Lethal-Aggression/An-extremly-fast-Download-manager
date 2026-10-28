package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Storage manages the destination file on disk and chunk state persistence.
type Storage struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	metaPath string
	total    int64
}

// MetaState represents the on-disk checkpoint of a download.
type MetaState struct {
	URL         string          `json:"url"`
	Filename    string          `json:"filename"`
	TotalSize   int64           `json:"total_size"`
	ChunkSize   int64           `json:"chunk_size"`
	Chunks      []*ChunkMeta    `json:"chunks"`
}

// ChunkMeta tracks individual chunk status.
type ChunkMeta struct {
	Index      int   `json:"index"`
	Start      int64 `json:"start"`
	End        int64 `json:"end"`
	Downloaded int64 `json:"downloaded"`
	Done       bool  `json:"done"`
}

// NewStorage initializes or resumes a target file on disk.
func NewStorage(destPath string, totalSize int64) (*Storage, error) {
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	metaPath := destPath + ".turbodl.meta"

	// Open or create file for reading and writing
	file, err := os.OpenFile(destPath, os.O_CREATE|os.O_RDWR, 0666)
	if err != nil {
		return nil, fmt.Errorf("failed to open output file: %w", err)
	}

	// Pre-allocate full size if known to avoid disk fragmentation
	if totalSize > 0 {
		info, err := file.Stat()
		if err == nil && info.Size() < totalSize {
			_ = file.Truncate(totalSize)
		}
	}

	return &Storage{
		file:     file,
		path:     destPath,
		metaPath: metaPath,
		total:    totalSize,
	}, nil
}

// WriteAt writes a buffer directly to the specified byte offset.
func (s *Storage) WriteAt(p []byte, offset int64) (int, error) {
	// os.File.WriteAt uses pwrite on Linux and OVERLAPPED WriteFile on Windows,
	// allowing lock-free concurrent writes to non-overlapping regions.
	return s.file.WriteAt(p, offset)
}

// SaveMeta persists chunk states to disk for resume support.
func (s *Storage) SaveMeta(meta *MetaState) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(s.metaPath, data, 0644)
}

// LoadMeta reads checkpoint data if available.
func (s *Storage) LoadMeta() (*MetaState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := os.ReadFile(s.metaPath)
	if err != nil {
		return nil, err
	}

	var meta MetaState
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

// Close closes the file handle.
func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}

// RemoveMeta cleans up the checkpoint metadata when download finishes cleanly.
func (s *Storage) RemoveMeta() {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = os.Remove(s.metaPath)
}
