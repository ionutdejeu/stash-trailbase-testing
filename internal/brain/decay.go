package brain

import (
	"context"
	"fmt"
	"time"
)

// DecayResult describes the outcome of a confidence decay run.
type DecayResult struct {
	FactsDecayed int `json:"facts_decayed"`
	FactsExpired int `json:"facts_expired"`
}

// DecayConfidence reduces confidence of facts not re-observed within the configured window.
// Facts below the expiry threshold are soft-deleted. This is pure SQL — no LLM calls.
func (b *Brain) DecayConfidence(ctx context.Context, nsID int64) (DecayResult, error) {
	if b.config.DecayFactor <= 0 || b.config.DecayFactor >= 1 {
		return DecayResult{}, nil
	}

	var result DecayResult
	cutoff := time.Now().UTC().Add(-b.config.Window)

	decayTag, err := b.pool.ExecContext(ctx,
		`UPDATE facts SET confidence = confidence * $3, updated_at = CURRENT_TIMESTAMP
		 WHERE namespace_id = $1 AND deleted_at IS NULL AND valid_until IS NULL
		 AND updated_at < $2`,
		nsID, cutoff, b.config.DecayFactor,
	)
	if err != nil {
		return result, fmt.Errorf("decay confidence: %w", err)
	}
	affected, err := rowsAffected(decayTag)
	if err != nil {
		return result, fmt.Errorf("decay rows affected: %w", err)
	}
	result.FactsDecayed = int(affected)

	expireTag, err := b.pool.ExecContext(ctx,
		`UPDATE facts SET valid_until = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
		 WHERE namespace_id = $1 AND deleted_at IS NULL AND valid_until IS NULL
		 AND confidence < $2`,
		nsID, b.config.ExpiryThreshold,
	)
	if err != nil {
		return result, fmt.Errorf("expire low-confidence facts: %w", err)
	}
	affected, err = rowsAffected(expireTag)
	if err != nil {
		return result, fmt.Errorf("expire rows affected: %w", err)
	}
	result.FactsExpired = int(affected)

	return result, nil
}
