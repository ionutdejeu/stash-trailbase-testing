package actions

import (
	"context"
	"time"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/alash3al/stash/internal/memory"
)

// CreateEventInput defines the input for creating an event.
type CreateEventInput struct {
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// CreateEventOutput defines the output after creating an event.
type CreateEventOutput struct {
	ID        string         `json:"id"`
	Content   string         `json:"content"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

// CreateEvent stores an event and returns its slim representation.
func CreateEvent(ctx context.Context, c *bootstrap.Context, input CreateEventInput) (CreateEventOutput, error) {
	if input.Content == "" {
		return CreateEventOutput{}, memory.ErrEmptyContent
	}

	eventID, err := c.Memory.Remember(ctx, input.Content, input.Metadata)
	if err != nil {
		return CreateEventOutput{}, err
	}

	// Fetch the created record to get the actual timestamp
	record, err := c.Store.Get(ctx, eventID)
	if err != nil {
		return CreateEventOutput{}, err
	}

	// Extract timestamp from metadata
	var timestamp time.Time
	if memMeta, ok := record.Metadata["_memory"].(map[string]any); ok {
		if tsStr, ok := memMeta["timestamp"].(string); ok {
			if ts, err := time.Parse(time.RFC3339, tsStr); err == nil {
				timestamp = ts
			}
		}
	}

	// Fallback to CreatedAt if timestamp not found in metadata
	if timestamp.IsZero() {
		timestamp = record.CreatedAt
	}

	// Filter out _memory metadata from response
	userMetadata := make(map[string]any)
	for k, v := range record.Metadata {
		if k != "_memory" {
			userMetadata[k] = v
		}
	}

	return CreateEventOutput{
		ID:        eventID,
		Content:   input.Content,
		Metadata:  userMetadata,
		Timestamp: timestamp,
	}, nil
}