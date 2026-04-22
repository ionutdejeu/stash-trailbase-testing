package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alash3al/stash/internal/actions"
	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func listCmd(ctx context.Context, cmd *cli.Command) error {
	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	limit := cmd.Int("limit")
	if limit <= 0 {
		limit = 20 // Default from action
	}

	output, err := actions.ListEvents(ctx, bc, actions.ListEventsInput{
		Limit: limit,
	})
	if err != nil {
		return err
	}

	jsonOutput, err := json.Marshal(output)
	if err != nil {
		return fmt.Errorf("failed to marshal response: %w", err)
	}
	
	fmt.Println(string(jsonOutput))
	return nil
}