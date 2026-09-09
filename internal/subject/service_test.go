package subject_test

import (
	"context"
	"testing"

	"buddy/server/internal/domain"
	"buddy/server/internal/subject"
)

func ctxAs(role domain.Role, uid string) context.Context {
	return domain.ContextWithAuthUser(context.Background(), &domain.AuthUser{
		ID: uid, UID: uid, Role: role, DisplayName: "Test " + string(role), Status: domain.StatusActive,
	})
}

func setupTestService() *subject.Service {
	return subject.NewService(subject.NewMemorySubjectRepository())
}

func TestSubjectService_CreateAndList(t *testing.T) {
	svc := setupTestService()
	teacherCtx := ctxAs(domain.RoleTeacher, "teacher-1")

	created, err := svc.CreateSubject(teacherCtx, domain.CreateSubjectRequest{
		Title:         "Applied Thermodynamics",
		Description:   "Heat engines and cycle efficiency.",
		CoverImageURL: "https://example.com/cover.jpg",
	})
	if err != nil {
		t.Fatalf("CreateSubject failed: %v", err)
	}
	if created.CreatedBy != "teacher-1" {
		t.Errorf("expected CreatedBy to be set to the creating user, got %q", created.CreatedBy)
	}

	// Any signed-in role — including a student — can read the full subject list;
	// subjects are shared library content, not batch- or owner-scoped.
	studentCtx := ctxAs(domain.RoleStudent, "student-1")
	subjects, err := svc.ListSubjects(studentCtx)
	if err != nil {
		t.Fatalf("ListSubjects failed: %v", err)
	}
	if len(subjects) != 1 || subjects[0].Title != "Applied Thermodynamics" {
		t.Fatalf("expected the student to see the subject a teacher created, got %+v", subjects)
	}
}

func TestSubjectService_AnyTeacherCanEditAnySubject(t *testing.T) {
	svc := setupTestService()
	creatorCtx := ctxAs(domain.RoleTeacher, "teacher-1")
	otherTeacherCtx := ctxAs(domain.RoleTeacher, "teacher-2")

	c, err := svc.CreateSubject(creatorCtx, domain.CreateSubjectRequest{Title: "Physics"})
	if err != nil {
		t.Fatalf("CreateSubject failed: %v", err)
	}

	// Unlike the earlier per-batch "course" model, a subject has no single owner —
	// any teacher or admin may manage any subject.
	newTitle := "Advanced Physics"
	updated, err := svc.UpdateSubject(otherTeacherCtx, c.ID, domain.UpdateSubjectRequest{Title: &newTitle})
	if err != nil {
		t.Fatalf("expected a non-creating teacher to be able to update the subject, got error: %v", err)
	}
	if updated.Title != newTitle {
		t.Errorf("expected title to be updated, got %q", updated.Title)
	}
}

func TestSubjectService_DeleteSubject_CascadesMaterials(t *testing.T) {
	svc := setupTestService()
	teacherCtx := ctxAs(domain.RoleTeacher, "teacher-1")

	c, err := svc.CreateSubject(teacherCtx, domain.CreateSubjectRequest{Title: "Physics"})
	if err != nil {
		t.Fatalf("CreateSubject failed: %v", err)
	}
	if _, err := svc.CreateMaterial(teacherCtx, c.ID, domain.CreateMaterialRequest{
		Title: "Lecture 1", FileName: "lecture1.pdf", FileURL: "https://example.com/lecture1.pdf",
	}); err != nil {
		t.Fatalf("CreateMaterial failed: %v", err)
	}

	if err := svc.DeleteSubject(teacherCtx, c.ID); err != nil {
		t.Fatalf("DeleteSubject failed: %v", err)
	}

	// Access checks resolve the subject first, so once it's gone, its materials are
	// unreachable through the API — the strongest observable proof the cascade
	// actually happened.
	if _, err := svc.ListMaterials(teacherCtx, c.ID); err != domain.ErrNotFound {
		t.Errorf("expected ErrNotFound listing materials of a deleted subject, got %v", err)
	}
}
