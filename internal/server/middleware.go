package server

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
)

// WriteJSON writes a JSON response with the given status code.
// The response body is encoded using json.NewEncoder with Content-Type
// set to "application/json". Errors during encoding are silently ignored
// as the headers have already been written.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
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
		for _, path := range excludedPaths {
			if r.URL.Path == path {
				next.ServeHTTP(w, r)
				return
			}
		}

		authHeader := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if !hasPrefix(authHeader, prefix) {
			http.Error(w, `{"error":"missing or invalid Authorization header"}`, http.StatusUnauthorized)
			return
		}

		token := authHeader[len(prefix):]
		if subtle.ConstantTimeCompare([]byte(token), []byte(key)) != 1 {
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

// hasPrefix is a strings.HasPrefix implementation that avoids the import.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
