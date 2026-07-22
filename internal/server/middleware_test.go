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

func TestAPIKeyMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := APIKeyMiddleware(next, "sekrit", "/", "/health")

	t.Run("excluded path bypasses auth", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("excluded path: status %d", rr.Code)
		}
	})

	t.Run("missing Authorization header rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/run", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("missing header: status %d, want 401", rr.Code)
		}
	})

	t.Run("malformed Authorization header rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/run", nil)
		req.Header.Set("Authorization", "sekrit")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("malformed header: status %d, want 401", rr.Code)
		}
	})

	t.Run("wrong key rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/run", nil)
		req.Header.Set("Authorization", "Bearer wrong")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Fatalf("wrong key: status %d, want 403", rr.Code)
		}
	})

	t.Run("correct key accepted", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/run", nil)
		req.Header.Set("Authorization", "Bearer sekrit")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("correct key: status %d, want 200", rr.Code)
		}
	})
}

func TestSecurityHeadersMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := SecurityHeadersMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	tests := map[string]string{
		"Content-Security-Policy": "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; font-src 'self' https://fonts.googleapis.com https://fonts.gstatic.com",
		"X-Content-Type-Options":  "nosniff",
		"X-Frame-Options":         "DENY",
		"Referrer-Policy":         "no-referrer",
	}
	for header, want := range tests {
		if got := rr.Header().Get(header); got != want {
			t.Errorf("%s: got %q, want %q", header, got, want)
		}
	}
	if rr.Code != http.StatusOK {
		t.Fatalf("status %d, want 200", rr.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := CORSMiddleware(next, "https://example.com")

	t.Run("sets CORS headers on normal request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
			t.Errorf("Allow-Origin: got %q", got)
		}
		if got := rr.Header().Get("Access-Control-Allow-Methods"); got != "GET, POST, DELETE, OPTIONS" {
			t.Errorf("Allow-Methods: got %q", got)
		}
		if got := rr.Header().Get("Access-Control-Allow-Headers"); got != "Content-Type, X-Session-ID, Authorization" {
			t.Errorf("Allow-Headers: got %q", got)
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("status %d, want 200", rr.Code)
		}
	})

	t.Run("OPTIONS preflight short-circuits with 204", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodOptions, "/run", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusNoContent {
			t.Fatalf("preflight status %d, want 204", rr.Code)
		}
		if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "https://example.com" {
			t.Errorf("preflight Allow-Origin: got %q", got)
		}
	})
}
