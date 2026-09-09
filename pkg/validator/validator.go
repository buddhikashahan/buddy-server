package validator

import (
	"regexp"
	"strings"

	"buddy/server/internal/domain"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}$`)

// Validator accumulates field-level validation errors.
type Validator struct {
	Errors map[string]string
}

// New creates a new Validator instance.
func New() *Validator {
	return &Validator{Errors: make(map[string]string)}
}

// AddError records an error for a given field if not already present.
func (v *Validator) AddError(field, message string) {
	if _, exists := v.Errors[field]; !exists {
		v.Errors[field] = message
	}
}

// Check evaluates a condition and registers an error if condition is false.
func (v *Validator) Check(ok bool, field, message string) {
	if !ok {
		v.AddError(field, message)
	}
}

// HasErrors returns true if validation failed.
func (v *Validator) HasErrors() bool {
	return len(v.Errors) > 0
}

// Error returns a domain.ValidationError or nil.
func (v *Validator) Error() error {
	if !v.HasErrors() {
		return nil
	}
	return &domain.ValidationError{Fields: v.Errors}
}

// Required verifies that a string is not empty.
func (v *Validator) Required(field, val string) {
	v.Check(strings.TrimSpace(val) != "", field, "this field is required")
}

// Email verifies valid email syntax.
func (v *Validator) Email(field, val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	v.Check(emailRegex.MatchString(val), field, "invalid email format")
}

// MinLength verifies minimum string length.
func (v *Validator) MinLength(field, val string, min int) {
	if strings.TrimSpace(val) == "" {
		return
	}
	v.Check(len(val) >= min, field, "must be at least minimum length")
}

// OneOf verifies value is in allowed list.
func (v *Validator) OneOf(field, val string, allowed ...string) {
	for _, a := range allowed {
		if val == a {
			return
		}
	}
	v.AddError(field, "invalid option specified")
}

