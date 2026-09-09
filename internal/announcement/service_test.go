package announcement_test

import (
	"context"
	"testing"

	"buddy/server/internal/announcement"
	"buddy/server/internal/domain"
)

func ctxAs(role domain.Role, uid string) context.Context {
	return domain.ContextWithAuthUser(context.Background(), &domain.AuthUser{
		ID: uid, UID: uid, Role: role, DisplayName: "Test " + string(role), Status: domain.StatusActive,
	})
}

func setupTestService() *announcement.Service {
	return announcement.NewService(announcement.NewMemoryAnnouncementRepository())
}

func TestAnnouncementService_CreateAndList(t *testing.T) {
	svc := setupTestService()
	teacherCtx := ctxAs(domain.RoleTeacher, "teacher-1")

	if _, err := svc.CreateAnnouncement(teacherCtx, domain.CreateAnnouncementRequest{
		Title: "Welcome", Content: "Welcome to the new term!",
	}); err != nil {
		t.Fatalf("CreateAnnouncement failed: %v", err)
	}

	// The feed is platform-wide: a student reads the same list a teacher posted to,
	// with no per-subject scoping.
	studentCtx := ctxAs(domain.RoleStudent, "student-1")
	list, err := svc.ListAnnouncements(studentCtx)
	if err != nil {
		t.Fatalf("ListAnnouncements failed: %v", err)
	}
	if len(list) != 1 || list[0].Title != "Welcome" {
		t.Fatalf("expected the student to see the announcement a teacher posted, got %+v", list)
	}
}

func TestAnnouncementService_PinnedSortsFirst(t *testing.T) {
	svc := setupTestService()
	ctx := ctxAs(domain.RoleAdmin, "admin-1")

	if _, err := svc.CreateAnnouncement(ctx, domain.CreateAnnouncementRequest{Title: "Regular", Content: "..."}); err != nil {
		t.Fatalf("CreateAnnouncement failed: %v", err)
	}
	pinned, err := svc.CreateAnnouncement(ctx, domain.CreateAnnouncementRequest{Title: "Pinned", Content: "...", Pinned: true})
	if err != nil {
		t.Fatalf("CreateAnnouncement failed: %v", err)
	}

	list, err := svc.ListAnnouncements(ctx)
	if err != nil {
		t.Fatalf("ListAnnouncements failed: %v", err)
	}
	if len(list) != 2 || list[0].ID != pinned.ID {
		t.Fatalf("expected the pinned announcement to sort first, got %+v", list)
	}
}

func TestAnnouncementService_AnyTeacherCanEditAnyAnnouncement(t *testing.T) {
	svc := setupTestService()
	authorCtx := ctxAs(domain.RoleTeacher, "teacher-1")
	otherTeacherCtx := ctxAs(domain.RoleTeacher, "teacher-2")

	a, err := svc.CreateAnnouncement(authorCtx, domain.CreateAnnouncementRequest{Title: "Original", Content: "..."})
	if err != nil {
		t.Fatalf("CreateAnnouncement failed: %v", err)
	}

	newTitle := "Edited by someone else"
	if _, err := svc.UpdateAnnouncement(otherTeacherCtx, a.ID, domain.UpdateAnnouncementRequest{Title: &newTitle}); err != nil {
		t.Fatalf("expected a non-authoring teacher to be able to update the announcement, got error: %v", err)
	}

	if err := svc.DeleteAnnouncement(otherTeacherCtx, a.ID); err != nil {
		t.Fatalf("expected a non-authoring teacher to be able to delete the announcement, got error: %v", err)
	}
}
