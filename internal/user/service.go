package user

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"buddy/server/internal/domain"
	"buddy/server/pkg/validator"

	"github.com/google/uuid"
)

// Service defines user management business capabilities.
type Service struct {
	repo       domain.UserRepository
	authClient domain.AuthClient
	// chatRepo cascades chat sessions, messages, and personal-intelligence memory when
	// a user is deleted. It's optional (may be nil, e.g. in tests that construct a
	// Service directly) so DeleteUser degrades to leaving that data behind rather than
	// panicking when it isn't wired up.
	chatRepo domain.ChatRepository
}

// NewService instantiates a new user service.
func NewService(repo domain.UserRepository, authClient domain.AuthClient, chatRepo domain.ChatRepository) *Service {
	return &Service{
		repo:       repo,
		authClient: authClient,
		chatRepo:   chatRepo,
	}
}

// CreateStudent handles student registration, identity creation, and profile provisioning.
func (s *Service) CreateStudent(ctx context.Context, req domain.CreateStudentRequest) (*domain.StudentUserComposite, error) {
	v := validator.New()
	v.Required("email", req.Email)
	v.Email("email", req.Email)
	v.Required("password", req.Password)
	v.MinLength("password", req.Password, 6)
	v.Required("display_name", req.DisplayName)
	v.Required("registration_number", req.RegistrationNumber)
	v.Required("grade", req.Grade)
	if v.HasErrors() {
		return nil, v.Error()
	}

	// 1. Resolve Identity in Auth provider.
	//
	// This endpoint is reached two different ways that both carry an authenticated
	// caller in ctx (the route requires auth): a student completing self-registration
	// (they already created their own Firebase Auth account client-side and are now
	// persisting their own profile under their own UID), or an admin/teacher creating
	// a new student on someone else's behalf (a brand-new Firebase Auth identity must
	// be created for that student — reusing the caller's own UID here would silently
	// assign the admin's own account to the new student record). The two cases are
	// told apart by whether the authenticated caller's email matches the requested
	// student email: only a genuine self-registration can match.
	var uid string
	if authUser, ok := domain.AuthUserFromContext(ctx); ok && authUser != nil && authUser.UID != "" &&
		strings.EqualFold(strings.TrimSpace(authUser.Email), strings.TrimSpace(req.Email)) {
		uid = authUser.UID
	} else {
		createdUID, err := s.authClient.CreateUser(ctx, req.Email, req.Password, req.DisplayName, string(domain.RoleStudent))
		if err != nil {
			if errors.Is(err, domain.ErrUserAlreadyExists) {
				if existingUser, getErr := s.repo.GetUserByEmail(ctx, req.Email); getErr == nil && existingUser != nil {
					return nil, domain.ErrUserAlreadyExists
				}
			}
			return nil, err
		}
		uid = createdUID
	}

	now := time.Now()
	userStatus := domain.StatusPending
	if req.Status != "" && req.Status.IsValid() {
		userStatus = req.Status
	}

	user := &domain.User{
		ID:          uid,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Role:        domain.RoleStudent,
		Status:      userStatus,
		PhoneNumber: req.PhoneNumber,
		PhotoURL:    req.PhotoURL,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 2. Persist User in repository
	if err := s.repo.CreateUser(ctx, user); err != nil {
		_ = s.authClient.DeleteUser(ctx, uid) // Rollback identity creation on failure
		return nil, fmt.Errorf("failed to save user record: %w", err)
	}

	profile := &domain.StudentProfile{
		UserID:             uid,
		RegistrationNumber: req.RegistrationNumber,
		Grade:              req.Grade,
		Section:            req.Section,
		BatchID:            req.BatchID,
		BatchName:          req.BatchName,
		DateOfBirth:        req.DateOfBirth,
		Guardian:           req.Guardian,
		EnrolledCourseIDs:  make([]string, 0),
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	// 3. Persist StudentProfile
	if err := s.repo.CreateStudentProfile(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to save student profile: %w", err)
	}

	return &domain.StudentUserComposite{
		User:    *user,
		Profile: *profile,
	}, nil
}

// CreateStaff handles teacher or admin registration and profile provisioning.
func (s *Service) CreateStaff(ctx context.Context, req domain.CreateStaffRequest) (*domain.StaffUserComposite, error) {
	v := validator.New()
	v.Required("email", req.Email)
	v.Email("email", req.Email)
	v.Required("password", req.Password)
	v.MinLength("password", req.Password, 6)
	v.Required("display_name", req.DisplayName)
	v.Required("role", string(req.Role))
	v.OneOf("role", string(req.Role), string(domain.RoleTeacher), string(domain.RoleAdmin))
	v.Required("employee_id", req.EmployeeID)
	v.Required("department", req.Department)
	v.Required("designation", req.Designation)
	if v.HasErrors() {
		return nil, v.Error()
	}

	// 1. Create Identity in Auth provider
	uid, err := s.authClient.CreateUser(ctx, req.Email, req.Password, req.DisplayName, string(req.Role))
	if err != nil {
		return nil, err
	}

	now := time.Now()
	user := &domain.User{
		ID:          uid,
		Email:       req.Email,
		DisplayName: req.DisplayName,
		Role:        req.Role,
		Status:      domain.StatusActive,
		PhoneNumber: req.PhoneNumber,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// 2. Persist User
	if err := s.repo.CreateUser(ctx, user); err != nil {
		_ = s.authClient.DeleteUser(ctx, uid)
		return nil, fmt.Errorf("failed to save user record: %w", err)
	}

	profile := &domain.StaffProfile{
		UserID:         uid,
		EmployeeID:     req.EmployeeID,
		Designation:    req.Designation,
		Department:     req.Department,
		Subjects:       req.Subjects,
		Qualifications: req.Qualifications,
		HireDate:       req.HireDate,
		Permissions:    req.Permissions,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	// 3. Persist StaffProfile
	if err := s.repo.CreateStaffProfile(ctx, profile); err != nil {
		return nil, fmt.Errorf("failed to save staff profile: %w", err)
	}

	return &domain.StaffUserComposite{
		User:    *user,
		Profile: *profile,
	}, nil
}

// GetStudent retrieves a student user and their academic profile.
func (s *Service) GetStudent(ctx context.Context, id string) (*domain.StudentUserComposite, error) {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user.Role != domain.RoleStudent {
		return nil, domain.ErrNotFound
	}

	profile, err := s.repo.GetStudentProfile(ctx, id)
	if err != nil {
		profile = &domain.StudentProfile{UserID: id}
	}

	return &domain.StudentUserComposite{
		User:    *user,
		Profile: *profile,
	}, nil
}

// UpdateStudent updates student demographic and academic profile details.
func (s *Service) UpdateStudent(ctx context.Context, id string, req domain.UpdateStudentRequest) (*domain.StudentUserComposite, error) {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user.Role != domain.RoleStudent {
		return nil, domain.ErrNotFound
	}

	profile, err := s.repo.GetStudentProfile(ctx, id)
	if err != nil {
		profile = &domain.StudentProfile{UserID: id}
	}

	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.PhoneNumber != nil {
		user.PhoneNumber = *req.PhoneNumber
	}
	if req.PhotoURL != nil {
		user.PhotoURL = *req.PhotoURL
	}

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	if req.RegistrationNumber != nil {
		profile.RegistrationNumber = *req.RegistrationNumber
	}
	if req.Grade != nil {
		profile.Grade = *req.Grade
	}
	if req.Section != nil {
		profile.Section = *req.Section
	}
	if req.BatchID != nil {
		profile.BatchID = *req.BatchID
	}
	if req.BatchName != nil {
		profile.BatchName = *req.BatchName
	}
	if req.Guardian != nil {
		profile.Guardian = req.Guardian
	}

	if err := s.repo.UpdateStudentProfile(ctx, profile); err != nil {
		return nil, err
	}

	return &domain.StudentUserComposite{
		User:    *user,
		Profile: *profile,
	}, nil
}

// GetStaff retrieves a teacher or admin user along with employment profile.
func (s *Service) GetStaff(ctx context.Context, id string) (*domain.StaffUserComposite, error) {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user.Role != domain.RoleTeacher && user.Role != domain.RoleAdmin {
		return nil, domain.ErrNotFound
	}

	profile, err := s.repo.GetStaffProfile(ctx, id)
	if err != nil {
		profile = &domain.StaffProfile{UserID: id}
	}

	return &domain.StaffUserComposite{
		User:    *user,
		Profile: *profile,
	}, nil
}

// UpdateStaff updates staff employment profile.
func (s *Service) UpdateStaff(ctx context.Context, id string, req domain.UpdateStaffRequest) (*domain.StaffUserComposite, error) {
	user, err := s.repo.GetUserByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if user.Role != domain.RoleTeacher && user.Role != domain.RoleAdmin {
		return nil, domain.ErrNotFound
	}

	profile, err := s.repo.GetStaffProfile(ctx, id)
	if err != nil {
		profile = &domain.StaffProfile{UserID: id}
	}

	if req.DisplayName != nil {
		user.DisplayName = *req.DisplayName
	}
	if req.PhoneNumber != nil {
		user.PhoneNumber = *req.PhoneNumber
	}
	if req.PhotoURL != nil {
		user.PhotoURL = *req.PhotoURL
	}

	if err := s.repo.UpdateUser(ctx, user); err != nil {
		return nil, err
	}

	if req.EmployeeID != nil {
		profile.EmployeeID = *req.EmployeeID
	}
	if req.Designation != nil {
		profile.Designation = *req.Designation
	}
	if req.Department != nil {
		profile.Department = *req.Department
	}
	if req.Subjects != nil {
		profile.Subjects = *req.Subjects
	}
	if req.Qualifications != nil {
		profile.Qualifications = *req.Qualifications
	}
	if req.Permissions != nil {
		profile.Permissions = *req.Permissions
	}

	if err := s.repo.UpdateStaffProfile(ctx, profile); err != nil {
		return nil, err
	}

	return &domain.StaffUserComposite{
		User:    *user,
		Profile: *profile,
	}, nil
}

// UpdateStatus changes user lifecycle state and syncs enabled/disabled status with Auth provider.
func (s *Service) UpdateStatus(ctx context.Context, id string, status domain.UserStatus) error {
	if !status.IsValid() {
		return domain.ErrInvalidInput
	}

	if err := s.repo.UpdateUserStatus(ctx, id, status); err != nil {
		return err
	}

	// Disable user in Firebase Auth if suspended
	disabled := status == domain.StatusSuspended
	_ = s.authClient.UpdateUserStatus(ctx, id, disabled)
	return nil
}

// DeleteUser removes a user account, its profile, and every piece of data associated
// with it: the Firebase Auth identity, chat sessions and their messages, and
// personal-intelligence memory.
//
// The Firebase Auth identity is deleted first, and a failure there aborts the whole
// operation before anything else is touched. An earlier version of this method treated
// that step as a best-effort side note and discarded its error, which meant a failed
// Auth deletion could still be reported back as "user deleted successfully" — leaving
// a Firebase Auth account that silently kept existing (and kept its email address
// unavailable for re-registration) with no record of anything having gone wrong.
// Deleting Auth first also means a failure here leaves every other record untouched,
// so the whole call can simply be retried once whatever's wrong with Auth is fixed,
// rather than resuming a half-completed cascade. FirebaseAuthClient.DeleteUser treats
// "already deleted" as success, so retrying after a partial failure (Auth gone, other
// data not yet cleaned up) is safe.
func (s *Service) DeleteUser(ctx context.Context, id string) error {
	if id == "" {
		return domain.ErrInvalidInput
	}

	if err := s.authClient.DeleteUser(ctx, id); err != nil {
		return fmt.Errorf("failed to delete firebase auth user: %w", err)
	}

	if s.chatRepo != nil {
		// Only students have chat sessions/memory today, but this is harmless to run
		// for teacher/admin accounts too — it's simply a no-op for them.
		if err := s.chatRepo.DeleteSessionsByStudent(ctx, id); err != nil {
			return fmt.Errorf("failed to delete chat sessions: %w", err)
		}
		if err := s.chatRepo.DeletePersonalIntelligence(ctx, id); err != nil {
			return fmt.Errorf("failed to delete personal intelligence memory: %w", err)
		}
	}

	return s.repo.DeleteUser(ctx, id)
}

// ListStudents returns filtered, paginated student records.
func (s *Service) ListStudents(ctx context.Context, filter domain.ListUsersFilter) ([]*domain.StudentUserComposite, string, error) {
	return s.repo.ListStudents(ctx, filter)
}

// ListStaff returns filtered, paginated staff records.
func (s *Service) ListStaff(ctx context.Context, filter domain.ListUsersFilter) ([]*domain.StaffUserComposite, string, error) {
	return s.repo.ListStaff(ctx, filter)
}

// GetUserProfileComposite retrieves the full profile of an authenticated user.
func (s *Service) GetUserProfileComposite(ctx context.Context, userID string) (interface{}, error) {
	user, err := s.repo.GetUserByID(ctx, userID)
	if err != nil {
		return nil, err
	}

	if user.Role == domain.RoleStudent {
		profile, _ := s.repo.GetStudentProfile(ctx, userID)
		composite := &domain.StudentUserComposite{User: *user}
		if profile != nil {
			composite.Profile = *profile
		}
		return composite, nil
	}

	profile, _ := s.repo.GetStaffProfile(ctx, userID)
	composite := &domain.StaffUserComposite{User: *user}
	if profile != nil {
		composite.Profile = *profile
	}
	return composite, nil
}

// GenerateID is a utility helper for generating unique IDs.
func GenerateID() string {
	return uuid.New().String()
}
