package server

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"
)

// hmacKey is a random per-process key used to compare secrets via keyed
// hashing instead of a bare hash, which is brute-forceable/precomputable.
var hmacKey = func() []byte {
	k := make([]byte, 32)
	_, _ = rand.Read(k)
	return k
}()

// WriteJSON writes a JSON response with the given status code.
// The response body is encoded using json.NewEncoder with Content-Type
// set to "application/json". Errors during encoding are silently ignored
// as the headers have already been written.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// secureCompare reports whether a and b are equal without leaking length
// via early-exit timing (keyed-hashes both sides first, then compares in
// constant time via hmac.Equal).
func secureCompare(a, b string) bool {
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write([]byte(a))
	ha := mac.Sum(nil)

	mac = hmac.New(sha256.New, hmacKey)
	mac.Write([]byte(b))
	hb := mac.Sum(nil)

	return hmac.Equal(ha, hb)
}

// APIKeyMiddleware enforces Bearer token authentication on all endpoints
// except those specified in excludedPaths (typically GET / and GET /health).
//
// Clients must include the header: Authorization: Bearer <key>
//
// Returns 401 Unauthorized if the Authorization header is missing or malformed.
// Returns 403 Forbidden if the API key doesn't match.
// Allows unauthenticated access to paths in excludedPaths.
func APIKeyMiddleware(next http.Handler, key string, excludedPaths ...string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Allow unauthenticated access to excluded paths.
		if slices.Contains(excludedPaths, r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !strings.HasPrefix(authHeader, prefix) {
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := authHeader[len(prefix):]
		if !secureCompare(token, key) {
			http.Error(w, `{"error":"invalid API key"}`, http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware adds security headers to all responses:
//   - Content-Security-Policy: restricts resource loading to same-origin
//   - X-Content-Type-Options: prevents MIME type sniffing
//   - X-Frame-Options: prevents clickjacking (DENY all framing)
//   - Referrer-Policy: controls referrer information in navigation
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; font-src 'self' https://fonts.googleapis.com https://fonts.gstatic.com")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// CORSMiddleware adds CORS headers for the specified allowed origin.
// Sets Access-Control-Allow-Origin, Allow-Methods, and Allow-Headers.
// Responds with 204 No Content to OPTIONS preflight requests.
func CORSMiddleware(next http.Handler, allowedOrigin string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Session-ID, Authorization")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RateLimitMiddleware applies a simple per-IP fixed window to expensive routes.
// limit is max requests per window duration. When trustProxy is false, only
// RemoteAddr is used (X-Forwarded-For is ignored).
func RateLimitMiddleware(next http.Handler, limit int, window time.Duration, trustProxy bool) http.Handler {
	if limit <= 0 {
		limit = 60
	}
	if window <= 0 {
		window = time.Minute
	}
	type bucket struct {
		count int
		reset time.Time
	}
	var mu sync.Mutex
	buckets := make(map[string]*bucket)
	var lastPrune time.Time

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip cheap read-only endpoints.
		if r.Method == http.MethodGet && (r.URL.Path == "/" || r.URL.Path == "/health") {
			next.ServeHTTP(w, r)
			return
		}
		ip := clientIP(r, trustProxy)
		now := time.Now()
		mu.Lock()
		// Prune expired buckets periodically to bound memory.
		if lastPrune.IsZero() || now.Sub(lastPrune) > window {
			for k, b := range buckets {
				if now.After(b.reset) {
					delete(buckets, k)
				}
			}
			lastPrune = now
		}
		b, ok := buckets[ip]
		if !ok || now.After(b.reset) {
			b = &bucket{count: 0, reset: now.Add(window)}
			buckets[ip] = b
		}
		b.count++
		over := b.count > limit
		mu.Unlock()
		if over {
			http.Error(w, `{"error":"rate limit exceeded"}`, http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func clientIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			return strings.TrimSpace(parts[0])
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
