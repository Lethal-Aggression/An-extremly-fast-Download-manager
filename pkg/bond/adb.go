package bond

import (
	_ "embed"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

//go:embed phone_node_arm64
var phoneNodeBinary []byte

const ADBProxyPort = 18989

var (
	adbMu         sync.Mutex
	cachedAdapter *Adapter
	cachedSerial  string
	lastChecked   time.Time
)

// findADB searches for adb in common paths.
func findADB() string {
	paths := []string{
		"adb",
		"/home/d4cx/.local/bin/adb",
		"/usr/bin/adb",
		"/usr/local/bin/adb",
	}
	for _, p := range paths {
		if path, err := exec.LookPath(p); err == nil {
			return path
		}
	}
	return ""
}

// CheckADBDevices checks for connected Android devices via ADB and sets up the direct high-speed pipe.
func CheckADBDevices() (*Adapter, error) {
	adbMu.Lock()
	defer adbMu.Unlock()

	// If we have a healthy cached adapter and checked recently, verify quickly
	if cachedAdapter != nil && time.Since(lastChecked) < 3*time.Second {
		return cachedAdapter, nil
	}

	// Quick health check on existing proxy
	testClient := &http.Client{Timeout: 300 * time.Millisecond}
	resp, err := testClient.Get(fmt.Sprintf("http://127.0.0.1:%d/health", ADBProxyPort))
	if err == nil && resp.StatusCode == http.StatusOK {
		resp.Body.Close()
		lastChecked = time.Now()
		if cachedAdapter != nil {
			return cachedAdapter, nil
		}
	}

	adbPath := findADB()
	if adbPath == "" {
		return nil, fmt.Errorf("adb not found")
	}

	cmd := exec.Command(adbPath, "devices")
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(out), "\n")
	var serial string
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[1] == "device" {
			serial = fields[0]
			break
		}
	}

	if serial == "" {
		cachedAdapter = nil
		return nil, fmt.Errorf("no authorized adb device attached")
	}

	// If serial matches and proxy is already up, keep using it without re-forwarding
	if cachedSerial == serial && cachedAdapter != nil {
		lastChecked = time.Now()
		return cachedAdapter, nil
	}

	// Get device model
	modelCmd := exec.Command(adbPath, "-s", serial, "shell", "getprop", "ro.product.model")
	modelOut, _ := modelCmd.Output()
	model := strings.TrimSpace(string(modelOut))
	if model == "" {
		model = "Android Phone"
	}

	// Extract embedded binary and push only once
	tmpPath := filepath.Join(os.TempDir(), "turbodl_node")
	_ = os.WriteFile(tmpPath, phoneNodeBinary, 0755)

	_ = exec.Command(adbPath, "-s", serial, "push", tmpPath, "/data/local/tmp/phone_node").Run()
	_ = exec.Command(adbPath, "-s", serial, "forward", fmt.Sprintf("tcp:%d", ADBProxyPort), fmt.Sprintf("tcp:%d", ADBProxyPort)).Run()
	_ = exec.Command(adbPath, "-s", serial, "shell", "nohup /data/local/tmp/phone_node >/dev/null 2>&1 &").Start()
	time.Sleep(300 * time.Millisecond)

	cachedSerial = serial
	cachedAdapter = &Adapter{
		ID:          "adb_" + serial,
		Name:        fmt.Sprintf("adb-%s", serial),
		DisplayName: fmt.Sprintf("[PHONE] TETHER (USB-ADB) [%s]", model),
		IP:          net.ParseIP("127.0.0.1"),
		MAC:         serial,
		IsPhone:     true,
		Enabled:     true,
		Connected:   true,
		ProxyURL:    fmt.Sprintf("http://127.0.0.1:%d/?url=", ADBProxyPort),
		LastSeen:    time.Now(),
	}
	lastChecked = time.Now()

	return cachedAdapter, nil
}
