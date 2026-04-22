package actions

import (
	"context"
	"time"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/alash3al/stash/internal/store"
)

// ListEventsInput defines the input for listing events.
type ListEventsInput struct {
	Limit int `json:"limit,omitempty"`
}

// ListEventsOutput defines the output after listing events.
type ListEventsOutput struct {
	Events []ListEventItem `json:"events"`
}

// ListEventItem represents a single event in list results.
type ListEventItem struct {
	EventItem
}

// ListEvents lists recent events and returns slim representations.
func ListEvents(ctx context.Context, c *bootstrap.Context, input ListEventsInput) (ListEventsOutput, error) {
	if input.Limit <= 0 {
		input.Limit = 20 // Default
	}

	records, err := c.Store.List(ctx, store.Filter{
		Limit: input.Limit,
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: "event",
		},
	})
	if err != nil {
		return ListEventsOutput{}, err
	}

	output := ListEventsOutput{
		Events: make([]ListEventItem, 0, len(records)),
	}

	for _, record := range records {
		// Extract timestamp from metadata
		var timestamp time.Time
		if memMeta, ok := record.Metadata["_memory"].(map[string]any); ok {
			if tsStr, ok := memMeta["timestamp"].(string); ok {
				if ts, err := time.Parse(time.RFC3339, tsStr); err == nil {
					timestamp = ts
				}
			}
		}

		// Fallback to CreatedAt
		if timestamp.IsZero() {
			timestamp = record.CreatedAt
		}

		// Filter out _memory metadata
		userMetadata := make(map[string]any)
		for k, v := range record.Metadata {
			if k != "_memory" {
				userMetadata[k] = v
			}
		}

		output.Events = append(output.Events, ListEventItem{
			EventItem: EventItem{
				ID:        record.ID,
				Content:   record.Content,
				Metadata:  userMetadata,
				Timestamp: timestamp,
			},
		})
	}

	return output, nil
}