package main

import (
	"io"
	"net/http"
	"net/url"
	"time"
)

func main() {
	transport := &http.Transport{
		MaxIdleConns:        200,
		MaxIdleConnsPerHost: 50,
		IdleConnTimeout:     90 * time.Second,
		DisableKeepAlives:   false,
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   60 * time.Second,
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		rawTarget := r.URL.Query().Get("url")
		if rawTarget == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		targetURL, err := url.QueryUnescape(rawTarget)
		if err != nil {
			targetURL = rawTarget
		}

		req, err := http.NewRequest(r.Method, targetURL, nil)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		if rng := r.Header.Get("Range"); rng != "" {
			req.Header.Set("Range", rng)
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 14) TurboDL/1.0")

		resp, err := client.Do(req)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		for k, v := range resp.Header {
			w.Header()[k] = v
		}
		w.WriteHeader(resp.StatusCode)

		flusher, canFlush := w.(http.Flusher)
		buf := make([]byte, 64*1024)

		for {
			n, rErr := resp.Body.Read(buf)
			if n > 0 {
				_, _ = w.Write(buf[:n])
				if canFlush {
					flusher.Flush()
				}
			}
			if rErr != nil {
				if rErr == io.EOF {
					break
				}
				return
			}
		}
	})

	_ = http.ListenAndServe("127.0.0.1:18989", nil)
}
