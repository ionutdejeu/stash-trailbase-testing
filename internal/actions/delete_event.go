package actions

import (
	"context"

	"github.com/alash3al/stash/internal/bootstrap"
)

// DeleteEventInput defines the input for deleting an event.
type DeleteEventInput struct {
	ID string `json:"id"`
}

// DeleteEventOutput defines the output after deleting an event.
type DeleteEventOutput struct {
	Success bool   `json:"success"`
	Deleted int    `json:"deleted"`
	ID      string `json:"id"`
}

// DeleteEvent soft-deletes an event and returns success status.
func DeleteEvent(ctx context.Context, c *bootstrap.Context, input DeleteEventInput) (DeleteEventOutput, error) {
	if input.ID == "" {
		return DeleteEventOutput{}, ErrInvalidID
	}

	err := c.Store.Delete(ctx, input.ID)
	if err != nil {
		return DeleteEventOutput{}, err
	}

	return DeleteEventOutput{
		Success: true,
		Deleted: 1,
		ID:      input.ID,
	}, nil
}