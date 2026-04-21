package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/alash3al/stash/internal/memory"
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
		return memory.ErrInvalidLimit
	}

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	events, err := bc.Memory.Recall(ctx, query, limit)
	if err != nil {
		return err
	}

	if cmd.Bool("json") {
		output, err := json.MarshalIndent(events, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal events to JSON: %w", err)
		}
		fmt.Println(string(output))
		return nil
	}

	if len(events) == 0 {
		fmt.Println("No events found.")
		return nil
	}

	for _, event := range events {
		// Format the timestamp nicely
		timestamp := event.Timestamp.Format("2006-01-02 15:04:05")

		// Truncate content for display
		content := event.Content
		if len(content) > 80 {
			content = content[:77] + "..."
		}

		fmt.Printf("• %s (score: %.2f)\n", content, event.Score)
		fmt.Printf("  ID: %s | Time: %s\n", event.ID, timestamp)

		// Show metadata if present
		if len(event.Metadata) > 0 {
			metadataStr, _ := json.Marshal(event.Metadata)
			fmt.Printf("  Metadata: %s\n", metadataStr)
		}
		fmt.Println()
	}

	return nil
}
