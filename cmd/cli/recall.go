package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alash3al/stash/internal/actions"
	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func recallCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("query argument is required")
	}

	query := args.First()
	if strings.TrimSpace(query) == "" {
		return fmt.Errorf("query cannot be empty")
	}

	limit := cmd.Int("limit")
	if limit <= 0 {
		limit = 10 // Default from action
	}

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	output, err := actions.SearchEvents(ctx, bc, actions.SearchEventsInput{
		Query: query,
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
