package middleware

import (
	"net/http"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

// RequireRoles enforces that the authenticated user possesses one of the allowed roles.
func RequireRoles(allowedRoles ...domain.Role) func(next http.Handler) http.Handler {
	allowedMap := make(map[domain.Role]bool, len(allowedRoles))
	for _, r := range allowedRoles {
		allowedMap[r] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user, ok := domain.AuthUserFromContext(r.Context())
			if !ok || user == nil {
				response.HandleError(w, domain.ErrUnauthorized)
				return
			}

			if !allowedMap[user.Role] {
				response.HandleError(w, domain.ErrForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

