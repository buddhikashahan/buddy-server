package config_test

import (
	"context"
	"os"
	"testing"

	"buddy/server/internal/config"
	platformAuth "buddy/server/internal/platform/auth"
	platformFirestore "buddy/server/internal/platform/firestore"
)

func TestConfig_Load(t *testing.T) {
	cfg := config.Load()

	if cfg.GCPProjectID != "buddy-2b92f" {
		t.Errorf("expected GCPProjectID to be 'buddy-2b92f', got '%s'", cfg.GCPProjectID)
	}

	if _, err := os.Stat(cfg.FirebaseKeyPath); err != nil {
		// A local service-account key is a development convenience, not a
		// requirement — production runs on Application Default Credentials with
		// no key file at all (see internal/config/config.go). Don't fail this
		// test in CI or a fresh clone just because that file isn't present.
		t.Skipf("no local service-account key at %q (expected on a dev machine, not in CI or production): %v", cfg.FirebaseKeyPath, err)
	}

	t.Logf("Config loaded successfully: ProjectID=%s, KeyPath=%s, Port=%s, AuthDevMode=%v",
		cfg.GCPProjectID, cfg.FirebaseKeyPath, cfg.Port, cfg.AuthDevMode)
}

func TestGCPCredentialsConnectivity(t *testing.T) {
	cfg := config.Load()

	// This test makes live calls to Firebase Auth and Firestore, so it only makes
	// sense — and can only succeed — with real local credentials in place.
	if _, err := os.Stat(cfg.FirebaseKeyPath); err != nil {
		t.Skipf("skipping live GCP connectivity test: no local credentials at %q", cfg.FirebaseKeyPath)
	}

	ctx := context.Background()

	// Verify Firebase Auth client connects with firebase.json
	authClient, err := platformAuth.NewFirebaseAuthClient(ctx, cfg.GCPProjectID, cfg.FirebaseKeyPath)
	if err != nil {
		t.Fatalf("Firebase Auth initialization failed: %v", err)
	}
	t.Logf("Firebase Auth initialized successfully: %T", authClient)

	// Verify Firestore client connects
	fsClient, err := platformFirestore.NewClient(ctx, cfg.GCPProjectID, cfg.FirestoreDBID, cfg.FirebaseKeyPath)
	if err != nil {
		t.Fatalf("Firestore client initialization failed: %v", err)
	}
	defer fsClient.Close()
	t.Logf("Firestore client initialized successfully for project '%s'", cfg.GCPProjectID)
}
