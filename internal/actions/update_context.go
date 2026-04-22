package actions

import (
	"context"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/alash3al/stash/internal/memory"
)

// UpdateContextInput defines the input for updating context.
type UpdateContextInput struct {
	Focus string `json:"focus"`
}

// UpdateContextOutput defines the output after updating context.
type UpdateContextOutput struct {
	Success bool   `json:"success"`
	Focus   string `json:"focus"`
	ID      string `json:"id"`
}

// UpdateContext updates the working memory context focus.
func UpdateContext(ctx context.Context, c *bootstrap.Context, input UpdateContextInput) (UpdateContextOutput, error) {
	if input.Focus == "" {
		return UpdateContextOutput{}, memory.ErrEmptyContent
	}

	wm, err := c.Memory.WorkingMemory(ctx, input.Focus)
	if err != nil {
		return UpdateContextOutput{}, err
	}

	return UpdateContextOutput{
		Success: true,
		Focus:   wm.Focus,
		ID:      wm.ID,
	}, nil
}