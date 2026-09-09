package domain

import (
	"context"
)

type authContextKey string

const (
	AuthUserKey authContextKey = "auth_user"
)

// AuthUser represents the verified identity extracted from a Firebase ID token.
type AuthUser struct {
	ID          string                 `json:"id"`
	UID         string                 `json:"uid"`
	Email       string                 `json:"email"`
	DisplayName string                 `json:"display_name"`
	Role        Role                   `json:"role"`
	Status      UserStatus             `json:"status"`
	Claims      map[string]interface{} `json:"claims,omitempty"`
}

// ContextWithAuthUser injects the authenticated user into the context.
func ContextWithAuthUser(ctx context.Context, user *AuthUser) context.Context {
	return context.WithValue(ctx, AuthUserKey, user)
}

// AuthUserFromContext retrieves the authenticated user from the context.
func AuthUserFromContext(ctx context.Context) (*AuthUser, bool) {
	u, ok := ctx.Value(AuthUserKey).(*AuthUser)
	return u, ok && u != nil
}
