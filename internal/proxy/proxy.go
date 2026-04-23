// Package proxy implements the HTTP proxy handler with caching functionality
package proxy

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"time"

	"github.com/snufkin23/caching-proxy-server/internal/cache"
)

// ProxyHandler handles incoming HTTP requests and proxies them through the cache
type ProxyHandler struct {
	cache  cache.Cache
	logger *slog.Logger
	client *http.Client
}

// NewProxyHandler creates a new proxy handler with SSRF protection
func NewProxyHandler(cache cache.Cache, logger *slog.Logger) *ProxyHandler {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
		if err != nil {
			return nil, err
		}
		for _, ip := range ips {
			if isPrivateIP(ip) {
				return nil, fmt.Errorf("access to private IP %s is blocked", ip)
			}
		}
		return (&net.Dialer{Timeout: 30 * time.Second}).DialContext(ctx, network, addr)
	}

	return &ProxyHandler{
		cache:  cache,
		logger: logger,
		client: &http.Client{
			Transport: transport,
			Timeout:   30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 10 {
					return fmt.Errorf("stopped after 10 redirects")
				}
				return nil
			},
		},
	}
}

// ServeHTTP is the main handler - called for every request
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only cache GET requests (POST/PUT/DELETE bypass cache)
	if r.Method != http.MethodGet {
		h.proxyRequestWithoutCache(w, r)
		return
	}

	// Step 1: Get the target URL
	targetURL := h.getTargetURL(r)
	if targetURL == "" {
		http.Error(w, "Missing target URL. Use ?url=<target> or X-Proxy-URL header", http.StatusBadRequest)
		return
	}

	// Step 2: Validate the URL
	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid URL: %v", err), http.StatusBadRequest)
		return
	}

	// Only allow HTTP/HTTPS
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		http.Error(w, "Only HTTP/HTTPS URLs are supported", http.StatusBadRequest)
		return
	}

	// Step 3: Generate cache key
	cacheKey := h.generateCacheKey(targetURL, r.Header)

	// Step 4: Check cache
	if entry, hit := h.cache.Get(r.Context(), cacheKey); hit {
		//  Cache HIT - return immediately!
		h.logger.InfoContext(r.Context(), "cache hit",
			slog.String("url", targetURL),
			slog.String("cache_key", cacheKey),
		)
		h.serveCachedResponse(w, entry)
		return
	}

	// Step 5: Cache MISS - fetch from internet
	h.logger.InfoContext(r.Context(), "cache miss",
		slog.String("url", targetURL),
		slog.String("cache_key", cacheKey),
	)

	entry, err := h.fetchUpstream(r.Context(), targetURL, r.Header)
	if err != nil {
		h.logger.ErrorContext(r.Context(), "upstream fetch failed",
			slog.String("url", targetURL),
			slog.String("error", err.Error()),
		)
		http.Error(w, fmt.Sprintf("Failed to fetch upstream: %v", err), http.StatusBadGateway)
		return
	}

	// Step 6: Cache successful responses (2xx status codes)
	if entry.StatusCode >= 200 && entry.StatusCode < 300 {
		if err := h.cache.Set(r.Context(), cacheKey, entry); err != nil {
			// Log error but don't fail the request
			h.logger.ErrorContext(r.Context(), "cache set failed",
				slog.String("error", err.Error()),
			)
		}
	}

	// Step 7: Serve response to user
	h.serveCachedResponse(w, entry)
}
