package memory

import "errors"

var (
	ErrEventNotFound    = errors.New("memory: event not found")
	ErrContextExpired   = errors.New("memory: working context has expired")
	ErrInvalidMetadata  = errors.New("memory: caller metadata must not use _memory namespace")
	ErrEmptyContent     = errors.New("memory: content must not be empty")
	ErrInvalidLimit     = errors.New("memory: limit must be greater than zero")
)
