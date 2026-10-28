package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// AppConfig represents user settings and remembered network devices.
type AppConfig struct {
	mu sync.RWMutex

	DownloadDir       string            `json:"download_dir"`
	ChunkSizeMB       int               `json:"chunk_size_mb"`
	ThreadsPerAdapter int               `json:"threads_per_adapter"`
	AutoBondAll       bool              `json:"auto_bond_all"`
	WebPort           int               `json:"web_port"`
	RememberedPhones  map[string]string `json:"remembered_phones"` // MAC or ID -> label (e.g. "LO's Phone")
}

var (
	defaultConfigPath = "turbodl_config.json"
)

// DefaultConfig returns safe and high-performance defaults.
func DefaultConfig() *AppConfig {
	home, err := os.UserHomeDir()
	dlDir := "./downloads"
	if err == nil {
		dlDir = filepath.Join(home, "Downloads")
	}

	return &AppConfig{
		DownloadDir:       dlDir,
		ChunkSizeMB:       8, // 8 MiB chunks balances memory overhead and concurrency
		ThreadsPerAdapter: 4, // 4 concurrent connections per adapter
		AutoBondAll:       true,
		WebPort:           8989,
		RememberedPhones:  make(map[string]string),
	}
}

// LoadConfig loads configuration from the default path or returns defaults.
func LoadConfig(path string) *AppConfig {
	if path == "" {
		path = defaultConfigPath
	}

	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		_ = cfg.Save(path)
		return cfg
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg
	}
	if cfg.RememberedPhones == nil {
		cfg.RememberedPhones = make(map[string]string)
	}
	if cfg.ThreadsPerAdapter <= 0 {
		cfg.ThreadsPerAdapter = 4
	}
	if cfg.ChunkSizeMB <= 0 {
		cfg.ChunkSizeMB = 8
	}
	return cfg
}

// Save writes configuration to disk.
func (c *AppConfig) Save(path string) error {
	if path == "" {
		path = defaultConfigPath
	}

	c.mu.RLock()
	defer c.mu.RUnlock()

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// RememberPhone saves a phone MAC or ID so it is remembered forever.
func (c *AppConfig) RememberPhone(id, label string, path string) {
	c.mu.Lock()
	if c.RememberedPhones == nil {
		c.RememberedPhones = make(map[string]string)
	}
	c.RememberedPhones[id] = label
	c.mu.Unlock()

	_ = c.Save(path)
}

// IsPhoneRemembered returns true if the device identifier is in the remembered list.
func (c *AppConfig) IsPhoneRemembered(id string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	_, exists := c.RememberedPhones[id]
	return exists
}
