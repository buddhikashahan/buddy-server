package domain

import (
	"errors"
	"fmt"
	"strings"
)

var (
	ErrNotFound          = errors.New("resource not found")
	ErrUserAlreadyExists = errors.New("user with this email already exists")
	ErrUnauthorized      = errors.New("unauthorized: missing or invalid authentication token")
	ErrForbidden         = errors.New("forbidden: insufficient permissions")
	ErrInvalidInput      = errors.New("invalid input provided")
	ErrUserSuspended     = errors.New("user account is suspended or inactive")
	ErrInternal          = errors.New("an internal server error occurred")
)

// ValidationError holds field-level validation errors.
type ValidationError struct {
	Fields map[string]string
}

func (v *ValidationError) Error() string {
	var msgs []string
	for field, msg := range v.Fields {
		msgs = append(msgs, fmt.Sprintf("%s: %s", field, msg))
	}
	return strings.Join(msgs, ", ")
}

