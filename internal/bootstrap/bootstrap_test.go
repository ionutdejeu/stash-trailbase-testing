package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestNew_MapDBFake(t *testing.T) {
	content := `
STASH_STORE_DRIVER=mapdb
STASH_STORE_DSN=memory://
STASH_VECTOR_DIM=8
STASH_MAX_RESULT_SIZE=1000
STASH_EMBEDDER_DRIVER=fake
STASH_OPENAI_API_KEY=test-key
STASH_OPENAI_BASE_URL=https://api.openai.com/v1
STASH_EMBEDDING_MODEL=fake-model
STASH_FRAME_TTL=1h
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
STASH_LOG_FORMAT=text
`
	cleanup := setupTestEnv(t, content)
	defer cleanup()

	ctx := context.Background()
	bootstrapCtx, err := New(ctx)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}
	defer bootstrapCtx.Close()

	if bootstrapCtx.Config == nil {
		t.Error("Config is nil")
	}
	if bootstrapCtx.Store == nil {
		t.Error("Store is nil")
	}
	if bootstrapCtx.Embedder == nil {
		t.Error("Embedder is nil")
	}
	if bootstrapCtx.Memory == nil {
		t.Error("Memory is nil")
	}
	if bootstrapCtx.Logger == nil {
		t.Error("Logger is nil")
	}

	if bootstrapCtx.Config.StoreDriver != "mapdb" {
		t.Errorf("StoreDriver = %q, want %q", bootstrapCtx.Config.StoreDriver, "mapdb")
	}
	if bootstrapCtx.Config.EmbedderDriver != "fake" {
		t.Errorf("EmbedderDriver = %q, want %q", bootstrapCtx.Config.EmbedderDriver, "fake")
	}
}

func TestNew_MissingRequiredEnv(t *testing.T) {
	content := `
STASH_STORE_DRIVER=mapdb
STASH_STORE_DSN=memory://
STASH_VECTOR_DIM=8
STASH_MAX_RESULT_SIZE=1000
STASH_EMBEDDER_DRIVER=fake
STASH_OPENAI_API_KEY=test-key
STASH_OPENAI_BASE_URL=https://api.openai.com/v1
STASH_EMBEDDING_MODEL=fake-model
STASH_FRAME_TTL=1h
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
# Missing STASH_LOG_FORMAT
`
	cleanup := setupTestEnv(t, content)
	defer cleanup()

	ctx := context.Background()
	_, err := New(ctx)
	if err == nil {
		t.Error("Expected error for missing required env var, got nil")
	}
}

func TestNew_DimensionMismatch(t *testing.T) {
	content := `
STASH_STORE_DRIVER=mapdb
STASH_STORE_DSN=memory://
STASH_VECTOR_DIM=1536
STASH_MAX_RESULT_SIZE=1000
STASH_EMBEDDER_DRIVER=fake
STASH_OPENAI_API_KEY=test-key
STASH_OPENAI_BASE_URL=https://api.openai.com/v1
STASH_EMBEDDING_MODEL=fake-model
STASH_FRAME_TTL=1h
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
STASH_LOG_FORMAT=text
`
	cleanup := setupTestEnv(t, content)
	defer cleanup()

	ctx := context.Background()
	_, err := New(ctx)
	if err == nil {
		t.Error("Expected error for dimension mismatch, got nil")
	} else if !contains(err.Error(), "fake embedder only supports 8 dimensions") {
		t.Errorf("Expected dimension mismatch error, got: %v", err)
	}
}

func TestNew_UnknownStoreDriver(t *testing.T) {
	content := `
STASH_STORE_DRIVER=unknown
STASH_STORE_DSN=memory://
STASH_VECTOR_DIM=8
STASH_MAX_RESULT_SIZE=1000
STASH_EMBEDDER_DRIVER=fake
STASH_OPENAI_API_KEY=test-key
STASH_OPENAI_BASE_URL=https://api.openai.com/v1
STASH_EMBEDDING_MODEL=fake-model
STASH_FRAME_TTL=1h
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
STASH_LOG_FORMAT=text
`
	cleanup := setupTestEnv(t, content)
	defer cleanup()

	ctx := context.Background()
	_, err := New(ctx)
	if err == nil {
		t.Error("Expected error for unknown store driver, got nil")
	} else if !contains(err.Error(), "unknown store driver") {
		t.Errorf("Expected unknown store driver error, got: %v", err)
	}
}

func TestNew_UnknownEmbedderDriver(t *testing.T) {
	content := `
STASH_STORE_DRIVER=mapdb
STASH_STORE_DSN=memory://
STASH_VECTOR_DIM=8
STASH_MAX_RESULT_SIZE=1000
STASH_EMBEDDER_DRIVER=unknown
STASH_OPENAI_API_KEY=test-key
STASH_OPENAI_BASE_URL=https://api.openai.com/v1
STASH_EMBEDDING_MODEL=fake-model
STASH_FRAME_TTL=1h
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
STASH_LOG_FORMAT=text
`
	cleanup := setupTestEnv(t, content)
	defer cleanup()

	ctx := context.Background()
	_, err := New(ctx)
	if err == nil {
		t.Error("Expected error for unknown embedder driver, got nil")
	} else if !contains(err.Error(), "unknown embedder driver") {
		t.Errorf("Expected unknown embedder driver error, got: %v", err)
	}
}

func TestMustNew_PanicsOnError(t *testing.T) {
	content := `
STASH_STORE_DRIVER=mapdb
STASH_STORE_DSN=memory://
STASH_VECTOR_DIM=8
STASH_MAX_RESULT_SIZE=1000
STASH_EMBEDDER_DRIVER=fake
STASH_OPENAI_API_KEY=test-key
STASH_OPENAI_BASE_URL=https://api.openai.com/v1
STASH_EMBEDDING_MODEL=fake-model
STASH_FRAME_TTL=1h
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
# Missing STASH_LOG_FORMAT
`
	cleanup := setupTestEnv(t, content)
	defer cleanup()

	ctx := context.Background()
	defer func() {
		if r := recover(); r == nil {
			t.Error("Expected MustNew to panic on error, but it didn't")
		} else if !contains(r.(string), "bootstrap failed") {
			t.Errorf("Expected panic with 'bootstrap failed', got: %v", r)
		}
	}()
	
	_ = MustNew(ctx)
}

func TestClose_CleansUpResources(t *testing.T) {
	content := `
STASH_STORE_DRIVER=mapdb
STASH_STORE_DSN=memory://
STASH_VECTOR_DIM=8
STASH_MAX_RESULT_SIZE=1000
STASH_EMBEDDER_DRIVER=fake
STASH_OPENAI_API_KEY=test-key
STASH_OPENAI_BASE_URL=https://api.openai.com/v1
STASH_EMBEDDING_MODEL=fake-model
STASH_FRAME_TTL=1h
STASH_HTTP_ADDR=:8080
STASH_LOG_LEVEL=info
STASH_LOG_FORMAT=text
`
	cleanup := setupTestEnv(t, content)
	defer cleanup()

	ctx := context.Background()
	bootstrapCtx, err := New(ctx)
	if err != nil {
		t.Fatalf("New failed: %v", err)
	}

	err = bootstrapCtx.Close()
	if err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || contains(s[1:], substr)))
}

func backupEnvVars() map[string]string {
	vars := []string{
		"STASH_STORE_DRIVER",
		"STASH_STORE_DSN",
		"STASH_VECTOR_DIM",
		"STASH_MAX_RESULT_SIZE",
		"STASH_EMBEDDER_DRIVER",
		"STASH_OPENAI_API_KEY",
		"STASH_OPENAI_BASE_URL",
		"STASH_EMBEDDING_MODEL",
		"STASH_FRAME_TTL",
		"STASH_HTTP_ADDR",
		"STASH_LOG_LEVEL",
		"STASH_LOG_FORMAT",
	}
	backup := make(map[string]string)
	for _, v := range vars {
		if val, ok := os.LookupEnv(v); ok {
			backup[v] = val
			os.Unsetenv(v)
		}
	}
	return backup
}

func restoreEnvVars(backup map[string]string) {
	for k, v := range backup {
		os.Setenv(k, v)
	}
}

func setupTestEnv(t *testing.T, content string) (cleanup func()) {
	tmpDir := t.TempDir()
	envFile := filepath.Join(tmpDir, ".env")
	if err := os.WriteFile(envFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	oldEnv := os.Getenv("STASHCONFIG")
	os.Setenv("STASHCONFIG", envFile)
	
	oldEnvVars := backupEnvVars()

	return func() {
		restoreEnvVars(oldEnvVars)
		if oldEnv == "" {
			os.Unsetenv("STASHCONFIG")
		} else {
			os.Setenv("STASHCONFIG", oldEnv)
		}
	}
}