package bond

import (
	"context"
	"log"
	"sync"
	"time"
)

// MonitorListener is called when adapter state changes.
type MonitorListener interface {
	OnAdapterConnected(adapter *Adapter)
	OnAdapterDisconnected(adapter *Adapter)
}

// Monitor periodically watches for newly attached or detached network adapters (e.g., USB phone tethering).
type Monitor struct {
	pool      *AdapterPool
	listeners []MonitorListener
	interval  time.Duration
	stopCh    chan struct{}
	mu        sync.Mutex
	running   bool
	known     map[string]*Adapter
}

// NewMonitor creates an adapter monitor.
func NewMonitor(pool *AdapterPool, interval time.Duration) *Monitor {
	if interval <= 0 {
		interval = 1500 * time.Millisecond
	}
	return &Monitor{
		pool:     pool,
		interval: interval,
		stopCh:   make(chan struct{}),
		known:    make(map[string]*Adapter),
	}
}

// AddListener registers a listener for adapter connect/disconnect events.
func (m *Monitor) AddListener(l MonitorListener) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.listeners = append(m.listeners, l)
}

// Start begins the background monitoring loop.
func (m *Monitor) Start(ctx context.Context) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	m.running = true
	m.mu.Unlock()

	// Initial scan
	m.scanOnce()

	ticker := time.NewTicker(m.interval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-m.stopCh:
				return
			case <-ticker.C:
				m.scanOnce()
			}
		}
	}()
}

// Stop stops the monitor.
func (m *Monitor) Stop() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	close(m.stopCh)
	m.running = false
}

func (m *Monitor) scanOnce() {
	active, err := GetActiveAdapters()
	if err != nil {
		return
	}

	m.mu.Lock()
	currentMap := make(map[string]*Adapter)
	for _, a := range active {
		currentMap[a.ID] = a
	}

	// Detect new adapters
	var newlyAdded []*Adapter
	for id, a := range currentMap {
		if _, existed := m.known[id]; !existed {
			// Newly detected interface
			m.known[id] = a
			newlyAdded = append(newlyAdded, a)
		} else {
			// Update IP/state
			m.known[id].IP = a.IP
			m.known[id].LastSeen = time.Now()
		}
	}

	// Detect disconnected adapters
	var disconnected []*Adapter
	for id, a := range m.known {
		if _, exists := currentMap[id]; !exists {
			disconnected = append(disconnected, a)
			delete(m.known, id)
		}
	}

	listeners := make([]MonitorListener, len(m.listeners))
	copy(listeners, m.listeners)
	m.mu.Unlock()

	// Update pool
	m.pool.Update(active)

	// Fire notifications outside lock
	for _, a := range newlyAdded {
		if a.IsPhone {
			log.Printf("[+] [HOT-PLUG] Phone Tethering interface detected: %s (IP: %s, MAC: %s)", a.DisplayName, a.IP, a.MAC)
		} else {
			log.Printf("[+] [HOT-PLUG] Network interface attached: %s (IP: %s)", a.DisplayName, a.IP)
		}
		for _, l := range listeners {
			l.OnAdapterConnected(a)
		}
	}

	for _, a := range disconnected {
		if a.IsPhone {
			log.Printf("[-] [HOT-UNPLUG] Phone Tethering interface detached: %s", a.DisplayName)
		} else {
			log.Printf("[-] [HOT-UNPLUG] Network interface detached: %s", a.DisplayName)
		}
		for _, l := range listeners {
			l.OnAdapterDisconnected(a)
		}
	}
}
