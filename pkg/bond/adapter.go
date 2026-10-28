package bond

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Adapter represents a network interface capable of routing traffic.
type Adapter struct {
	ID           string    `json:"id"`           // Unique identifier (e.g., interface name or MAC)
	Name         string    `json:"name"`         // System name (e.g., wlan0, eth0, Ethernet 2)
	DisplayName  string    `json:"display_name"` // Friendly label
	IP           net.IP    `json:"ip"`           // Bound local IPv4 address
	MAC          string    `json:"mac"`          // MAC address
	IsPhone      bool      `json:"is_phone"`     // Whether identified as USB tethering / mobile
	IsRemembered bool      `json:"is_remembered"`// Saved in profile
	Enabled      bool      `json:"enabled"`      // Whether enabled for bonding
	Connected    bool      `json:"connected"`    // Has verified internet route
	SpeedBps     int64     `json:"speed_bps"`    // Current measured download speed in bytes/sec
	LastSeen     time.Time `json:"last_seen"`
}

// PhoneKeywords are substrings commonly found in USB tethered network adapter names.
var PhoneKeywords = []string{
	"rndis",
	"remote ndis",
	"apple mobile",
	"usb ethernet",
	"cellular",
	"tether",
	"cdc_ncm",
	"cdc_ether",
	"usb0",
	"enp0s20f0u",
	"eth0u",
	"enx",
}

// IsPhoneInterface checks if an interface name or description matches known phone tethering keywords.
func IsPhoneInterface(name, mac string) bool {
	lowerName := strings.ToLower(name)
	for _, kw := range PhoneKeywords {
		if strings.Contains(lowerName, kw) {
			return true
		}
	}
	return false
}

// GetActiveAdapters scans system interfaces and returns all active non-loopback adapters with valid IPv4 addresses.
func GetActiveAdapters() ([]*Adapter, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list network interfaces: %w", err)
	}

	var adapters []*Adapter

	for _, iface := range ifaces {
		// Ignore loopback and down interfaces
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		var ipv4 net.IP
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}

			// Must be valid IPv4, not loopback, not link-local (169.254.x.x)
			if ip != nil && ip.To4() != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast() {
				ipv4 = ip.To4()
				break
			}
		}

		if ipv4 == nil {
			continue
		}

		mac := iface.HardwareAddr.String()
		isPhone := IsPhoneInterface(iface.Name, mac)

		displayName := iface.Name
		if isPhone {
			displayName = fmt.Sprintf("PHONE TETHER (%s)", iface.Name)
		} else if strings.HasPrefix(strings.ToLower(iface.Name), "wl") || strings.Contains(strings.ToLower(iface.Name), "wi-fi") {
			displayName = fmt.Sprintf("PC WI-FI (%s)", iface.Name)
		} else {
			displayName = fmt.Sprintf("NETWORK ADAPTER (%s)", iface.Name)
		}

		adapters = append(adapters, &Adapter{
			ID:          iface.Name,
			Name:        iface.Name,
			DisplayName: displayName,
			IP:          ipv4,
			MAC:         mac,
			IsPhone:     isPhone,
			Enabled:     true,
			Connected:   true,
			LastSeen:    time.Now(),
		})
	}

	return adapters, nil
}

// TestConnectivity checks if an adapter can reach the internet with a short timeout.
func (a *Adapter) TestConnectivity(testURL string, timeout time.Duration) bool {
	if testURL == "" {
		testURL = "http://www.google.com/generate_204"
	}

	client := NewBoundHTTPClient(a, timeout)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "HEAD", testURL, nil)
	if err != nil {
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		// Fallback to GET just in case HEAD is not supported by target
		req2, err2 := http.NewRequestWithContext(ctx, "GET", testURL, nil)
		if err2 != nil {
			return false
		}
		resp2, err3 := client.Do(req2)
		if err3 != nil {
			return false
		}
		resp2.Body.Close()
		return resp2.StatusCode >= 200 && resp2.StatusCode < 400
	}
	resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 400
}

// AdapterPool holds thread-safe collection of available adapters.
type AdapterPool struct {
	mu       sync.RWMutex
	adapters map[string]*Adapter
}

// NewAdapterPool creates a new empty pool.
func NewAdapterPool() *AdapterPool {
	return &AdapterPool{
		adapters: make(map[string]*Adapter),
	}
}

// Update sets or updates adapters in pool.
func (p *AdapterPool) Update(adapters []*Adapter) {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Update or add
	for _, a := range adapters {
		if existing, exists := p.adapters[a.ID]; exists {
			existing.IP = a.IP
			existing.LastSeen = time.Now()
			existing.Connected = a.Connected
		} else {
			p.adapters[a.ID] = a
		}
	}
}

// GetAll returns a copy of all current adapters.
func (p *AdapterPool) GetAll() []*Adapter {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var list []*Adapter
	for _, a := range p.adapters {
		// make shallow copy
		cp := *a
		list = append(list, &cp)
	}
	return list
}

// GetEnabled returns all active and enabled adapters.
func (p *AdapterPool) GetEnabled() []*Adapter {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var list []*Adapter
	for _, a := range p.adapters {
		if a.Enabled && a.Connected {
			cp := *a
			list = append(list, &cp)
		}
	}
	return list
}

// SetEnabled toggles an adapter's active state.
func (p *AdapterPool) SetEnabled(id string, enabled bool) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	if a, exists := p.adapters[id]; exists {
		a.Enabled = enabled
		return true
	}
	return false
}
