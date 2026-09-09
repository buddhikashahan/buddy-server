package user

import (
	"context"
	"fmt"
	"time"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"

	"buddy/server/internal/domain"
	fs "buddy/server/internal/platform/firestore"
)

// FirestoreRepository implements domain.UserRepository backed by Google Cloud Firestore.
type FirestoreRepository struct {
	client *firestore.Client
}

// NewFirestoreRepository creates a new Firestore-backed repository.
func NewFirestoreRepository(client *firestore.Client) domain.UserRepository {
	return &FirestoreRepository{client: client}
}

func (r *FirestoreRepository) CreateUser(ctx context.Context, user *domain.User) error {
	userRef := r.client.Collection(fs.CollectionUsers).Doc(user.ID)
	_, err := userRef.Set(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to create user document: %w", err)
	}
	return nil
}

func (r *FirestoreRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	docSnap, err := r.client.Collection(fs.CollectionUsers).Doc(id).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	var u domain.User
	if err := docSnap.DataTo(&u); err != nil {
		return nil, fmt.Errorf("failed to decode user: %w", err)
	}
	return &u, nil
}

func (r *FirestoreRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	iter := r.client.Collection(fs.CollectionUsers).Where("email", "==", email).Limit(1).Documents(ctx)
	docSnap, err := iter.Next()
	if err == iterator.Done {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query by email failed: %w", err)
	}

	var u domain.User
	if err := docSnap.DataTo(&u); err != nil {
		return nil, fmt.Errorf("failed to decode user: %w", err)
	}
	return &u, nil
}

func (r *FirestoreRepository) UpdateUser(ctx context.Context, user *domain.User) error {
	user.UpdatedAt = time.Now()
	_, err := r.client.Collection(fs.CollectionUsers).Doc(user.ID).Set(ctx, user)
	if err != nil {
		return fmt.Errorf("failed to update user: %w", err)
	}
	return nil
}

func (r *FirestoreRepository) UpdateUserStatus(ctx context.Context, id string, status domain.UserStatus) error {
	_, err := r.client.Collection(fs.CollectionUsers).Doc(id).Update(ctx, []firestore.Update{
		{Path: "status", Value: status},
		{Path: "updated_at", Value: time.Now()},
	})
	if err != nil {
		return fmt.Errorf("failed to update user status: %w", err)
	}
	return nil
}

func (r *FirestoreRepository) DeleteUser(ctx context.Context, id string) error {
	// 1. Delete associated profile documents
	_, _ = r.client.Collection(fs.CollectionStudentProfiles).Doc(id).Delete(ctx)
	_, _ = r.client.Collection(fs.CollectionStaffProfiles).Doc(id).Delete(ctx)

	// 2. Delete student memory context document
	_, _ = r.client.Collection("student_memories").Doc(id).Delete(ctx)

	// 3. Cascade delete all chat sessions and messages authored by this student
	sessIter := r.client.Collection("chat_sessions").Where("student_id", "==", id).Documents(ctx)
	for {
		sDoc, err := sessIter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			break
		}
		sessionID := sDoc.Ref.ID
		// Delete all messages belonging to this session
		msgIter := r.client.Collection("chat_messages").Where("session_id", "==", sessionID).Documents(ctx)
		for {
			mDoc, mErr := msgIter.Next()
			if mErr == iterator.Done || mErr != nil {
				break
			}
			_, _ = mDoc.Ref.Delete(ctx)
		}
		_, _ = sDoc.Ref.Delete(ctx)
	}

	// 4. Delete root user document
	_, err := r.client.Collection(fs.CollectionUsers).Doc(id).Delete(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete user document: %w", err)
	}
	return nil
}

func (r *FirestoreRepository) CreateStudentProfile(ctx context.Context, profile *domain.StudentProfile) error {
	_, err := r.client.Collection(fs.CollectionStudentProfiles).Doc(profile.UserID).Set(ctx, profile)
	if err != nil {
		return fmt.Errorf("failed to create student profile: %w", err)
	}
	return nil
}

func (r *FirestoreRepository) GetStudentProfile(ctx context.Context, userID string) (*domain.StudentProfile, error) {
	docSnap, err := r.client.Collection(fs.CollectionStudentProfiles).Doc(userID).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	var p domain.StudentProfile
	if err := docSnap.DataTo(&p); err != nil {
		return nil, fmt.Errorf("failed to decode student profile: %w", err)
	}
	return &p, nil
}

func (r *FirestoreRepository) UpdateStudentProfile(ctx context.Context, profile *domain.StudentProfile) error {
	profile.UpdatedAt = time.Now()
	_, err := r.client.Collection(fs.CollectionStudentProfiles).Doc(profile.UserID).Set(ctx, profile)
	if err != nil {
		return fmt.Errorf("failed to update student profile: %w", err)
	}
	return nil
}

func (r *FirestoreRepository) ListStudents(ctx context.Context, filter domain.ListUsersFilter) ([]*domain.StudentUserComposite, string, error) {
	query := r.client.Collection(fs.CollectionUsers).Where("role", "==", domain.RoleStudent)
	if filter.Status != "" {
		query = query.Where("status", "==", filter.Status)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	query = query.Limit(limit)

	iter := query.Documents(ctx)
	var results []*domain.StudentUserComposite

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("iterating students failed: %w", err)
		}

		var u domain.User
		if err := doc.DataTo(&u); err != nil {
			continue
		}

		profile, _ := r.GetStudentProfile(ctx, u.ID)
		composite := &domain.StudentUserComposite{User: u}
		if profile != nil {
			composite.Profile = *profile
		}
		results = append(results, composite)
	}

	return results, "", nil
}

func (r *FirestoreRepository) CreateStaffProfile(ctx context.Context, profile *domain.StaffProfile) error {
	_, err := r.client.Collection(fs.CollectionStaffProfiles).Doc(profile.UserID).Set(ctx, profile)
	if err != nil {
		return fmt.Errorf("failed to create staff profile: %w", err)
	}
	return nil
}

func (r *FirestoreRepository) GetStaffProfile(ctx context.Context, userID string) (*domain.StaffProfile, error) {
	docSnap, err := r.client.Collection(fs.CollectionStaffProfiles).Doc(userID).Get(ctx)
	if err != nil {
		return nil, domain.ErrNotFound
	}

	var p domain.StaffProfile
	if err := docSnap.DataTo(&p); err != nil {
		return nil, fmt.Errorf("failed to decode staff profile: %w", err)
	}
	return &p, nil
}

func (r *FirestoreRepository) UpdateStaffProfile(ctx context.Context, profile *domain.StaffProfile) error {
	profile.UpdatedAt = time.Now()
	_, err := r.client.Collection(fs.CollectionStaffProfiles).Doc(profile.UserID).Set(ctx, profile)
	if err != nil {
		return fmt.Errorf("failed to update staff profile: %w", err)
	}
	return nil
}

func (r *FirestoreRepository) ListStaff(ctx context.Context, filter domain.ListUsersFilter) ([]*domain.StaffUserComposite, string, error) {
	query := r.client.Collection(fs.CollectionUsers).Query
	if filter.Role != "" {
		query = query.Where("role", "==", filter.Role)
	}
	if filter.Status != "" {
		query = query.Where("status", "==", filter.Status)
	}

	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	query = query.Limit(limit)

	iter := query.Documents(ctx)
	var results []*domain.StaffUserComposite

	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, "", fmt.Errorf("iterating staff failed: %w", err)
		}

		var u domain.User
		if err := doc.DataTo(&u); err != nil {
			continue
		}

		// Filter out students if no explicit role was set in filter
		if filter.Role == "" && u.Role == domain.RoleStudent {
			continue
		}

		profile, _ := r.GetStaffProfile(ctx, u.ID)
		composite := &domain.StaffUserComposite{User: u}
		if profile != nil {
			composite.Profile = *profile
		}
		results = append(results, composite)
	}

	return results, "", nil
}
