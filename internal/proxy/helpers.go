// Package proxy helpers contains the main proxy handler and related utilities
package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/snufkin23/caching-proxy-server/internal/cache"
)

// getTargetURL extracts the target URL from the request
func (h *ProxyHandler) getTargetURL(r *http.Request) string {
	// Method 1: Query parameter
	if url := r.URL.Query().Get("url"); url != "" {
		return url
	}

	// Method 2: Custom header
	if url := r.Header.Get("X-Proxy-URL"); url != "" {
		return url
	}

	// Method 3: Path-based routing
	path := strings.TrimPrefix(r.URL.Path, "/")
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}

	return ""
}

// generateCacheKey creates a unique key for the cache
func (h *ProxyHandler) generateCacheKey(targetURL string, headers http.Header) string {
	// Build the key components
	key := targetURL

	// Include headers that affect the response
	if accept := headers.Get("Accept"); accept != "" {
		key += "|Accept:" + accept
	}
	if acceptLang := headers.Get("Accept-Language"); acceptLang != "" {
		key += "|Accept-Language:" + acceptLang
	}
	if acceptEnc := headers.Get("Accept-Encoding"); acceptEnc != "" {
		key += "|Accept-Encoding:" + acceptEnc
	}

	// Hash it (SHA256) to keep it a reasonable length
	hash := sha256.Sum256([]byte(key))
	return hex.EncodeToString(hash[:])
}

// fetchUpstream fetches the actual data from the internet
func (h *ProxyHandler) fetchUpstream(ctx context.Context, targetURL string, requestHeaders http.Header) (*cache.Entry, error) {
	// Step 1: Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	// Step 2: Forward relevant headers
	h.forwardHeaders(req, requestHeaders)

	// Step 3: Execute request
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	// Step 4: Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// Step 5: Build cache entry
	entry := &cache.Entry{
		StatusCode:  resp.StatusCode,
		Headers:     make(map[string][]string),
		Body:        body,
		CachedAt:    time.Now(),
		ContentType: resp.Header.Get("Content-Type"),
	}

	// Copy response headers
	for key, values := range resp.Header {
		entry.Headers[key] = values
	}

	return entry, nil
}

// forwardHeaders copies important headers from the original request
func (h *ProxyHandler) forwardHeaders(req *http.Request, headers http.Header) {
	forwardableHeaders := []string{
		"Accept",
		"Accept-Language",
		"Accept-Encoding",
		"User-Agent",
		"Referer",
	}

	for _, header := range forwardableHeaders {
		if value := headers.Get(header); value != "" {
			req.Header.Set(header, value)
		}
	}
}

// serveCachedResponse sends a cached entry to the user
func (h *ProxyHandler) serveCachedResponse(w http.ResponseWriter, entry *cache.Entry) {
	// Step 1: Set all original headers
	for key, values := range entry.Headers {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Step 2: Add cache metadata headers
	w.Header().Set("X-Cache-Status", "HIT")
	w.Header().Set("X-Cache-Date", entry.CachedAt.Format(time.RFC1123))
	w.Header().Set("X-Cache-Expires", entry.ExpiresAt.Format(time.RFC1123))

	// Step 3: Write status code
	w.WriteHeader(entry.StatusCode)

	// Step 4: Write body
	if _, err := w.Write(entry.Body); err != nil {
		h.logger.Error("write response failed", slog.String("error", err.Error()))
	}
}

// proxyRequestWithoutCache handles non-GET requests (POST, PUT, DELETE, etc.)
func (h *ProxyHandler) proxyRequestWithoutCache(w http.ResponseWriter, r *http.Request) {
	targetURL := h.getTargetURL(r)
	if targetURL == "" {
		http.Error(w, "Missing target URL", http.StatusBadRequest)
		return
	}

	// Create upstream request with the same method and body
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create request: %v", err), http.StatusInternalServerError)
		return
	}

	// Copy all headers
	upstreamReq.Header = r.Header.Clone()

	// Execute request
	resp, err := h.client.Do(upstreamReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Upstream request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Mark as bypassed cache
	w.Header().Set("X-Cache-Status", "BYPASS")
	w.WriteHeader(resp.StatusCode)

	// Stream response body
	if _, err := io.Copy(w, resp.Body); err != nil {
		h.logger.Error("copy response failed", slog.String("error", err.Error()))
	}
}
