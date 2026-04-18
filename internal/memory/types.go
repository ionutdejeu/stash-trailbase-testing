package memory

import (
	"time"
)

// Event represents something that happened at a specific point in time.
// Stored as a store.Record with _memory.type = "event".
type Event struct {
	ID        string
	Content   string
	Timestamp time.Time
	Metadata  map[string]any
}

// Frame represents working memory — what is actively being thought about.
// Single global frame for MVP, stored with fixed ID "_memory.working_frame".
// Stored as a store.Record with _memory.type = "frame".
type Frame struct {
	ID        string
	Focus     string
	EventIDs  []string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time
}
