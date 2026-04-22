package actions

import (
	"context"

	"github.com/alash3al/stash/internal/bootstrap"
)

// PurgeEventInput defines the input for purging an event.
type PurgeEventInput struct {
	ID string `json:"id"`
}

// PurgeEventOutput defines the output after purging an event.
type PurgeEventOutput struct {
	Success bool   `json:"success"`
	Purged  int    `json:"purged"`
	ID      string `json:"id"`
}

// PurgeEvent hard-deletes an event and returns success status.
func PurgeEvent(ctx context.Context, c *bootstrap.Context, input PurgeEventInput) (PurgeEventOutput, error) {
	if input.ID == "" {
		return PurgeEventOutput{}, ErrInvalidID
	}

	err := c.Store.Purge(ctx, input.ID)
	if err != nil {
		return PurgeEventOutput{}, err
	}

	return PurgeEventOutput{
		Success: true,
		Purged:  1,
		ID:      input.ID,
	}, nil
}