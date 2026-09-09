package auth

import (
	"context"
	"fmt"
	"strings"

	firebase "firebase.google.com/go/v4"
	fbauth "firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"

	"buddy/server/internal/domain"
)

// FirebaseAuthClient implements domain.AuthClient using Firebase Admin SDK.
type FirebaseAuthClient struct {
	client *fbauth.Client
}

// NewFirebaseAuthClient initializes the Firebase Auth SDK client.
func NewFirebaseAuthClient(ctx context.Context, projectID, credentialsFile string) (domain.AuthClient, error) {
	var opts []option.ClientOption
	if credentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsFile))
	}

	fbConfig := &firebase.Config{
		ProjectID: projectID,
	}

	app, err := firebase.NewApp(ctx, fbConfig, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase app: %w", err)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize firebase auth client: %w", err)
	}

	return &FirebaseAuthClient{client: authClient}, nil
}

// VerifyIDToken validates a Firebase JWT token and returns AuthUser.
func (c *FirebaseAuthClient) VerifyIDToken(ctx context.Context, idToken string) (*domain.AuthUser, error) {
	// If idToken is a dev token, resolve it locally for seamless development testing
	if strings.HasPrefix(idToken, "dev:") || strings.HasPrefix(idToken, "dev-") {
		devClient := &DevAuthClient{}
		return devClient.VerifyIDToken(ctx, idToken)
	}

	token, err := c.client.VerifyIDToken(ctx, idToken)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrUnauthorized, err)
	}

	roleStr := "student"
	if r, ok := token.Claims["role"].(string); ok && r != "" {
		roleStr = r
	}

	email := ""
	if em, ok := token.Claims["email"].(string); ok {
		email = em
	}

	name := ""
	if n, ok := token.Claims["name"].(string); ok {
		name = n
	}

	return &domain.AuthUser{
		UID:         token.UID,
		Email:       email,
		DisplayName: name,
		Role:        domain.Role(roleStr),
		Status:      domain.StatusActive,
		Claims:      token.Claims,
	}, nil
}

// CreateUser registers a new user in Firebase Auth.
func (c *FirebaseAuthClient) CreateUser(ctx context.Context, email, password, displayName, role string) (string, error) {
	params := (&fbauth.UserToCreate{}).
		Email(email).
		Password(password).
		DisplayName(displayName).
		EmailVerified(false)

	u, err := c.client.CreateUser(ctx, params)
	if err != nil {
		if fbauth.IsEmailAlreadyExists(err) {
			return "", domain.ErrUserAlreadyExists
		}
		return "", fmt.Errorf("failed to create firebase auth user: %w", err)
	}

	// Set initial role custom claim
	claims := map[string]interface{}{
		"role": role,
	}
	if err := c.client.SetCustomUserClaims(ctx, u.UID, claims); err != nil {
		return "", fmt.Errorf("failed to set custom claims: %w", err)
	}

	return u.UID, nil
}

// SetCustomUserClaims sets custom JWT claims on Firebase user.
func (c *FirebaseAuthClient) SetCustomUserClaims(ctx context.Context, uid string, claims map[string]interface{}) error {
	return c.client.SetCustomUserClaims(ctx, uid, claims)
}

// UpdateUserStatus enables or disables a user account in Firebase Auth.
func (c *FirebaseAuthClient) UpdateUserStatus(ctx context.Context, uid string, disabled bool) error {
	params := (&fbauth.UserToUpdate{}).
		Disabled(disabled)
	_, err := c.client.UpdateUser(ctx, uid, params)
	return err
}

// DeleteUser removes a user from Firebase Auth. Deleting a uid that's already gone is
// treated as success rather than an error: this keeps the operation idempotent, so a
// caller that retries after a partial failure elsewhere in a larger deletion (e.g.
// user.Service.DeleteUser's cascade) doesn't get tripped up re-deleting an identity
// that's already gone.
func (c *FirebaseAuthClient) DeleteUser(ctx context.Context, uid string) error {
	if err := c.client.DeleteUser(ctx, uid); err != nil {
		if fbauth.IsUserNotFound(err) {
			return nil
		}
		return fmt.Errorf("firebase auth: failed to delete user %s: %w", uid, err)
	}
	return nil
}

// DevAuthClient provides a mock provider for offline local testing.
type DevAuthClient struct{}

// NewDevAuthClient creates a dev-mode auth client.
func NewDevAuthClient() domain.AuthClient {
	return &DevAuthClient{}
}

func (d *DevAuthClient) VerifyIDToken(ctx context.Context, idToken string) (*domain.AuthUser, error) {
	// In dev mode, if the token is "dev-admin-token", "dev-teacher-token", or "dev-student-token"
	// or in format "dev:<role>:<uid>:<email>"
	if strings.HasPrefix(idToken, "dev:") {
		parts := strings.Split(idToken, ":")
		role := domain.RoleStudent
		uid := "dev-user-123"
		email := "dev@buddyai.local"

		if len(parts) >= 2 {
			role = domain.Role(parts[1])
		}
		if len(parts) >= 3 {
			uid = parts[2]
		}
		if len(parts) >= 4 {
			email = parts[3]
		}

		return &domain.AuthUser{
			UID:         uid,
			Email:       email,
			DisplayName: "Dev " + string(role),
			Role:        role,
			Status:      domain.StatusActive,
			Claims: map[string]interface{}{
				"role": string(role),
			},
		}, nil
	}

	switch idToken {
	case "dev-admin-token":
		return &domain.AuthUser{
			UID:         "admin-001",
			Email:       "admin@buddyai.local",
			DisplayName: "System Admin",
			Role:        domain.RoleAdmin,
			Status:      domain.StatusActive,
			Claims:      map[string]interface{}{"role": "admin"},
		}, nil
	case "dev-teacher-token":
		return &domain.AuthUser{
			UID:         "teacher-001",
			Email:       "teacher@buddyai.local",
			DisplayName: "Math Teacher",
			Role:        domain.RoleTeacher,
			Status:      domain.StatusActive,
			Claims:      map[string]interface{}{"role": "teacher"},
		}, nil
	case "dev-student-token":
		return &domain.AuthUser{
			UID:         "student-001",
			Email:       "student@buddyai.local",
			DisplayName: "Alice Student",
			Role:        domain.RoleStudent,
			Status:      domain.StatusActive,
			Claims:      map[string]interface{}{"role": "student"},
		}, nil
	}

	return nil, fmt.Errorf("%w: invalid dev token", domain.ErrUnauthorized)
}

func (d *DevAuthClient) CreateUser(ctx context.Context, email, password, displayName, role string) (string, error) {
	// In dev mode, generate a pseudo UID
	hash := fmt.Sprintf("dev-uid-%s", strings.ReplaceAll(email, "@", "-at-"))
	return hash, nil
}

func (d *DevAuthClient) SetCustomUserClaims(ctx context.Context, uid string, claims map[string]interface{}) error {
	return nil
}

func (d *DevAuthClient) UpdateUserStatus(ctx context.Context, uid string, disabled bool) error {
	return nil
}

func (d *DevAuthClient) DeleteUser(ctx context.Context, uid string) error {
	return nil
}
