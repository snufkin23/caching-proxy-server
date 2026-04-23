package proxy

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/snufkin23/caching-proxy-server/internal/cache"
)

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip       string
		expected bool
	}{
		{"8.8.8.8", false},
		{"127.0.0.1", true},
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"::1", true},
	}

	for _, tc := range tests {
		ip := net.ParseIP(tc.ip)
		if result := isPrivateIP(ip); result != tc.expected {
			t.Errorf("isPrivateIP(%s) = %v; want %v", tc.ip, result, tc.expected)
		}
	}
}

func TestSecurityHeaders(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := cache.NewMemoryCache(0, 0)
	h := NewProxyHandler(c, logger)

	rr := httptest.NewRecorder()
	h.copySafeResponseHeaders(rr, http.Header{})

	if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Error("X-Content-Type-Options should be nosniff")
	}
	if rr.Header().Get("X-Frame-Options") != "SAMEORIGIN" {
		t.Error("X-Frame-Options should be SAMEORIGIN")
	}
}

func TestHeaderFiltering(t *testing.T) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	c := cache.NewMemoryCache(0, 0)
	h := NewProxyHandler(c, logger)

	upstreamHeaders := http.Header{
		"Content-Type":                 {"text/plain"},
		"Content-Encoding":             {"gzip"},
		"Location":                     {"http://example.com"},
		"Set-Cookie":                   {"session=secret"},
		"Access-Control-Allow-Origin": {"*"},
	}

	rr := httptest.NewRecorder()
	h.copySafeResponseHeaders(rr, upstreamHeaders)

	if rr.Header().Get("Content-Type") != "text/plain" {
		t.Error("Content-Type should be forwarded")
	}
	if rr.Header().Get("Content-Encoding") != "gzip" {
		t.Error("Content-Encoding should be forwarded")
	}
	if rr.Header().Get("Location") != "http://example.com" {
		t.Error("Location should be forwarded")
	}
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS headers should be forwarded")
	}
	if rr.Header().Get("Set-Cookie") != "" {
		t.Error("Set-Cookie should NOT be forwarded")
	}
}
