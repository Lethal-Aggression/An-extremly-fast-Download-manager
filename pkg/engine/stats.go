package engine

import (
	"sync"
	"time"
)

// SpeedTracker tracks throughput for adapters and overall download.
type SpeedTracker struct {
	mu           sync.RWMutex
	lastTime     time.Time
	lastBytes    int64
	totalBytes   int64
	currentSpeed int64 // bytes per second (smoothed)
	adapterBytes map[string]int64
	adapterSpeed map[string]int64
}

// NewSpeedTracker creates a new tracker.
func NewSpeedTracker() *SpeedTracker {
	return &SpeedTracker{
		lastTime:     time.Now(),
		adapterBytes: make(map[string]int64),
		adapterSpeed: make(map[string]int64),
	}
}

// AddBytes registers bytes downloaded by a specific adapter.
func (s *SpeedTracker) AddBytes(adapterID string, n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalBytes += n
	s.adapterBytes[adapterID] += n
}

// Tick recalculates current speeds based on elapsed time (call ~once per 500ms - 1s).
func (s *SpeedTracker) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(s.lastTime).Seconds()
	if elapsed < 0.2 {
		return
	}

	// Overall speed
	diffBytes := s.totalBytes - s.lastBytes
	instantSpeed := int64(float64(diffBytes) / elapsed)

	// Smooth with EWMA (70% previous, 30% new)
	if s.currentSpeed == 0 {
		s.currentSpeed = instantSpeed
	} else {
		s.currentSpeed = int64(0.7*float64(s.currentSpeed) + 0.3*float64(instantSpeed))
	}

	s.lastBytes = s.totalBytes
	s.lastTime = now

	// Per-adapter speeds
	for id, bytes := range s.adapterBytes {
		// Calculate speed for adapter
		speed := int64(float64(bytes) / elapsed)
		if prev, ok := s.adapterSpeed[id]; ok && prev > 0 {
			s.adapterSpeed[id] = int64(0.7*float64(prev) + 0.3*float64(speed))
		} else {
			s.adapterSpeed[id] = speed
		}
		// Reset interval counter
		s.adapterBytes[id] = 0
	}
}

// GetOverallSpeed returns smoothed total download speed in bytes per second.
func (s *SpeedTracker) GetOverallSpeed() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.currentSpeed
}

// GetAdapterSpeed returns smoothed speed for a specific adapter.
func (s *SpeedTracker) GetAdapterSpeed(adapterID string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.adapterSpeed[adapterID]
}

// GetAllAdapterSpeeds returns a map of all adapter speeds.
func (s *SpeedTracker) GetAllAdapterSpeeds() map[string]int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make(map[string]int64, len(s.adapterSpeed))
	for k, v := range s.adapterSpeed {
		res[k] = v
	}
	return res
}

// TotalBytes returns total bytes downloaded so far.
func (s *SpeedTracker) TotalBytes() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.totalBytes
}
