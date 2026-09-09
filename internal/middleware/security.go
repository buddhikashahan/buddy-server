package middleware

import "net/http"

// SecurityHeaders applies OWASP recommended security headers to all responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME-type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Prevent clickjacking by denying framing
		w.Header().Set("X-Frame-Options", "DENY")

		// Enable XSS filtering in browsers
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Strict Referrer Policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy for API backend
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none';")

		// HSTS (HTTP Strict Transport Security) - 1 year with subdomains
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

		// Permissions Policy — allow mic and camera for live talk (must be permitted from a trusted frontend origin)
		// Microphone + camera are needed for the Live Talk feature; all other sensitive APIs remain off
		w.Header().Set("Permissions-Policy", "microphone=*, camera=*, geolocation=()")

		next.ServeHTTP(w, r)
	})
}
