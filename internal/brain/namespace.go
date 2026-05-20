package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/ionutdejeu/stash-trailbase-testing/internal/models"
)

func scanNamespace(scanner rowScanner, ns *models.Namespace) error {
	var createdAtRaw any
	var updatedAtRaw any
	if err := scanner.Scan(&ns.ID, &ns.Slug, &ns.Name, &ns.Description, &createdAtRaw, &updatedAtRaw); err != nil {
		return err
	}
	var err error
	if ns.CreatedAt, err = parseSQLiteTime(createdAtRaw); err != nil {
		return fmt.Errorf("parse created_at: %w", err)
	}
	if ns.UpdatedAt, err = parseSQLiteTime(updatedAtRaw); err != nil {
		return fmt.Errorf("parse updated_at: %w", err)
	}
	return nil
}

// CreateNamespace creates a new namespace with the given slug, name, and description.
// Parent namespaces are auto-created with slug as name if they don't exist.
func (b *Brain) CreateNamespace(ctx context.Context, slug, name, description string) (int64, error) {
	if err := validatePath(slug); err != nil {
		return 0, err
	}

	segments := splitPath(slug)
	if len(segments) == 0 {
		var id int64
		err := b.pool.QueryRowContext(ctx,
			"INSERT INTO namespaces (slug, name) VALUES ('/', '/') ON CONFLICT (slug) DO UPDATE SET updated_at = CURRENT_TIMESTAMP RETURNING id",
		).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("create root namespace: %w", err)
		}
		return id, nil
	}

	currentPath := ""
	for i, seg := range segments {
		currentPath += "/" + seg
		if i < len(segments)-1 {
			var id int64
			_ = b.pool.QueryRowContext(ctx,
				"INSERT INTO namespaces (slug, name) VALUES ($1, $1) ON CONFLICT (slug) DO UPDATE SET updated_at = CURRENT_TIMESTAMP RETURNING id",
				currentPath,
			).Scan(&id)
		}
	}

	var id int64
	err := b.pool.QueryRowContext(ctx,
		"INSERT INTO namespaces (slug, name, description) VALUES ($1, $2, $3) ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, description = EXCLUDED.description, updated_at = CURRENT_TIMESTAMP RETURNING id",
		slug, name, description,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("create namespace: %w", err)
	}
	return id, nil
}

// GetNamespace returns a namespace by slug.
func (b *Brain) GetNamespace(ctx context.Context, slug string) (*models.Namespace, error) {
	var ns models.Namespace
	row := b.pool.QueryRowContext(ctx,
		"SELECT id, slug, name, description, created_at, updated_at FROM namespaces WHERE slug = $1",
		slug,
	)
	err := scanNamespace(row, &ns)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrNamespaceNotFound
		}
		return nil, fmt.Errorf("get namespace: %w", err)
	}
	return &ns, nil
}

// ListNamespaces returns namespaces, optionally filtered by slug paths.
// If slugs is empty, returns all namespaces.
// Each path matches itself and all descendants.
func (b *Brain) ListNamespaces(ctx context.Context, slugs []string, page Pagination) ([]models.Namespace, error) {
	page = page.Sanitize()

	if len(slugs) == 0 {
		rows, err := b.pool.QueryContext(ctx,
			"SELECT id, slug, name, description, created_at, updated_at FROM namespaces ORDER BY slug LIMIT $1 OFFSET $2",
			page.Limit, page.Offset,
		)
		if err != nil {
			return nil, fmt.Errorf("list namespaces: %w", err)
		}
		defer rows.Close()

		var result []models.Namespace
		for rows.Next() {
			var ns models.Namespace
			if err := scanNamespace(rows, &ns); err != nil {
				return nil, fmt.Errorf("scan namespace: %w", err)
			}
			result = append(result, ns)
		}
		return result, rows.Err()
	}

	ids, err := b.resolveNamespaceIDs(ctx, slugs)
	if err != nil {
		return nil, err
	}

	placeholders, args := inClause(1, ids)
	args = append(args, page.Limit, page.Offset)
	rows, err := b.pool.QueryContext(ctx,
		fmt.Sprintf("SELECT id, slug, name, description, created_at, updated_at FROM namespaces WHERE id IN (%s) ORDER BY slug LIMIT $%d OFFSET $%d", placeholders, len(ids)+1, len(ids)+2),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list namespaces: %w", err)
	}
	defer rows.Close()

	var result []models.Namespace
	for rows.Next() {
		var ns models.Namespace
		if err := scanNamespace(rows, &ns); err != nil {
			return nil, fmt.Errorf("scan namespace: %w", err)
		}
		result = append(result, ns)
	}
	return result, rows.Err()
}

// GetOrCreateConsolidationProgress returns progress for a namespace, creating a row if needed.
func (b *Brain) GetOrCreateConsolidationProgress(ctx context.Context, namespaceID int64) (*models.ConsolidationProgress, error) {
	var cp models.ConsolidationProgress
	var lastDecayRunRaw any
	var lastRunRaw any
	var updatedAtRaw any
	err := b.pool.QueryRowContext(ctx,
		`INSERT INTO consolidation_progress (namespace_id) VALUES ($1)
		 ON CONFLICT (namespace_id) DO UPDATE SET updated_at = consolidation_progress.updated_at
		 RETURNING namespace_id, last_episode_id, last_fact_id, last_relationship_id, last_pattern_fact_id, last_pattern_rel_id, last_goal_progress_fact_id, last_failure_id, last_failure_episode_id, last_hypothesis_fact_id, last_decay_run, last_run, updated_at`,
		namespaceID,
	).Scan(&cp.NamespaceID, &cp.LastEpisodeID, &cp.LastFactID, &cp.LastRelationshipID, &cp.LastPatternFactID, &cp.LastPatternRelID, &cp.LastGoalProgressFactID, &cp.LastFailureID, &cp.LastFailureEpisodeID, &cp.LastHypothesisFactID, &lastDecayRunRaw, &lastRunRaw, &updatedAtRaw)
	if err != nil {
		return nil, fmt.Errorf("get consolidation progress: %w", err)
	}
	if cp.LastDecayRun, err = parseOptionalSQLiteTime(lastDecayRunRaw); err != nil {
		return nil, fmt.Errorf("get consolidation progress: parse last_decay_run: %w", err)
	}
	if cp.LastRun, err = parseOptionalSQLiteTime(lastRunRaw); err != nil {
		return nil, fmt.Errorf("get consolidation progress: parse last_run: %w", err)
	}
	if cp.UpdatedAt, err = parseSQLiteTime(updatedAtRaw); err != nil {
		return nil, fmt.Errorf("get consolidation progress: parse updated_at: %w", err)
	}
	return &cp, nil
}

// SaveConsolidationProgress updates the checkpoint for a namespace.
func (b *Brain) SaveConsolidationProgress(ctx context.Context, cp models.ConsolidationProgress) error {
	now := time.Now().UTC()
	_, err := b.pool.ExecContext(ctx,
		`UPDATE consolidation_progress SET
			last_episode_id = $2, last_fact_id = $3, last_relationship_id = $4,
			last_pattern_fact_id = $5, last_pattern_rel_id = $6,
			last_goal_progress_fact_id = $7, last_failure_id = $8, last_failure_episode_id = $9, last_hypothesis_fact_id = $10,
			last_decay_run = $11, last_run = $12, updated_at = $13
		 WHERE namespace_id = $1`,
		cp.NamespaceID, cp.LastEpisodeID, cp.LastFactID, cp.LastRelationshipID,
		cp.LastPatternFactID, cp.LastPatternRelID,
		cp.LastGoalProgressFactID, cp.LastFailureID, cp.LastFailureEpisodeID, cp.LastHypothesisFactID,
		cp.LastDecayRun, now, now,
	)
	if err != nil {
		return fmt.Errorf("save consolidation progress: %w", err)
	}
	return nil
}
