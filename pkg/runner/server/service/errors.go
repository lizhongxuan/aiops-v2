package service

import "errors"

var (
	ErrNotFound      = errors.New("not found")
	ErrAlreadyExists = errors.New("already exists")
	ErrConflict      = errors.New("conflict")
	ErrInvalid       = errors.New("invalid request")
	ErrQueueFull     = errors.New("run queue full")
	ErrUnavailable   = errors.New("service unavailable")
)
