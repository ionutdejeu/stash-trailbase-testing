package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/alash3al/stash/internal/brain"
	"github.com/alash3al/stash/internal/brain/store"
	"github.com/alash3al/stash/internal/brain/store/postgres"
	"github.com/alash3al/stash/internal/config"
	"github.com/alash3al/stash/internal/embedder"
	"github.com/alash3al/stash/internal/reasoner"
)

type Context struct {
	Config *config.Config
	Brain  *brain.Brain
	Logger *slog.Logger
}

func MustNew(ctx context.Context) *Context {
	bootstrapCtx, err := New(ctx)
	if err != nil {
		panic(fmt.Sprintf("bootstrap failed: %v", err))
	}
	return bootstrapCtx
}

func New(ctx context.Context) (*Context, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

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
		return nil, fmt.Errorf("unknown log level: %q", cfg.LogLevel)
	}
	opts.Level = lvl

	if cfg.LogFormat == "json" {
		h = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		h = slog.NewTextHandler(os.Stdout, opts)
	}
	logger := slog.New(h)

	str, err := buildStore(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("build store: %w", err)
	}

	emb, err := buildEmbedder(cfg)
	if err != nil {
		str.Close()
		return nil, fmt.Errorf("build embedder: %w", err)
	}

	if emb.Dims() != cfg.VectorDim {
		str.Close()
		return nil, fmt.Errorf("vector dimension mismatch: embedder returns %d, config expects %d", emb.Dims(), cfg.VectorDim)
	}

	// Wrap embedder with store-backed cache
	// Uses namespace "_cache:embeddings" - no interface changes to store
	emb = embedder.NewCached(
		emb,
		// getRecord: try to get cached embedding
		func(ctx context.Context, id string) (map[string][]float32, error) {
			record, err := str.Get(ctx, id)
			if err != nil {
				return nil, err
			}
			// Extract just the vectors
			vectors := make(map[string][]float32)
			for model, vec := range record.Vectors {
				vectors[model] = vec.Values
			}
			return vectors, nil
		},
		// putRecord: store embedding in cache namespace
		func(ctx context.Context, id string, text string, vector []float32, model string) error {
			return str.Put(ctx, store.Record{
				ID:        id,
				Namespace: "_cache:embeddings",
				Content:   text,
				Vectors: map[string]store.Vector{
					model: {
						Values: vector,
						Model:  model,
					},
				},
				Metadata: map[string]any{
					"_memory": map[string]any{
						"type": "cache",
					},
				},
			})
		},
	)

	reas, err := buildReasoner(cfg)
	if err != nil {
		str.Close()
		return nil, fmt.Errorf("build reasoner: %w", err)
	}

	// Parse consolidation window duration
	consolidationWindow, err := time.ParseDuration(cfg.ConsolidationWindow)
	if err != nil {
		str.Close()
		return nil, fmt.Errorf("parse consolidation window: %w", err)
	}

	br, err := brain.NewWithConfig(str, emb, reas, brain.ConsolidationConfig{
		BatchSize:          cfg.ConsolidationBatchSize,
		MaxLLMCallsPerHour: cfg.ConsolidationMaxLLMCallsPerHour,
		SimilarityThreshold: cfg.ConsolidationSimilarityThreshold,
		Window:             consolidationWindow,
	})
	if err != nil {
		str.Close()
		return nil, fmt.Errorf("build brain: %w", err)
	}

	return &Context{
		Config: cfg,
		Brain:  br,
		Logger: logger,
	}, nil
}

func (c *Context) Close() error {
	var errs []string
	if c.Brain != nil {
		if err := c.Brain.Close(); err != nil {
			errs = append(errs, fmt.Sprintf("brain: %v", err))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("close errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func loadConfig() (*config.Config, error) {
	filename := os.Getenv("STASHCONFIG")
	if filename == "" {
		filename = ".env"
	}
	return config.NewFromFile(filename)
}

func buildStore(ctx context.Context, cfg *config.Config) (store.Store, error) {
	pgCfg := postgres.Config{
		DSN:             cfg.StoreDSN,
		VectorDim:       cfg.VectorDim,
		IndexedMetadata: []string{}, // TODO: make configurable
		MaxResultSize:   cfg.MaxResultSize,
	}
	return postgres.New(pgCfg)
}

func buildEmbedder(cfg *config.Config) (embedder.Embedder, error) {
	return embedder.NewOpenAI(
		cfg.OpenAIBaseURL,
		cfg.OpenAIAPIKey,
		cfg.EmbeddingModel,
		cfg.VectorDim,
	)
}

func buildReasoner(cfg *config.Config) (reasoner.Reasoner, error) {
	return reasoner.NewOpenAI(
		cfg.OpenAIBaseURL,
		cfg.OpenAIAPIKey,
		cfg.ReasonerModel,
	)
}
