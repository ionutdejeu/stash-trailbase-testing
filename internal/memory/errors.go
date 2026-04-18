package memory

import "errors"

var (
	ErrEventNotFound   = errors.New("memory: event not found")
	ErrFrameExpired    = errors.New("memory: working frame has expired")
	ErrInvalidMetadata = errors.New("memory: caller metadata must not use _memory namespace")
	ErrEmptyContent    = errors.New("memory: content must not be empty")
	ErrInvalidLimit    = errors.New("memory: limit must be greater than zero")
)
