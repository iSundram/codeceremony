package domain

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrUnauthorized  = errors.New("unauthorized")
	ErrForbidden     = errors.New("forbidden")
	ErrConflict      = errors.New("conflict")
	ErrValidation    = errors.New("validation failed")
	ErrAlreadyExists = errors.New("already exists")
)
