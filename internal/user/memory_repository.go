package user

import (
	"context"
	"sync"
	"time"

	"buddy/server/internal/domain"
)

// MemoryRepository provides a concurrent in-memory implementation of domain.UserRepository for development & testing.
type MemoryRepository struct {
	mu              sync.RWMutex
	users           map[string]*domain.User
	studentProfiles map[string]*domain.StudentProfile
	staffProfiles   map[string]*domain.StaffProfile
}

// NewMemoryRepository initializes an empty in-memory repository.
func NewMemoryRepository() domain.UserRepository {
	return &MemoryRepository{
		users:           make(map[string]*domain.User),
		studentProfiles: make(map[string]*domain.StudentProfile),
		staffProfiles:   make(map[string]*domain.StaffProfile),
	}
}

func (m *MemoryRepository) CreateUser(ctx context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, u := range m.users {
		if u.Email == user.Email {
			return domain.ErrUserAlreadyExists
		}
	}

	m.users[user.ID] = user
	return nil
}

func (m *MemoryRepository) GetUserByID(ctx context.Context, id string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	u, ok := m.users[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *u
	return &copy, nil
}

func (m *MemoryRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, u := range m.users {
		if u.Email == email {
			copy := *u
			return &copy, nil
		}
	}
	return nil, domain.ErrNotFound
}

func (m *MemoryRepository) UpdateUser(ctx context.Context, user *domain.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.users[user.ID]
	if !ok {
		return domain.ErrNotFound
	}

	user.CreatedAt = existing.CreatedAt
	user.UpdatedAt = time.Now()
	m.users[user.ID] = user
	return nil
}

func (m *MemoryRepository) UpdateUserStatus(ctx context.Context, id string, status domain.UserStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	u, ok := m.users[id]
	if !ok {
		return domain.ErrNotFound
	}

	u.Status = status
	u.UpdatedAt = time.Now()
	return nil
}

func (m *MemoryRepository) DeleteUser(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.users, id)
	delete(m.studentProfiles, id)
	delete(m.staffProfiles, id)
	return nil
}

func (m *MemoryRepository) CreateStudentProfile(ctx context.Context, profile *domain.StudentProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.studentProfiles[profile.UserID] = profile
	return nil
}

func (m *MemoryRepository) GetStudentProfile(ctx context.Context, userID string) (*domain.StudentProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.studentProfiles[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *p
	return &copy, nil
}

func (m *MemoryRepository) UpdateStudentProfile(ctx context.Context, profile *domain.StudentProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.studentProfiles[profile.UserID]
	if !ok {
		return domain.ErrNotFound
	}

	profile.CreatedAt = existing.CreatedAt
	profile.UpdatedAt = time.Now()
	m.studentProfiles[profile.UserID] = profile
	return nil
}

func (m *MemoryRepository) ListStudents(ctx context.Context, filter domain.ListUsersFilter) ([]*domain.StudentUserComposite, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*domain.StudentUserComposite
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	for _, u := range m.users {
		if u.Role != domain.RoleStudent {
			continue
		}
		if filter.Status != "" && u.Status != filter.Status {
			continue
		}

		profile := m.studentProfiles[u.ID]
		composite := &domain.StudentUserComposite{
			User: *u,
		}
		if profile != nil {
			composite.Profile = *profile
		}
		results = append(results, composite)
		if len(results) >= limit {
			break
		}
	}

	return results, "", nil
}

func (m *MemoryRepository) CreateStaffProfile(ctx context.Context, profile *domain.StaffProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.staffProfiles[profile.UserID] = profile
	return nil
}

func (m *MemoryRepository) GetStaffProfile(ctx context.Context, userID string) (*domain.StaffProfile, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, ok := m.staffProfiles[userID]
	if !ok {
		return nil, domain.ErrNotFound
	}
	copy := *p
	return &copy, nil
}

func (m *MemoryRepository) UpdateStaffProfile(ctx context.Context, profile *domain.StaffProfile) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, ok := m.staffProfiles[profile.UserID]
	if !ok {
		return domain.ErrNotFound
	}

	profile.CreatedAt = existing.CreatedAt
	profile.UpdatedAt = time.Now()
	m.staffProfiles[profile.UserID] = profile
	return nil
}

func (m *MemoryRepository) ListStaff(ctx context.Context, filter domain.ListUsersFilter) ([]*domain.StaffUserComposite, string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var results []*domain.StaffUserComposite
	limit := filter.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	for _, u := range m.users {
		if u.Role != domain.RoleTeacher && u.Role != domain.RoleAdmin {
			continue
		}
		if filter.Role != "" && u.Role != filter.Role {
			continue
		}
		if filter.Status != "" && u.Status != filter.Status {
			continue
		}

		profile := m.staffProfiles[u.ID]
		composite := &domain.StaffUserComposite{
			User: *u,
		}
		if profile != nil {
			composite.Profile = *profile
		}
		results = append(results, composite)
		if len(results) >= limit {
			break
		}
	}

	return results, "", nil
}
