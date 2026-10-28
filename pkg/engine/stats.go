package engine

import (
	"sync"
	"time"
)

type speedSample struct {
	time  time.Time
	bytes int64
}

// SpeedTracker tracks throughput for adapters and overall download using sliding window.
type SpeedTracker struct {
	mu            sync.RWMutex
	totalBytes    int64
	currentSpeed  int64 // bytes per second
	totalSamples  []speedSample
	adapterBytes  map[string]int64
	adapterSpeed  map[string]int64
	adapterSamples map[string][]speedSample
}

// NewSpeedTracker creates a new tracker.
func NewSpeedTracker() *SpeedTracker {
	return &SpeedTracker{
		adapterBytes:   make(map[string]int64),
		adapterSpeed:   make(map[string]int64),
		adapterSamples: make(map[string][]speedSample),
	}
}

// AddBytes registers bytes downloaded by a specific adapter.
func (s *SpeedTracker) AddBytes(adapterID string, n int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.totalBytes += n
	s.adapterBytes[adapterID] += n
}

// Tick recalculates smoothed rolling window speeds (call every ~200ms - 500ms).
func (s *SpeedTracker) Tick() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	// 1. Total speed window (rolling 1.5 seconds)
	s.totalSamples = append(s.totalSamples, speedSample{time: now, bytes: s.totalBytes})
	// Purge samples older than 1.5s
	cutoff := now.Add(-1500 * time.Millisecond)
	for len(s.totalSamples) > 2 && s.totalSamples[0].time.Before(cutoff) {
		s.totalSamples = s.totalSamples[1:]
	}

	if len(s.totalSamples) >= 2 {
		first := s.totalSamples[0]
		last := s.totalSamples[len(s.totalSamples)-1]
		elapsed := last.time.Sub(first.time).Seconds()
		if elapsed > 0.1 {
			diff := last.bytes - first.bytes
			s.currentSpeed = int64(float64(diff) / elapsed)
		}
	}

	// 2. Per-adapter rolling window
	for id, totalAdapterBytes := range s.adapterBytes {
		samples := append(s.adapterSamples[id], speedSample{time: now, bytes: totalAdapterBytes})
		for len(samples) > 2 && samples[0].time.Before(cutoff) {
			samples = samples[1:]
		}
		s.adapterSamples[id] = samples

		if len(samples) >= 2 {
			first := samples[0]
			last := samples[len(samples)-1]
			elapsed := last.time.Sub(first.time).Seconds()
			if elapsed > 0.1 {
				diff := last.bytes - first.bytes
				s.adapterSpeed[id] = int64(float64(diff) / elapsed)
			}
		}
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
