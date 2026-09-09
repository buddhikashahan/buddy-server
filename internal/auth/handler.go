package auth

import (
	"encoding/json"
	"net/http"

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

	if h.userRepo != nil {
		if u, err := h.userRepo.GetUserByID(r.Context(), authUser.UID); err == nil && u != nil {
			authUser.Status = u.Status
			authUser.Role = u.Role
			authUser.DisplayName = u.DisplayName
		}
	}
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

	// Enrich with real status and role stored in Firestore
	if h.userRepo != nil {
		if u, err := h.userRepo.GetUserByID(r.Context(), authUser.UID); err == nil && u != nil {
			authUser.Status = u.Status
			authUser.Role = u.Role
			authUser.DisplayName = u.DisplayName
		}
	}
	authUser.ID = authUser.UID

	response.JSON(w, http.StatusOK, authUser)
}
