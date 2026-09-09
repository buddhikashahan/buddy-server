package middleware

import (
	"errors"
	"net/http"

	"buddy/server/pkg/response"
)

// BodyLimit restricts the maximum number of bytes allowed in a request body to prevent DoS memory exhaustion.
func BodyLimit(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Body == nil {
				next.ServeHTTP(w, r)
				return
			}

			r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

			// Intercept oversized bodies cleanly
			defer func() {
				var maxBytesErr *http.MaxBytesError
				if rec := recover(); rec != nil {
					if errors.As(rec.(error), &maxBytesErr) {
						response.Error(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "Request body exceeds maximum allowed size", nil)
						return
					}
					panic(rec)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
