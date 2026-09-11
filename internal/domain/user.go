package domain

import (
	"context"
	"time"
)

// Role defines role-based access control tiers.
type Role string

const (
	RoleAdmin   Role = "admin"
	RoleTeacher Role = "teacher"
	RoleStudent Role = "student"
)

func (r Role) IsValid() bool {
	switch r {
	case RoleAdmin, RoleTeacher, RoleStudent:
		return true
	default:
		return false
	}
}

// UserStatus indicates the account lifecycle state.
type UserStatus string

const (
	StatusActive    UserStatus = "active"
	StatusSuspended UserStatus = "suspended"
	StatusPending   UserStatus = "pending"
)

func (s UserStatus) IsValid() bool {
	switch s {
	case StatusActive, StatusSuspended, StatusPending:
		return true
	default:
		return false
	}
}

// User represents the central account identity in Buddy AI LMS.
type User struct {
	ID          string     `json:"id" firestore:"id"`
	Email       string     `json:"email" firestore:"email"`
	DisplayName string     `json:"display_name" firestore:"display_name"`
	Role        Role       `json:"role" firestore:"role"`
	Status      UserStatus `json:"status" firestore:"status"`
	PhoneNumber string     `json:"phone_number,omitempty" firestore:"phone_number,omitempty"`
	PhotoURL    string     `json:"photo_url,omitempty" firestore:"photo_url,omitempty"`
	CreatedAt   time.Time  `json:"created_at" firestore:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" firestore:"updated_at"`
}

// GuardianInfo holds parent or emergency contact details for a student.
type GuardianInfo struct {
	Name         string `json:"name" firestore:"name"`
	Relationship string `json:"relationship" firestore:"relationship"`
	Phone        string `json:"phone" firestore:"phone"`
	Email        string `json:"email,omitempty" firestore:"email,omitempty"`
}

// StudentProfile holds specific details for student users.
type StudentProfile struct {
	UserID             string                 `json:"user_id" firestore:"user_id"`
	RegistrationNumber string                 `json:"registration_number" firestore:"registration_number"`
	Grade              string                 `json:"grade" firestore:"grade"`
	Section            string                 `json:"section,omitempty" firestore:"section,omitempty"`
	BatchID            string                 `json:"batch_id,omitempty" firestore:"batch_id,omitempty"`
	BatchName          string                 `json:"batch_name,omitempty" firestore:"batch_name,omitempty"`
	DateOfBirth        string                 `json:"date_of_birth,omitempty" firestore:"date_of_birth,omitempty"`
	Guardian           *GuardianInfo          `json:"guardian,omitempty" firestore:"guardian,omitempty"`
	EnrolledCourseIDs  []string               `json:"enrolled_course_ids" firestore:"enrolled_course_ids"`
	Metadata           map[string]interface{} `json:"metadata,omitempty" firestore:"metadata,omitempty"`
	CreatedAt          time.Time              `json:"created_at" firestore:"created_at"`
	UpdatedAt          time.Time              `json:"updated_at" firestore:"updated_at"`
}

// StaffProfile holds specific details for academic and administrative staff.
type StaffProfile struct {
	UserID         string                 `json:"user_id" firestore:"user_id"`
	EmployeeID     string                 `json:"employee_id" firestore:"employee_id"`
	Designation    string                 `json:"designation" firestore:"designation"`
	Department     string                 `json:"department" firestore:"department"`
	Subjects       []string               `json:"subjects,omitempty" firestore:"subjects,omitempty"`
	Qualifications []string               `json:"qualifications,omitempty" firestore:"qualifications,omitempty"`
	HireDate       string                 `json:"hire_date,omitempty" firestore:"hire_date,omitempty"`
	Permissions    []string               `json:"permissions,omitempty" firestore:"permissions,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty" firestore:"metadata,omitempty"`
	CreatedAt      time.Time              `json:"created_at" firestore:"created_at"`
	UpdatedAt      time.Time              `json:"updated_at" firestore:"updated_at"`
}

// StudentUserComposite aggregates User and StudentProfile.
type StudentUserComposite struct {
	User
	Profile StudentProfile `json:"profile"`
}

// StaffUserComposite aggregates User and StaffProfile.
type StaffUserComposite struct {
	User
	Profile StaffProfile `json:"profile"`
}

// CreateStudentRequest is the DTO payload to create a new student.
type CreateStudentRequest struct {
	Email              string        `json:"email"`
	Password           string        `json:"password"`
	DisplayName        string        `json:"display_name"`
	PhoneNumber        string        `json:"phone_number,omitempty"`
	PhotoURL           string        `json:"photo_url,omitempty"`
	RegistrationNumber string        `json:"registration_number"`
	Grade              string        `json:"grade"`
	Section            string        `json:"section,omitempty"`
	BatchID            string        `json:"batch_id,omitempty"`
	BatchName          string        `json:"batch_name,omitempty"`
	DateOfBirth        string        `json:"date_of_birth,omitempty"`
	Guardian           *GuardianInfo `json:"guardian,omitempty"`
	Status             UserStatus    `json:"status,omitempty"`
}

// CreateStaffRequest is the DTO payload to create a teacher or admin.
type CreateStaffRequest struct {
	Email          string   `json:"email"`
	Password       string   `json:"password"`
	DisplayName    string   `json:"display_name"`
	PhoneNumber    string   `json:"phone_number,omitempty"`
	Role           Role     `json:"role"` // RoleTeacher or RoleAdmin
	EmployeeID     string   `json:"employee_id"`
	Designation    string   `json:"designation"`
	Department     string   `json:"department"`
	Subjects       []string `json:"subjects,omitempty"`
	Qualifications []string `json:"qualifications,omitempty"`
	HireDate       string   `json:"hire_date,omitempty"`
	Permissions    []string `json:"permissions,omitempty"`
}

// UpdateStudentRequest is the DTO payload to update student info.
type UpdateStudentRequest struct {
	DisplayName        *string       `json:"display_name,omitempty"`
	PhoneNumber        *string       `json:"phone_number,omitempty"`
	PhotoURL           *string       `json:"photo_url,omitempty"`
	RegistrationNumber *string       `json:"registration_number,omitempty"`
	Grade              *string       `json:"grade,omitempty"`
	Section            *string       `json:"section,omitempty"`
	BatchID            *string       `json:"batch_id,omitempty"`
	BatchName          *string       `json:"batch_name,omitempty"`
	Guardian           *GuardianInfo `json:"guardian,omitempty"`
}

// UpdateStaffRequest is the DTO payload to update staff info.
type UpdateStaffRequest struct {
	DisplayName    *string   `json:"display_name,omitempty"`
	PhoneNumber    *string   `json:"phone_number,omitempty"`
	PhotoURL       *string   `json:"photo_url,omitempty"`
	EmployeeID     *string   `json:"employee_id,omitempty"`
	Designation    *string   `json:"designation,omitempty"`
	Department     *string   `json:"department,omitempty"`
	Subjects       *[]string `json:"subjects,omitempty"`
	Qualifications *[]string `json:"qualifications,omitempty"`
	Permissions    *[]string `json:"permissions,omitempty"`
}

// UpdateStatusRequest payload to activate or suspend a user, optionally reassigning
// their role in the same call — this is how an admin approves a pending account
// (typically an auto-provisioned first-time Google sign-in, always created as a
// student) while also correcting its role to teacher/admin, since there's no other
// way to promote an account past self-registration's student-only default.
type UpdateStatusRequest struct {
	Status UserStatus `json:"status"`
	Role   Role       `json:"role,omitempty"`
	Reason string     `json:"reason,omitempty"`
}

// ListUsersFilter provides criteria for listing users.
type ListUsersFilter struct {
	Role   Role       `json:"role,omitempty"`
	Status UserStatus `json:"status,omitempty"`
	Limit  int        `json:"limit"`
	Cursor string     `json:"cursor,omitempty"`
}

// UserRepository provides data access methods for users and profiles in Firestore.
type UserRepository interface {
	CreateUser(ctx context.Context, user *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	UpdateUser(ctx context.Context, user *User) error
	UpdateUserStatus(ctx context.Context, id string, status UserStatus) error
	DeleteUser(ctx context.Context, id string) error

	CreateStudentProfile(ctx context.Context, profile *StudentProfile) error
	GetStudentProfile(ctx context.Context, userID string) (*StudentProfile, error)
	UpdateStudentProfile(ctx context.Context, profile *StudentProfile) error
	ListStudents(ctx context.Context, filter ListUsersFilter) ([]*StudentUserComposite, string, error)

	CreateStaffProfile(ctx context.Context, profile *StaffProfile) error
	GetStaffProfile(ctx context.Context, userID string) (*StaffProfile, error)
	UpdateStaffProfile(ctx context.Context, profile *StaffProfile) error
	ListStaff(ctx context.Context, filter ListUsersFilter) ([]*StaffUserComposite, string, error)
}

// AuthClient provides abstraction for Identity Provider (Firebase Auth or Dev Mock).
type AuthClient interface {
	VerifyIDToken(ctx context.Context, idToken string) (*AuthUser, error)
	CreateUser(ctx context.Context, email, password, displayName, role string) (string, error)
	SetCustomUserClaims(ctx context.Context, uid string, claims map[string]interface{}) error
	UpdateUserStatus(ctx context.Context, uid string, disabled bool) error
	DeleteUser(ctx context.Context, uid string) error
}
