package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/alash3al/stash/internal/actions"
	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/urfave/cli/v3"
)

func contextShowCmd(ctx context.Context, cmd *cli.Command) error {
	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	output, err := actions.ShowContext(ctx, bc, actions.ShowContextInput{})
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

func contextUpdateCmd(ctx context.Context, cmd *cli.Command) error {
	args := cmd.Args()
	if args.Len() == 0 {
		return fmt.Errorf("focus argument is required")
	}

	focus := args.First()

	bc, ok := cmd.Root().Metadata["bootstrapCtx"].(*bootstrap.Context)
	if !ok {
		return fmt.Errorf("bootstrap context not available")
	}

	output, err := actions.UpdateContext(ctx, bc, actions.UpdateContextInput{
		Focus: focus,
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
