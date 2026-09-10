package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"buddy/server/internal/domain"
	"buddy/server/pkg/response"
)

// Handler handles auth introspection and verification endpoints.
type Handler struct {
	authClient domain.AuthClient
	userRepo   domain.UserRepository
}

// NewHandler initializes a new auth HTTP handler.
func NewHandler(authClient domain.AuthClient, userRepo domain.UserRepository) *Handler {
	return &Handler{authClient: authClient, userRepo: userRepo}
}

// Me returns the identity claims of the authenticated user from the context.
func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	authUser, ok := domain.AuthUserFromContext(r.Context())
	if !ok || authUser == nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	h.syncUserRecord(r.Context(), authUser)
	authUser.ID = authUser.UID

	response.JSON(w, http.StatusOK, authUser)
}

type verifyTokenRequest struct {
	Token string `json:"token"`
}

// VerifyToken validates a token provided in the JSON body.
func (h *Handler) VerifyToken(w http.ResponseWriter, r *http.Request) {
	var req verifyTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Token == "" {
		response.Error(w, http.StatusBadRequest, "INVALID_BODY", "Token is required in request body", nil)
		return
	}

	authUser, err := h.authClient.VerifyIDToken(r.Context(), req.Token)
	if err != nil {
		response.HandleError(w, domain.ErrUnauthorized)
		return
	}

	h.syncUserRecord(r.Context(), authUser)
	authUser.ID = authUser.UID

	response.JSON(w, http.StatusOK, authUser)
}

// syncUserRecord enriches authUser with the real status/role/display name already
// stored in Firestore for this UID — or, if this Firebase identity has never been seen
// before, creates that record now.
//
// A verified Firebase ID token alone doesn't mean a Firestore user document exists for
// it: email/password self-registration explicitly creates one (see
// user.Service.CreateStudent), but "Sign In with Google" only ever creates the
// Firebase Auth identity itself — Firebase does that automatically on first sign-in —
// and never called any backend endpoint that would create the matching Firestore
// record. Without this, that gap was invisible in two ways: the account never showed
// up anywhere in admin's user management (nothing to find it by), and — worse — since
// VerifyIDToken falls back to Role: "student", Status: "active" for a token with no
// custom claims (which a first-time sign-in never has), the student was silently
// granted full access immediately, bypassing the same pending-admin-approval step
// email/password registration requires.
//
// New records default to RoleStudent/StatusPending, matching self-registration's
// defaults: this app has no self-service path to become a teacher/admin at all
// (creating staff is admin-only, see user.Handler.CreateStaff), so a brand-new sign-in
// with no existing record is always a prospective student.
func (h *Handler) syncUserRecord(ctx context.Context, authUser *domain.AuthUser) {
	if h.userRepo == nil {
		return
	}

	if u, err := h.userRepo.GetUserByID(ctx, authUser.UID); err == nil && u != nil {
		authUser.Status = u.Status
		authUser.Role = u.Role
		authUser.DisplayName = u.DisplayName
		return
	}

	photoURL := ""
	if authUser.Claims != nil {
		if p, ok := authUser.Claims["picture"].(string); ok {
			photoURL = p
		}
	}

	now := time.Now()
	newUser := &domain.User{
		ID:          authUser.UID,
		Email:       authUser.Email,
		DisplayName: authUser.DisplayName,
		Role:        domain.RoleStudent,
		Status:      domain.StatusPending,
		PhotoURL:    photoURL,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := h.userRepo.CreateUser(ctx, newUser); err != nil {
		slog.Error("[Auth] failed to auto-provision user record for new sign-in",
			slog.String("uid", authUser.UID), slog.String("error", err.Error()))
		return
	}
	if err := h.userRepo.CreateStudentProfile(ctx, &domain.StudentProfile{
		UserID:            authUser.UID,
		EnrolledCourseIDs: make([]string, 0),
		CreatedAt:         now,
		UpdatedAt:         now,
	}); err != nil {
		slog.Error("[Auth] failed to auto-provision student profile for new sign-in",
			slog.String("uid", authUser.UID), slog.String("error", err.Error()))
	}

	slog.Info("[Auth] auto-provisioned new user record from first sign-in",
		slog.String("uid", authUser.UID), slog.String("email", authUser.Email))

	authUser.Role = newUser.Role
	authUser.Status = newUser.Status
}
