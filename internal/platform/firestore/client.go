package firestore

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"
)

const (
	CollectionUsers           = "users"
	CollectionStudentProfiles = "student_profiles"
	CollectionStaffProfiles   = "staff_profiles"
	CollectionAuditLogs       = "audit_logs"
	CollectionBatches         = "batches"
)

// NewClient initializes a Firestore client based on project ID and credentials.
func NewClient(ctx context.Context, projectID, databaseID, credentialsFile string) (*firestore.Client, error) {
	var opts []option.ClientOption
	if credentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(credentialsFile))
	}

	var client *firestore.Client
	var err error

	if databaseID != "" && databaseID != "(default)" {
		client, err = firestore.NewClientWithDatabase(ctx, projectID, databaseID, opts...)
	} else {
		client, err = firestore.NewClient(ctx, projectID, opts...)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create firestore client: %w", err)
	}

	return client, nil
}
