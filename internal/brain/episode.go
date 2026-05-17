package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/alash3al/stash/internal/models"
	"github.com/alash3al/stash/internal/vector"
)

// Remember stores a new episode in the given namespace.
// If occurredAt is nil, the current time is used.
// Returns the episode ID on success.
func (b *Brain) Remember(ctx context.Context, namespaceSlug, content string, occurredAt *time.Time) (int64, error) {
	if err := validateContent(content); err != nil {
		return 0, err
	}
	if err := validatePath(namespaceSlug); err != nil {
		return 0, err
	}

	nsID, err := b.resolveNamespaceID(ctx, namespaceSlug)
	if err != nil {
		return 0, err
	}

	occurred := time.Now().UTC()
	if occurredAt != nil {
		occurred = *occurredAt
	}

	vec, err := b.embedder.Embed(ctx, content)
	if err != nil {
		return 0, fmt.Errorf("embed: %w", err)
	}

	var id int64
	err = b.pool.QueryRowContext(ctx,
		`INSERT INTO episodes (namespace_id, content, embedding, embedding_model, occurred_at)
		 VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		nsID, content, vector.New(vec), b.embedder.Model(), occurred,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("insert episode: %w", err)
	}
	return id, nil
}

// ForgetEpisode soft-deletes the episode that best matches the query across the given namespaces.
// If namespaceSlugs is empty, searches all namespaces.
func (b *Brain) ForgetEpisode(ctx context.Context, namespaceSlugs []string, query string) error {
	if err := validateContent(query); err != nil {
		return err
	}
	for _, slug := range namespaceSlugs {
		if err := validatePath(slug); err != nil {
			return err
		}
	}

	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return err
	}

	vec, err := b.embedder.Embed(ctx, query)
	if err != nil {
		return fmt.Errorf("embed: %w", err)
	}

	placeholders, args := inClause(1, nsIDs)
	rows, err := b.pool.QueryContext(ctx,
		fmt.Sprintf(`SELECT id, embedding FROM episodes
		 WHERE namespace_id IN (%s) AND deleted_at IS NULL AND embedding IS NOT NULL`, placeholders),
		args...,
	)
	if err != nil {
		return fmt.Errorf("query forget candidates: %w", err)
	}
	defer rows.Close()

	var id int64
	var bestScore float32 = -1
	for rows.Next() {
		var candidateID int64
		var embedding vector.Vector
		if err := rows.Scan(&candidateID, &embedding); err != nil {
			return fmt.Errorf("scan forget candidate: %w", err)
		}
		score := vector.CosineSimilarity(embedding.Slice(), vec)
		if score > bestScore {
			bestScore = score
			id = candidateID
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate forget candidates: %w", err)
	}
	if id == 0 {
		return ErrEpisodeNotFound
	}

	_, err = b.pool.ExecContext(ctx,
		"UPDATE episodes SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1", id,
	)
	if err != nil {
		return fmt.Errorf("soft delete episode: %w", err)
	}
	return nil
}

// PurgeEpisode hard-deletes an episode by ID.
func (b *Brain) PurgeEpisode(ctx context.Context, episodeID int64) error {
	tag, err := b.pool.ExecContext(ctx, "DELETE FROM episodes WHERE id = $1", episodeID)
	if err != nil {
		return fmt.Errorf("purge episode: %w", err)
	}
	affected, err := rowsAffected(tag)
	if err != nil {
		return fmt.Errorf("purge episode rows affected: %w", err)
	}
	if affected == 0 {
		return ErrEpisodeNotFound
	}
	return nil
}

// GetEpisode returns a single episode by ID.
func (b *Brain) GetEpisode(ctx context.Context, episodeID int64) (*models.Episode, error) {
	var e models.Episode
	err := b.pool.QueryRowContext(ctx,
		`SELECT id, namespace_id, content, embedding, embedding_model, occurred_at, created_at, deleted_at
		 FROM episodes WHERE id = $1`,
		episodeID,
	).Scan(&e.ID, &e.NamespaceID, &e.Content, &e.Embedding, &e.EmbeddingModel, &e.OccurredAt, &e.CreatedAt, &e.DeletedAt)
	if err != nil {
		return nil, ErrEpisodeNotFound
	}
	return &e, nil
}
