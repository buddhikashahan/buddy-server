package middleware

import (
	"context"
	"net/http"
	"strings"

	"buddy/server/internal/domain"
	"buddy/server/pkg/logger"
	"buddy/server/pkg/response"
)

// Authenticate verifies the incoming Bearer token using AuthClient.
func Authenticate(authClient domain.AuthClient, devMode bool) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")

			var token string
			if strings.HasPrefix(authHeader, "Bearer ") {
				token = strings.TrimPrefix(authHeader, "Bearer ")
			}

			// Dev mode fallback headers for rapid API prototyping
			if token == "" && devMode {
				devUID := r.Header.Get("X-Dev-User-Id")
				if devUID == "" {
					devUID = r.Header.Get("X-Dev-User-ID")
				}
				devRole := r.Header.Get("X-Dev-Role")
				if devRole == "" {
					devRole = "student"
				}
				if devUID != "" {
					token = "dev:" + devRole + ":" + devUID + ":dev@buddyai.local"
				}
			}

			if token == "" {
				response.HandleError(w, domain.ErrUnauthorized)
				return
			}

			authUser, err := authClient.VerifyIDToken(r.Context(), token)
			if err != nil {
				response.HandleError(w, domain.ErrUnauthorized)
				return
			}

			// Add auth user and user ID to context
			ctx := domain.ContextWithAuthUser(r.Context(), authUser)
			ctx = context.WithValue(ctx, logger.UserIDKey, authUser.UID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
