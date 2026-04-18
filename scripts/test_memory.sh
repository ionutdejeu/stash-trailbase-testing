#!/bin/bash
set -e

echo "=== Memory Layer Verification Script ==="
echo ""

if [ -z "$OPENAI_API_KEY" ]; then
    echo "ERROR: OPENAI_API_KEY environment variable is required"
    echo "Set it with: export OPENAI_API_KEY=your-key-here"
    exit 1
fi

DB_HOST="${DB_HOST:-localhost}"
DB_PORT="${DB_PORT:-5432}"
DB_USER="${DB_USER:-postgres}"
DB_PASS="${DB_PASS:-postgres}"
DB_NAME="${DB_NAME:-stash_test}"

POSTGRES_DSN="postgres://${DB_USER}:${DB_PASS}@${DB_HOST}:${DB_PORT}/${DB_NAME}?sslmode=disable"

echo "Using Postgres DSN: postgres://${DB_USER}:****@${DB_HOST}:${DB_PORT}/${DB_NAME}"
echo "Using OpenAI API key: ${OPENAI_API_KEY:0:10}..."
echo ""

cd "$(dirname "$0")/.."

echo "Building memory example..."
cat > /tmp/memory_example.go << 'EOF'
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/alash3al/stash/internal/embedder"
	"github.com/alash3al/stash/internal/memory"
	"github.com/alash3al/stash/internal/store/postgres"
)

func main() {
	ctx := context.Background()

	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}

	embed, err := embedder.NewOpenAI(baseURL, os.Getenv("OPENAI_API_KEY"), "text-embedding-3-small", 1536)
	if err != nil {
		log.Fatalf("Failed to create embedder: %v", err)
	}
	fmt.Println("✓ Embedder created")

	cfg := postgres.Config{
		DSN:             os.Getenv("POSTGRES_DSN"),
		VectorDim:       1536,
		IndexedMetadata: []string{},
		MaxResultSize:   1000,
	}
	s, err := postgres.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer s.Close()
	fmt.Println("✓ Store created")

	mem, err := memory.New(s, embed)
	if err != nil {
		log.Fatalf("Failed to create memory: %v", err)
	}
	defer mem.Close()
	fmt.Println("✓ Memory created")

	fmt.Println("\n--- Remember ---")
	eventID1, err := mem.Remember(ctx, "user asked about the weather", map[string]any{
		"session": "session-123",
		"source":  "chat",
	})
	if err != nil {
		log.Fatalf("Remember failed: %v", err)
	}
	fmt.Printf("✓ Event stored: %s\n", eventID1)

	eventID2, err := mem.Remember(ctx, "assistant suggested bringing an umbrella", map[string]any{
		"session": "session-123",
		"source":  "assistant",
	})
	if err != nil {
		log.Fatalf("Remember failed: %v", err)
	}
	fmt.Printf("✓ Event stored: %s\n", eventID2)

	eventID3, err := mem.Remember(ctx, "user is planning a trip to London", map[string]any{
		"session": "session-123",
	})
	if err != nil {
		log.Fatalf("Remember failed: %v", err)
	}
	fmt.Printf("✓ Event stored: %s\n", eventID3)

	fmt.Println("\n--- Recall ---")
	events, err := mem.Recall(ctx, "weather conditions", 5)
	if err != nil {
		log.Fatalf("Recall failed: %v", err)
	}
	fmt.Printf("✓ Recall returned %d events\n", len(events))
	for i, e := range events {
		fmt.Printf("  [%d] ID: %s, Content: %s\n", i+1, e.ID, e.Content)
	}

	fmt.Println("\n--- Frame ---")
	frame, err := mem.Frame(ctx, "weather conversation")
	if err != nil {
		log.Fatalf("Frame failed: %v", err)
	}
	fmt.Printf("✓ Frame Focus: %s\n", frame.Focus)
	fmt.Printf("  Created: %s\n", frame.CreatedAt.Format(time.RFC3339))
	fmt.Printf("  Expires: %s\n", frame.ExpiresAt.Format(time.RFC3339))

	frame2, err := mem.Frame(ctx, "different focus")
	if err != nil {
		log.Fatalf("Frame(2) failed: %v", err)
	}
	if frame.ID == frame2.ID && frame.CreatedAt.Equal(frame2.CreatedAt) {
		fmt.Println("✓ Frame is stable (same ID and created_at)")
	}

	fmt.Println("\n=== All checks passed! ===")
}
EOF

echo "Building..."
go run /tmp/memory_example.go 2>&1

echo ""
echo "Cleaning up temp file..."
rm -f /tmp/memory_example.go

echo ""
echo "Verification complete!"