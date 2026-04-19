package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/alash3al/stash/internal/bootstrap"
	"github.com/alash3al/stash/internal/config"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/urfave/cli/v3"
)

func EnvCmd(ctx context.Context, cmd *cli.Command) error {
	configFile := cmd.String("file")

	// Use same logic as bootstrap to determine config file
	filename := os.Getenv("STASHCONFIG")
	if filename == "" {
		filename = configFile
	}

	// Load config
	cfg, err := config.NewFromFile(filename)
	if err != nil {
		return fmt.Errorf("error loading config: %w", err)
	}

	// Build logger from config
	var h slog.Handler
	opts := &slog.HandlerOptions{}

	lvl := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		lvl = slog.LevelDebug
	case "info":
		lvl = slog.LevelInfo
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		return fmt.Errorf("unknown log level: %q", cfg.LogLevel)
	}
	opts.Level = lvl

	if cfg.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	logger := slog.New(h)

	// Output config details using a table
	t := table.NewWriter()
	t.SetOutputMirror(os.Stdout)
	t.SetStyle(table.StyleLight)
	t.AppendHeader(table.Row{"Configuration Key", "Value"})
	t.AppendRows([]table.Row{
		{"ConfigFile", filename},
		{"STASHCONFIG Env", os.Getenv("STASHCONFIG")},
		{"Store Driver", cfg.StoreDriver},
		{"Store DSN", MaskDSN(cfg.StoreDSN)},
		{"Vector Dimension", cfg.VectorDim},
		{"Max Result Size", cfg.MaxResultSize},
		{"Embedder Driver", cfg.EmbedderDriver},
		{"OpenAI API Key", MaskAPIKey(cfg.OpenAIAPIKey)},
		{"OpenAI Base URL", cfg.OpenAIBaseURL},
		{"Embedding Model", cfg.EmbeddingModel},
		{"Frame TTL", cfg.FrameTTL},
		{"HTTP Addr", cfg.HTTPAddr},
		{"Log Level", cfg.LogLevel},
		{"Log Format", cfg.LogFormat},
	})
	fmt.Println("=== Stash Configuration ===")
	t.Render()
	fmt.Println()

	// Attempt bootstrap
	logger.Info("Attempting bootstrap...")
	bootstrapCtx, err := bootstrap.New(ctx)
	if err != nil {
		logger.Error("Bootstrap failed", slog.Any("error", err))
		return fmt.Errorf("bootstrap failed: %w", err)
	}
	defer bootstrapCtx.Close()

	logger.Info("Bootstrap successful",
		slog.Bool("StoreInitialized", bootstrapCtx.Store != nil),
		slog.Bool("EmbedderInitialized", bootstrapCtx.Embedder != nil),
		slog.Bool("MemoryInitialized", bootstrapCtx.Memory != nil),
	)

	return nil
}

func MaskDSN(dsn string) string {
	if len(dsn) > 50 {
		return dsn[:20] + "..." + dsn[len(dsn)-20:]
	}
	return dsn
}

func MaskAPIKey(key string) string {
	if len(key) < 8 {
		return "***"
	}
	return key[:4] + "..." + key[len(key)-4:]
}
