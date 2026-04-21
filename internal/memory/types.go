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
	Score     float32
}

// WorkingMemory represents working memory — what is actively being thought about.
// Single global working memory for MVP, stored with fixed ID "_memory.context".
// Stored as a store.Record with _memory.type = "context".
type WorkingMemory struct {
	ID        string
	Focus     string
	EventIDs  []string
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time
}
