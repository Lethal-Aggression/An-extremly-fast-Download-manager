package bond

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// NewBoundHTTPClient creates an HTTP client strictly bound to the adapter's interface and IP.
func NewBoundHTTPClient(adapter *Adapter, timeout time.Duration) *http.Client {
	dialer := NewBoundDialer(adapter, timeout)

	transport := &http.Transport{
		Proxy:                 nil, // Direct connection, bypass system proxy to ensure interface binding
		DialContext:           dialer.DialContext,
		MaxIdleConns:          200,
		MaxIdleConnsPerHost:   100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     true,
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false,
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}
}

// DefaultDialerTimeout is the default connection timeout for worker sockets.
const DefaultDialerTimeout = 15 * time.Second

// NewBoundDialer returns a net.Dialer configured for the target OS and bound to the adapter.
func NewBoundDialer(adapter *Adapter, timeout time.Duration) *net.Dialer {
	if timeout <= 0 {
		timeout = DefaultDialerTimeout
	}

	dialer := &net.Dialer{
		Timeout:   timeout,
		KeepAlive: 30 * time.Second,
	}

	if adapter != nil && adapter.ProxyURL != "" {
		// ADB local proxy: connect over standard loopback tunnel
		return dialer
	}

	if adapter != nil && adapter.IP != nil {
		dialer.LocalAddr = &net.TCPAddr{
			IP:   adapter.IP,
			Port: 0,
		}
	}

	configureDialerForOS(dialer, adapter)
	return dialer
}
