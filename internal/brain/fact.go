package brain

import (
	"context"
	"fmt"
	"time"

	"github.com/alash3al/stash/internal/models"
)

// QueryFacts returns facts across namespaces matching the given slug paths, within an optional time range.
// Each path matches itself and all descendants.
func (b *Brain) QueryFacts(ctx context.Context, namespaceSlugs []string, since, until *time.Time, page Pagination) ([]models.Fact, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	page = page.Sanitize()

	placeholders, nsArgs := inClause(1, nsIDs)
	query := fmt.Sprintf(`SELECT id, namespace_id, content, embedding, embedding_model, confidence,
	          entity, property, value, valid_from, valid_until, created_at, updated_at, deleted_at
	          FROM facts WHERE namespace_id IN (%s) AND deleted_at IS NULL`, placeholders)
	args := nsArgs
	argN := len(args)

	if since != nil {
		argN++
		query += fmt.Sprintf(" AND created_at >= $%d", argN)
		args = append(args, *since)
	}
	if until != nil {
		argN++
		query += fmt.Sprintf(" AND created_at <= $%d", argN)
		args = append(args, *until)
	}

	argN++
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT $%d", argN)
	args = append(args, page.Limit)

	argN++
	query += fmt.Sprintf(" OFFSET $%d", argN)
	args = append(args, page.Offset)

	rows, err := b.pool.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query facts: %w", err)
	}
	defer rows.Close()

	var facts []models.Fact
	for rows.Next() {
		var f models.Fact
		if err := rows.Scan(
			&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel,
			&f.Confidence, &f.Entity, &f.Property, &f.Value,
			&f.ValidFrom, &f.ValidUntil,
			&f.CreatedAt, &f.UpdatedAt, &f.DeletedAt,
		); err != nil {
			return nil, fmt.Errorf("scan fact: %w", err)
		}
		facts = append(facts, f)
	}
	return facts, rows.Err()
}

// UpdateFactConfidence updates the confidence score of a fact.
func (b *Brain) UpdateFactConfidence(ctx context.Context, factID int64, confidence float32) error {
	_, err := b.pool.ExecContext(ctx,
		"UPDATE facts SET confidence = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $1",
		factID, confidence,
	)
	if err != nil {
		return fmt.Errorf("update fact confidence: %w", err)
	}
	return nil
}

// PurgeFact hard-deletes a fact by ID.
func (b *Brain) PurgeFact(ctx context.Context, factID int64) error {
	tag, err := b.pool.ExecContext(ctx, "DELETE FROM facts WHERE id = $1", factID)
	if err != nil {
		return fmt.Errorf("purge fact: %w", err)
	}
	affected, err := rowsAffected(tag)
	if err != nil {
		return fmt.Errorf("purge fact rows affected: %w", err)
	}
	if affected == 0 {
		return ErrFactNotFound
	}
	return nil
}

// GetFact returns a single fact by ID.
func (b *Brain) GetFact(ctx context.Context, factID int64) (*models.Fact, error) {
	var f models.Fact
	err := b.pool.QueryRowContext(ctx,
		`SELECT id, namespace_id, content, embedding, embedding_model, confidence,
		 entity, property, value, valid_from, valid_until, created_at, updated_at, deleted_at
		 FROM facts WHERE id = $1`,
		factID,
	).Scan(
		&f.ID, &f.NamespaceID, &f.Content, &f.Embedding, &f.EmbeddingModel,
		&f.Confidence, &f.Entity, &f.Property, &f.Value,
		&f.ValidFrom, &f.ValidUntil,
		&f.CreatedAt, &f.UpdatedAt, &f.DeletedAt,
	)
	if err != nil {
		if isNoRows(err) {
			return nil, ErrFactNotFound
		}
		return nil, fmt.Errorf("get fact: %w", err)
	}
	return &f, nil
}

// QueryRelationships returns relationships across namespaces matching the given slug paths.
// Each path matches itself and all descendants.
func (b *Brain) QueryRelationships(ctx context.Context, namespaceSlugs []string, page Pagination) ([]models.Relationship, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	page = page.Sanitize()

	placeholders, args := inClause(1, nsIDs)
	args = append(args, page.Limit, page.Offset)
	rows, err := b.pool.QueryContext(ctx,
		fmt.Sprintf(`SELECT id, namespace_id, from_entity, relation_type, to_entity, confidence, source_fact_id, created_at, deleted_at
		 FROM relationships WHERE namespace_id IN (%s) AND deleted_at IS NULL ORDER BY id LIMIT $%d OFFSET $%d`, placeholders, len(nsIDs)+1, len(nsIDs)+2),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query relationships: %w", err)
	}
	defer rows.Close()

	var rels []models.Relationship
	for rows.Next() {
		var r models.Relationship
		if err := rows.Scan(&r.ID, &r.NamespaceID, &r.FromEntity, &r.RelationType, &r.ToEntity, &r.Confidence, &r.SourceFactID, &r.CreatedAt, &r.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan relationship: %w", err)
		}
		rels = append(rels, r)
	}
	return rels, rows.Err()
}

// QueryPatterns returns patterns across namespaces matching the given slug paths.
// Each path matches itself and all descendants.
func (b *Brain) QueryPatterns(ctx context.Context, namespaceSlugs []string, page Pagination) ([]models.Pattern, error) {
	nsIDs, err := b.resolveNamespaceIDs(ctx, namespaceSlugs)
	if err != nil {
		return nil, err
	}

	page = page.Sanitize()

	placeholders, args := inClause(1, nsIDs)
	args = append(args, page.Limit, page.Offset)
	rows, err := b.pool.QueryContext(ctx,
		fmt.Sprintf(`SELECT id, namespace_id, content, confidence, source_fact_ids, source_rel_ids, coherence_score, created_at, updated_at, deleted_at
		 FROM patterns WHERE namespace_id IN (%s) AND deleted_at IS NULL ORDER BY id LIMIT $%d OFFSET $%d`, placeholders, len(nsIDs)+1, len(nsIDs)+2),
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("query patterns: %w", err)
	}
	defer rows.Close()

	var patterns []models.Pattern
	for rows.Next() {
		var p models.Pattern
		if err := rows.Scan(&p.ID, &p.NamespaceID, &p.Content, &p.Confidence, &p.SourceFactIDs, &p.SourceRelIDs, &p.CoherenceScore, &p.CreatedAt, &p.UpdatedAt, &p.DeletedAt); err != nil {
			return nil, fmt.Errorf("scan pattern: %w", err)
		}
		patterns = append(patterns, p)
	}
	return patterns, rows.Err()
}
