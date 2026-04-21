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

func rememberCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("content argument is required")
	}

	content := args.First()
	if strings.TrimSpace(content) == "" {
		return memory.ErrEmptyContent
	}

	var metadata map[string]any
	if metadataFlag := cmd.String("metadata"); metadataFlag != "" {
		if err := json.Unmarshal([]byte(metadataFlag), &metadata); err != nil {
			return fmt.Errorf("invalid metadata JSON: %w", err)
		}
	}

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	eventID, err := bc.Memory.Remember(ctx, content, metadata)
	if err != nil {
		return err
	}

	fmt.Printf("Event stored: %s\n", eventID)
	return nil
}
