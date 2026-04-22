package actions

import (
	"context"
	"time"

	"github.com/alash3al/stash/internal/bootstrap"
)

// ShowContextInput defines the input for showing context.
type ShowContextInput struct {
	// Empty for now, could include options in the future
}

// ShowContextOutput defines the output after showing context.
type ShowContextOutput struct {
	ID        string    `json:"id"`
	Focus     string    `json:"focus"`
	EventIDs  []string  `json:"event_ids"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	ExpiresAt time.Time `json:"expires_at"`
	ExpiresIn string    `json:"expires_in"`
}

// ShowContext shows the current working memory context.
func ShowContext(ctx context.Context, c *bootstrap.Context, input ShowContextInput) (ShowContextOutput, error) {
	wm, err := c.Memory.WorkingMemory(ctx, "")
	if err != nil {
		return ShowContextOutput{}, err
	}

	expiresIn := time.Until(wm.ExpiresAt).Round(time.Second).String()

	return ShowContextOutput{
		ID:        wm.ID,
		Focus:     wm.Focus,
		EventIDs:  wm.EventIDs,
		CreatedAt: wm.CreatedAt,
		UpdatedAt: wm.UpdatedAt,
		ExpiresAt: wm.ExpiresAt,
		ExpiresIn: expiresIn,
	}, nil
}