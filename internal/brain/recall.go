package brain

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ionutdejeu/stash-trailbase-testing/internal/vector"
)

// RecallResult is a unified result from semantic search across episodes and facts.
type RecallResult struct {
	ID          int64   `json:"id"`
	NamespaceID int64   `json:"namespace_id"`
	Content     string  `json:"content"`
	Confidence  float32 `json:"confidence,omitempty"`
	Score       float32 `json:"score"`
	Type        string  `json:"type"`
	OccurredAt  string  `json:"occurred_at,omitempty"`
	ValidFrom   string  `json:"valid_from,omitempty"`
	CreatedAt   string  `json:"created_at"`
}

// Recall searches episodes and facts by semantic similarity across the given namespaces.
// Each namespace path matches itself and all descendants. Namespaces is required.
func (b *Brain) Recall(ctx context.Context, namespaces []string, query string, limit int) ([]RecallResult, error) {
	if err := validateContent(query); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		limit = 100
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed: %w", err)
	}

	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaces)
	if err != nil {
		return nil, err
	}

	factSQL, factArgs, err := b.queries.RecallFacts(nsIDs)
	if err != nil {
		return nil, fmt.Errorf("build fact query: %w", err)
	}

	factRows, err := b.pool.QueryContext(ctx, factSQL, factArgs...)
	if err != nil {
		return nil, fmt.Errorf("query facts: %w", err)
	}
	defer factRows.Close()

	var results []RecallResult
	for factRows.Next() {
		var id int64
		var namespaceID int64
		var content string
		var embedding vector.Vector
		var confidence float32
		var createdAtRaw any

		if err := factRows.Scan(&id, &namespaceID, &content, &embedding, &confidence, &createdAtRaw); err != nil {
			return nil, fmt.Errorf("scan fact: %w", err)
		}
		createdAt, err := parseSQLiteTime(createdAtRaw)
		if err != nil {
			return nil, fmt.Errorf("scan fact created_at: %w", err)
		}
		score := vector.CosineSimilarity(embedding.Slice(), vec)
		results = append(results, RecallResult{
			ID:          id,
			NamespaceID: namespaceID,
			Content:     content,
			Confidence:  confidence,
			Score:       score,
			Type:        "fact",
			CreatedAt:   createdAt.Format(time.RFC3339),
		})
	}
	if err := factRows.Err(); err != nil {
		return nil, fmt.Errorf("fact rows: %w", err)
	}

	epSQL, epArgs, err := b.queries.RecallEpisodes(nsIDs)
	if err != nil {
		return nil, fmt.Errorf("build episode query: %w", err)
	}

	epRows, err := b.pool.QueryContext(ctx, epSQL, epArgs...)
	if err != nil {
		return nil, fmt.Errorf("query episodes: %w", err)
	}
	defer epRows.Close()

	for epRows.Next() {
		var id int64
		var namespaceID int64
		var content string
		var embedding vector.Vector
		var occurredAtRaw any
		var createdAtRaw any

		if err := epRows.Scan(&id, &namespaceID, &content, &embedding, &occurredAtRaw, &createdAtRaw); err != nil {
			return nil, fmt.Errorf("scan episode: %w", err)
		}
		occurredAt, err := parseSQLiteTime(occurredAtRaw)
		if err != nil {
			return nil, fmt.Errorf("scan episode occurred_at: %w", err)
		}
		createdAt, err := parseSQLiteTime(createdAtRaw)
		if err != nil {
			return nil, fmt.Errorf("scan episode created_at: %w", err)
		}
		results = append(results, RecallResult{
			ID:          id,
			NamespaceID: namespaceID,
			Content:     content,
			Score:       vector.CosineSimilarity(embedding.Slice(), vec),
			Type:        "episode",
			OccurredAt:  occurredAt.Format(time.RFC3339),
			CreatedAt:   createdAt.Format(time.RFC3339),
		})
	}
	if err := epRows.Err(); err != nil {
		return nil, fmt.Errorf("episode rows: %w", err)
	}

	// Sort all results by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if len(results) > limit {
		results = results[:limit]
	}

	return results, nil
}
