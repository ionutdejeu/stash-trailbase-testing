// Package actions provides typed, reusable units of work for Stash operations.
// Each action is a pure function with typed input and output, providing clean contracts
// for both CLI and future API endpoints.
package actions

import (
	"errors"
	"time"
)

// EventItem contains common fields for event representations in action outputs.
type EventItem struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// Common errors
var (
	ErrInvalidID = errors.New("invalid ID")
)