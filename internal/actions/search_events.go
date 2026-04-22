package actions

import (
	"context"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/alash3al/stash/internal/memory"
)

// SearchEventsInput defines the input for searching events.
type SearchEventsInput struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
}

// SearchEventsOutput defines the output after searching events.
type SearchEventsOutput struct {
	Events []SearchEventItem `json:"events"`
}

// SearchEventItem represents a single event in search results.
type SearchEventItem struct {
	EventItem
	Score float32 `json:"score,omitempty"`
}

// SearchEvents searches for events relevant to a query and returns slim representations.
func SearchEvents(ctx context.Context, c *bootstrap.Context, input SearchEventsInput) (SearchEventsOutput, error) {
	if input.Query == "" {
		return SearchEventsOutput{}, memory.ErrEmptyContent
	}

	if input.Limit <= 0 {
		input.Limit = 10 // Default
	}

	events, err := c.Memory.Recall(ctx, input.Query, input.Limit)
	if err != nil {
		return SearchEventsOutput{}, err
	}

	output := SearchEventsOutput{
		Events: make([]SearchEventItem, 0, len(events)),
	}

	for _, event := range events {
		// Filter out _memory metadata
		userMetadata := make(map[string]any)
		for k, v := range event.Metadata {
			if k != "_memory" {
				userMetadata[k] = v
			}
		}

		output.Events = append(output.Events, SearchEventItem{
			EventItem: EventItem{
				ID:        event.ID,
				Content:   event.Content,
				Metadata:  userMetadata,
				Timestamp: event.Timestamp,
			},
			Score: event.Score,
		})
	}

	return output, nil
}