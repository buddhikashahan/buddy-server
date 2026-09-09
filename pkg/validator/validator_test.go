package validator_test

import (
	"testing"

	"buddy/server/pkg/validator"
)

func TestValidator_Required(t *testing.T) {
	v := validator.New()
	v.Required("name", "")
	if !v.HasErrors() {
		t.Fatal("expected validation error for empty name")
	}

	v2 := validator.New()
	v2.Required("name", "Buddy")
	if v2.HasErrors() {
		t.Fatal("expected no validation error for non-empty name")
	}
}

func TestValidator_Email(t *testing.T) {
	tests := []struct {
		email   string
		isValid bool
	}{
		{"student@buddyai.com", true},
		{"teacher.doe@school.edu", true},
		{"invalid-email", false},
		{"@missing-local.com", false},
		{"missing-domain@", false},
	}

	for _, tt := range tests {
		v := validator.New()
		v.Email("email", tt.email)
		if tt.isValid && v.HasErrors() {
			t.Errorf("expected %s to be valid, got error: %v", tt.email, v.Errors)
		}
		if !tt.isValid && !v.HasErrors() {
			t.Errorf("expected %s to be invalid", tt.email)
		}
	}
}

func TestValidator_OneOf(t *testing.T) {
	v := validator.New()
	v.OneOf("role", "admin", "admin", "teacher", "student")
	if v.HasErrors() {
		t.Fatal("expected role admin to be valid")
	}

	v2 := validator.New()
	v2.OneOf("role", "superhero", "admin", "teacher", "student")
	if !v2.HasErrors() {
		t.Fatal("expected role superhero to fail validation")
	}
}
