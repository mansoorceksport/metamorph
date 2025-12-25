package domain

import "errors"

// Common errors
var (
	ErrNotFound  = errors.New("record not found")
	ErrForbidden = errors.New("access forbidden: you don't own this resource")
)
