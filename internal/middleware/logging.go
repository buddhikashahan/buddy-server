package middleware

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"buddy/server/pkg/logger"

	"github.com/go-chi/chi/v5/middleware"
)

// RequestLogger logs incoming HTTP requests with latency, status, and context IDs using slog.
func RequestLogger(log *slog.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqID := middleware.GetReqID(r.Context())

			// Inject request_id into context for downstream slog calls
			ctx := context.WithValue(r.Context(), logger.RequestIDKey, reqID)
			r = r.WithContext(ctx)

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			defer func() {
				latency := time.Since(start)
				status := ww.Status()

				attrs := []any{
					slog.String("method", r.Method),
					slog.String("path", r.URL.Path),
					slog.Int("status", status),
					slog.Duration("latency", latency),
					slog.String("ip", r.RemoteAddr),
					slog.Int("bytes", ww.BytesWritten()),
				}

				if status >= 500 {
					log.ErrorContext(ctx, "HTTP request failed", attrs...)
				} else if status >= 400 {
					log.WarnContext(ctx, "HTTP client warning", attrs...)
				} else {
					log.InfoContext(ctx, "HTTP request handled", attrs...)
				}
			}()

			next.ServeHTTP(ww, r)
		})
	}
}
