package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientIPTrustProxy(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")

	if got := clientIP(r, false); got != "10.0.0.1" {
		t.Errorf("trustProxy=false: got %q, want 10.0.0.1", got)
	}
	if got := clientIP(r, true); got != "1.2.3.4" {
		t.Errorf("trustProxy=true: got %q, want 1.2.3.4", got)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := RateLimitMiddleware(next, 2, time.Minute, false)

	req := httptest.NewRequest(http.MethodPost, "/run", nil)
	req.RemoteAddr = "192.0.2.1:9"

	for i := 0; i < 2; i++ {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Fatalf("req %d: status %d", i+1, rr.Code)
		}
	}
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", rr.Code)
	}
}

func TestSecureCompare(t *testing.T) {
	if !secureCompare("secret", "secret") {
		t.Error("equal keys should match")
	}
	if secureCompare("secret", "other") {
		t.Error("different keys should not match")
	}
	if secureCompare("short", "longerkey") {
		t.Error("different lengths should not match")
	}
}
