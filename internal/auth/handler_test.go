package auth

import (
	"context"
	"testing"

	"buddy/server/internal/domain"
	"buddy/server/internal/user"
)

// TestSyncUserRecord_AutoProvisionsFirstTimeSignIn guards the exact bug this method
// exists to fix: "Sign In with Google" only ever creates the Firebase Auth identity
// itself (Firebase does that automatically) — it never calls any backend endpoint that
// would create the matching Firestore user document. Before this fix, a brand-new
// Google sign-in was invisible to admin's user management and, worse, silently granted
// active student access (VerifyIDToken's fallback for a token with no custom claims)
// without ever going through the same pending-admin-approval step email/password
// registration requires.
func TestSyncUserRecord_AutoProvisionsFirstTimeSignIn(t *testing.T) {
	repo := user.NewMemoryRepository()
	h := &Handler{userRepo: repo}
	ctx := context.Background()

	authUser := &domain.AuthUser{
		UID:         "google-uid-new-001",
		Email:       "new.student@example.com",
		DisplayName: "New Student",
		Role:        domain.RoleStudent, // VerifyIDToken's fallback for a token with no custom claims
		Status:      domain.StatusActive,
		Claims:      map[string]interface{}{"picture": "https://example.com/photo.jpg"},
	}

	h.syncUserRecord(ctx, authUser)

	// The record must actually be persisted, not just reflected on authUser in memory —
	// that's the whole point: it must now be visible in admin's user management.
	stored, err := repo.GetUserByID(ctx, "google-uid-new-001")
	if err != nil {
		t.Fatalf("expected a Firestore user record to have been created, got error: %v", err)
	}
	if stored.Email != "new.student@example.com" {
		t.Errorf("expected stored email to match, got %q", stored.Email)
	}
	if stored.DisplayName != "New Student" {
		t.Errorf("expected stored display name to match, got %q", stored.DisplayName)
	}
	if stored.PhotoURL != "https://example.com/photo.jpg" {
		t.Errorf("expected photo URL to be pulled from the token's picture claim, got %q", stored.PhotoURL)
	}

	// Must NOT silently keep the active/student fallback — a brand-new sign-in must
	// go through the same pending-admin-approval step self-registration requires.
	if stored.Status != domain.StatusPending {
		t.Errorf("expected new auto-provisioned user to default to pending status, got %q", stored.Status)
	}
	if stored.Role != domain.RoleStudent {
		t.Errorf("expected new auto-provisioned user to default to student role, got %q", stored.Role)
	}

	// authUser itself (what actually gets returned to the frontend this request) must
	// reflect the corrected pending status too, not the stale active fallback.
	if authUser.Status != domain.StatusPending {
		t.Errorf("expected authUser.Status to be corrected to pending, got %q", authUser.Status)
	}

	// A student profile must exist too, so GetStudent-style lookups don't need a
	// separate lazy-fallback path.
	if _, err := repo.GetStudentProfile(ctx, "google-uid-new-001"); err != nil {
		t.Errorf("expected a student profile to have been created alongside the user, got error: %v", err)
	}
}

// TestSyncUserRecord_ExistingUserIsEnrichedNotOverwritten verifies the normal path
// (a returning user, whichever provider they signed in with) is untouched: their real
// stored role/status/display name is used to correct the token's claims, and no
// duplicate or overwritten record is created.
func TestSyncUserRecord_ExistingUserIsEnrichedNotOverwritten(t *testing.T) {
	repo := user.NewMemoryRepository()
	h := &Handler{userRepo: repo}
	ctx := context.Background()

	if err := repo.CreateUser(ctx, &domain.User{
		ID:          "existing-uid-001",
		Email:       "teacher@example.com",
		DisplayName: "Real Teacher Name",
		Role:        domain.RoleTeacher,
		Status:      domain.StatusActive,
	}); err != nil {
		t.Fatalf("failed to seed existing user: %v", err)
	}

	// Simulate a token whose fallback claims are wrong/stale compared to Firestore —
	// this is the normal case any time VerifyIDToken has no custom "role" claim.
	authUser := &domain.AuthUser{
		UID:         "existing-uid-001",
		Email:       "teacher@example.com",
		DisplayName: "Stale Display Name",
		Role:        domain.RoleStudent,
		Status:      domain.StatusActive,
	}

	h.syncUserRecord(ctx, authUser)

	if authUser.Role != domain.RoleTeacher {
		t.Errorf("expected authUser.Role to be corrected from Firestore, got %q", authUser.Role)
	}
	if authUser.DisplayName != "Real Teacher Name" {
		t.Errorf("expected authUser.DisplayName to be corrected from Firestore, got %q", authUser.DisplayName)
	}
}
