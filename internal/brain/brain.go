package brain

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/alash3al/stash/internal/brain/store"
	"github.com/alash3al/stash/internal/embedder"
	"github.com/alash3al/stash/internal/reasoner"
	"github.com/google/uuid"
)

var (
	errMissingStore    = errors.New("brain: store is required")
	errMissingEmbedder = errors.New("brain: embedder is required")
	errMissingReasoner = errors.New("brain: reasoner is required")
)

const (
	consolidationWindow = 7 * 24 * time.Hour
	similarityThreshold = 0.85
)

// Brain is the agent's memory system.
type Brain struct {
	store    store.Store
	embedder embedder.Embedder
	reasoner reasoner.Reasoner
	
	// consolidation configuration
	consolidationConfig ConsolidationConfig
	rateLimiter         *rateLimiter
}

// ConsolidationConfig holds configuration for consolidation process.
type ConsolidationConfig struct {
	BatchSize          int           // Number of events to process per run
	MaxLLMCallsPerHour int           // Rate limit for LLM calls
	SimilarityThreshold float64      // Threshold for clustering similarity
	Window             time.Duration // Time window for event consolidation
}

// rateLimiter implements a simple token bucket rate limiter.
type rateLimiter struct {
	tokens     int
	maxTokens  int
	refillRate time.Duration // time between token refills
	lastRefill time.Time
	mu         sync.Mutex
}

// newRateLimiter creates a rate limiter that allows maxTokens per hour.
func newRateLimiter(maxTokensPerHour int) *rateLimiter {
	return &rateLimiter{
		tokens:     maxTokensPerHour,
		maxTokens:  maxTokensPerHour,
		refillRate: time.Hour / time.Duration(maxTokensPerHour),
		lastRefill: time.Now(),
	}
}

// Allow returns true if a token is available and consumes it.
func (rl *rateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	// Refill tokens based on elapsed time
	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	tokensToAdd := int(elapsed / rl.refillRate)
	if tokensToAdd > 0 {
		rl.tokens += tokensToAdd
		if rl.tokens > rl.maxTokens {
			rl.tokens = rl.maxTokens
		}
		rl.lastRefill = now
	}

	if rl.tokens > 0 {
		rl.tokens--
		return true
	}
	return false
}

// New creates a Brain with the provided store, embedder, and reasoner.
func New(s store.Store, e embedder.Embedder, r reasoner.Reasoner) (*Brain, error) {
	return NewWithConfig(s, e, r, ConsolidationConfig{
		BatchSize:          100,
		MaxLLMCallsPerHour: 100,
		SimilarityThreshold: similarityThreshold,
		Window:             consolidationWindow,
	})
}

// NewWithConfig creates a Brain with custom consolidation configuration.
func NewWithConfig(s store.Store, e embedder.Embedder, r reasoner.Reasoner, cfg ConsolidationConfig) (*Brain, error) {
	if s == nil {
		return nil, errMissingStore
	}
	if e == nil {
		return nil, errMissingEmbedder
	}
	if r == nil {
		return nil, errMissingReasoner
	}
	return &Brain{
		store:    s,
		embedder: e,
		reasoner: r,
		consolidationConfig: cfg,
		rateLimiter:         newRateLimiter(cfg.MaxLLMCallsPerHour),
	}, nil
}

// Close releases resources.
func (b *Brain) Close() error {
	return b.store.Close()
}

// Consolidate processes recent events into facts and extracts relationships.
func (b *Brain) Consolidate(ctx context.Context, namespace string) error {
	// Check context before starting expensive work
	if checkContext(ctx) {
		return ctx.Err()
	}

	// Get or create checkpoint
	cp, err := b.getCheckpoint(ctx, namespace)
	if err != nil {
		return fmt.Errorf("get checkpoint: %w", err)
	}

	// Query events since last checkpoint (incremental)
	records, err := b.queryEventsSince(ctx, namespace, cp.LastRowID, b.consolidationConfig.BatchSize)
	if err != nil {
		return fmt.Errorf("query events since checkpoint: %w", err)
	}
	if len(records) == 0 {
		// No new events to process
		return nil
	}

	// Track metrics for this run
	metrics := ConsolidationMetrics{
		StartTime: time.Now(),
	}

	// Cluster by similarity
	clusters := b.clusterRecordsBySimilarity(records, b.consolidationConfig.SimilarityThreshold)

	for _, cluster := range clusters {
		// Check context cancellation
		if checkContext(ctx) {
			break
		}

		if len(cluster) < 2 {
			continue
		}

		// Check rate limit before making LLM call
		if !b.rateLimiter.Allow() {
			// Rate limit exceeded, stop processing
			break
		}

		// Extract event contents
		var texts []string
		var eventIDs []string
		for _, r := range cluster {
			texts = append(texts, r.Content)
			eventIDs = append(eventIDs, r.ID)
		}

		// Use LLM to synthesize fact
		structured, err := b.reasoner.ReasonStructured(ctx, texts)
		if err != nil {
			// Continue with other clusters
			metrics.LLMCalls++
			continue
		}

		metrics.LLMCalls++

		if structured.Summary == "" {
			continue
		}

		// Store the fact
		factType := FactTypeState
		if structured.Entity != "" && structured.Property != "" {
			factType = FactTypeAtemporal
		}

		err = b.storeFact(ctx, namespace, structured.Summary, factType, len(cluster), "consolidation", eventIDs)
		if err != nil {
			// Continue with other clusters
			continue
		}

		metrics.FactsCreated++
	}

	// Update checkpoint with progress
	metrics.EndTime = time.Now()
	metrics.EventsProcessed = len(records)

	// Find the highest RowID from processed records
	var lastRowID int64
	for _, r := range records {
		if r.RowID > lastRowID {
			lastRowID = r.RowID
		}
	}

	if lastRowID > 0 {
		cp.LastRowID = lastRowID
	}
	cp.LastRun = time.Now()
	cp.EventsProcessed += metrics.EventsProcessed
	cp.FactsCreated += metrics.FactsCreated
	cp.LLMCalls += metrics.LLMCalls

	// Save checkpoint
	if err := b.saveCheckpoint(ctx, *cp); err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}

	// Extract relationships from facts (incremental)
	return b.ExtractRelationships(ctx, namespace)
}

// ExtractRelationships uses the LLM to find relationships in facts.
func (b *Brain) ExtractRelationships(ctx context.Context, namespace string) error {
	// Check context before starting
	if checkContext(ctx) {
		return ctx.Err()
	}

	// Get checkpoint to find where we left off
	cp, err := b.getCheckpoint(ctx, namespace)
	if err != nil {
		return fmt.Errorf("get checkpoint: %w", err)
	}

	// Query facts since last checkpoint (incremental)
	const factsBatchSize = 50
	facts, err := b.queryFactsSince(ctx, namespace, cp.LastFactRowID, factsBatchSize)
	if err != nil {
		return fmt.Errorf("query facts since checkpoint: %w", err)
	}
	if len(facts) == 0 {
		// No new facts to process
		return nil
	}

	relationshipsFound := 0

	for _, fact := range facts {
		// Check context cancellation in loop
		if checkContext(ctx) {
			break
		}

		// Check rate limit before making LLM call
		if !b.rateLimiter.Allow() {
			// Rate limit exceeded, stop processing
			break
		}

		// Skip facts that already have relationships extracted
		if memMeta, ok := fact.Metadata["_memory"].(map[string]any); ok {
			if processed, ok := memMeta["relationships_extracted"].(bool); ok && processed {
				continue
			}
		}

		relationships, err := b.reasoner.ReasonRelationships(ctx, fact.Content)
		if err != nil {
			// Continue with other facts
			cp.LLMCalls++
			continue
		}

		cp.LLMCalls++

		for _, rel := range relationships {
			err := b.storeRelationship(ctx, namespace, rel.FromEntity, rel.RelationType, rel.ToEntity, "consolidation", rel.Confidence)
			if err != nil {
				// Continue with other relationships
			} else {
				relationshipsFound++
			}
		}
	}

	// Update checkpoint with progress
	// Find the highest RowID from processed facts
	var lastFactRowID int64
	for _, fact := range facts {
		if fact.RowID > lastFactRowID {
			lastFactRowID = fact.RowID
		}
	}

	if lastFactRowID > 0 {
		cp.LastFactRowID = lastFactRowID
	}
	cp.RelationshipsFound += relationshipsFound

	// Save checkpoint
	if err := b.saveCheckpoint(ctx, *cp); err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}

	return nil
}

// vectorKey returns the key used for storing vectors.
func (b *Brain) vectorKey() string {
	return b.embedder.Model()
}

// calculateConfidence computes confidence from observation count.
func calculateConfidence(observationCount int) float32 {
	if observationCount == 0 {
		return 0.0
	}
	return float32(observationCount) / float32(observationCount+2)
}

// checkContext returns true if context is cancelled.
func checkContext(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// --- Internal helpers for consolidation ---

// queryRecentEventRecords returns event records from the last 7 days.
func (b *Brain) queryRecentEventRecords(ctx context.Context, namespace string, since time.Time) ([]store.Record, error) {
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	}

	records, err := b.store.List(ctx, store.Filter{
		Namespaces: namespaces,
		Where: &store.Predicate{
			And: []store.Predicate{
				{Field: "metadata._memory.type", Op: store.OpEq, Value: typeEvent},
				{Field: "metadata._memory.timestamp", Op: store.OpGte, Value: since.Format(time.RFC3339)},
			},
		},
		Limit: 1000,
	})
	if err != nil {
		return nil, err
	}
	return records, nil
}

// clusterRecordsBySimilarity groups records by cosine similarity.
func (b *Brain) clusterRecordsBySimilarity(records []store.Record, threshold float64) [][]store.Record {
	if len(records) == 0 {
		return nil
	}

	clusters := make([][]store.Record, 0)
	used := make(map[int]bool)

	for i, r1 := range records {
		if used[i] {
			continue
		}

		cluster := []store.Record{r1}
		used[i] = true

		vec1, ok1 := r1.Vectors[b.vectorKey()]
		if !ok1 {
			clusters = append(clusters, cluster)
			continue
		}

		for j := i + 1; j < len(records); j++ {
			if used[j] {
				continue
			}

			vec2, ok2 := records[j].Vectors[b.vectorKey()]
			if !ok2 {
				continue
			}

			sim := cosineSimilarity(vec1.Values, vec2.Values)
			if sim >= threshold {
				cluster = append(cluster, records[j])
				used[j] = true
			}
		}

		clusters = append(clusters, cluster)
	}

	return clusters
}

// cosineSimilarity computes cosine similarity between two vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := 0; i < len(a); i++ {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// storeFact stores a consolidated fact in the store.
func (b *Brain) storeFact(ctx context.Context, namespace, content, factType string, observationCount int, source string, synthesizedFrom []string) error {
	now := time.Now().UTC()
	factID := uuid.New().String()

	vec, err := b.embedder.Embed(ctx, content)
	if err != nil {
		return fmt.Errorf("embed fact: %w", err)
	}

	confidence := calculateConfidence(observationCount)

	memMeta := map[string]any{
		"type":              typeFact,
		"fact_type":         factType,
		"confidence":        float64(confidence),
		"observation_count": observationCount,
		"source":            source,
		"synthesized_from":  synthesizedFrom,
		"created_at":        now.Format(time.RFC3339),
		"valid_from":        now.Format(time.RFC3339),
	}

	record := store.Record{
		ID:        factID,
		Namespace: namespace,
		Content:   content,
		Vectors: map[string]store.Vector{
			b.vectorKey(): {
				Values: vec,
				Model:  b.embedder.Model(),
			},
		},
		Metadata: map[string]any{
			"_memory": memMeta,
		},
	}

	return b.store.Put(ctx, record)
}

// storeRelationship stores a relationship in the store.
func (b *Brain) storeRelationship(ctx context.Context, namespace, fromEntity, relationType, toEntity, source string, confidence float32) error {
	now := time.Now().UTC()
	relID := uuid.New().String()

	memMeta := map[string]any{
		"type":              typeRelationship,
		"from_entity":       fromEntity,
		"relationship_type": relationType,
		"to_entity":         toEntity,
		"confidence":        float64(confidence),
		"source":            source,
		"created_at":        now.Format(time.RFC3339),
	}

	record := store.Record{
		ID:        relID,
		Namespace: namespace,
		Content:   fmt.Sprintf("%s %s %s", fromEntity, relationType, toEntity),
		Metadata: map[string]any{
			"_memory": memMeta,
		},
	}

	return b.store.Put(ctx, record)
}

// queryFacts returns all facts in a namespace.
func (b *Brain) queryFacts(ctx context.Context, namespace string) ([]Fact, error) {
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	}

	records, err := b.store.List(ctx, store.Filter{
		Namespaces: namespaces,
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeFact,
		},
		Limit: 10000,
	})
	if err != nil {
		return nil, err
	}

	facts := make([]Fact, 0, len(records))
	for _, r := range records {
		f, err := factFromRecord(r)
		if err != nil {
			continue
		}
		facts = append(facts, *f)
	}

	return facts, nil
}

// queryRelationships returns all relationships in a namespace.
func (b *Brain) queryRelationships(ctx context.Context, namespace string) ([]Relationship, error) {
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	}

	records, err := b.store.List(ctx, store.Filter{
		Namespaces: namespaces,
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeRelationship,
		},
		Limit: 10000,
	})
	if err != nil {
		return nil, err
	}

	relationships := make([]Relationship, 0, len(records))
	for _, r := range records {
		rel, err := relationshipFromRecord(r)
		if err != nil {
			continue
		}
		relationships = append(relationships, *rel)
	}

	return relationships, nil
}

// queryEvents returns all events in a namespace.
func (b *Brain) queryEvents(ctx context.Context, namespace string) ([]store.Record, error) {
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	}

	return b.store.List(ctx, store.Filter{
		Namespaces: namespaces,
		Where: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeEvent,
		},
		Limit: 10000,
	})
}

// getCheckpoint retrieves the checkpoint for a namespace.
func (b *Brain) getCheckpoint(ctx context.Context, namespace string) (*Checkpoint, error) {
	record, err := b.store.Get(ctx, fmt.Sprintf("checkpoint:%s", namespace))
	if err != nil {
		// If not found, return empty checkpoint
		if strings.Contains(err.Error(), "not found") {
			return &Checkpoint{
				Namespace: namespace,
				LastRun:   time.Time{},
			}, nil
		}
		return nil, fmt.Errorf("get checkpoint: %w", err)
	}

	cp, err := checkpointFromRecord(record)
	if err != nil {
		return nil, fmt.Errorf("parse checkpoint: %w", err)
	}

	return cp, nil
}

// saveCheckpoint stores a checkpoint.
func (b *Brain) saveCheckpoint(ctx context.Context, cp Checkpoint) error {
	record := checkpointToRecord(cp)
	if err := b.store.Put(ctx, record); err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}
	return nil
}

// queryEventsSince returns events after a checkpoint.
func (b *Brain) queryEventsSince(ctx context.Context, namespace string, sinceRowID int64, limit int) ([]store.Record, error) {
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	}

	// Build predicate: events after sinceRowID (if provided)
	var predicate *store.Predicate
	if sinceRowID > 0 {
		predicate = &store.Predicate{
			And: []store.Predicate{
				{Field: "metadata._memory.type", Op: store.OpEq, Value: typeEvent},
				{Field: "_row_id", Op: store.OpGt, Value: sinceRowID}, // Using _row_id for ordering
			},
		}
	} else {
		predicate = &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeEvent,
		}
	}

	return b.store.List(ctx, store.Filter{
		Namespaces: namespaces,
		Where:      predicate,
		Order:      []store.Order{{Field: "_row_id", Desc: false}}, // Use internal row ID for ordering
		Limit:      limit,
	})
}

// queryFactsSince returns facts after a checkpoint.
func (b *Brain) queryFactsSince(ctx context.Context, namespace string, sinceRowID int64, limit int) ([]store.Record, error) {
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	}

	// Build predicate: facts after sinceRowID (if provided)
	var predicate *store.Predicate
	if sinceRowID > 0 {
		predicate = &store.Predicate{
			And: []store.Predicate{
				{Field: "metadata._memory.type", Op: store.OpEq, Value: typeFact},
				{Field: "_row_id", Op: store.OpGt, Value: sinceRowID}, // Using _row_id for ordering
			},
		}
	} else {
		predicate = &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeFact,
		}
	}

	return b.store.List(ctx, store.Filter{
		Namespaces: namespaces,
		Where:      predicate,
		Order:      []store.Order{{Field: "_row_id", Desc: false}}, // Use internal row ID for ordering
		Limit:      limit,
	})
}
