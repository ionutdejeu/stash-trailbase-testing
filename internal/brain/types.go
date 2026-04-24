package brain

import (
	"fmt"
	"time"

	"github.com/alash3al/stash/internal/brain/store"
)

// --- Core Types ---

// Memory is the agent-facing representation of a stored fact.
type Memory struct {
	ID               string    `json:"id"`
	Namespace        string    `json:"namespace"`
	Content          string    `json:"content"`
	Confidence       float32   `json:"confidence"`
	ObservationCount int       `json:"observation_count"`
	Score            float32   `json:"score,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

// Fact is the internal representation of a consolidated belief.
type Fact struct {
	ID               string         `json:"id"`
	Namespace        string         `json:"namespace"`
	Content          string         `json:"content"`
	Type             string         `json:"type"` // "atemporal", "state", "point-in-time"
	Confidence       float32        `json:"confidence"`
	ObservationCount int            `json:"observation_count"`
	Source           string         `json:"source"`
	ValidFrom        time.Time      `json:"valid_from"`
	Score            float32        `json:"score,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

// Relationship represents a directed edge in the knowledge graph.
type Relationship struct {
	ID           string    `json:"id"`
	Namespace    string    `json:"namespace"`
	FromEntity   string    `json:"from_entity"`
	RelationType string    `json:"relationship_type"`
	ToEntity     string    `json:"to_entity"`
	Confidence   float32   `json:"confidence"`
	Source       string    `json:"source"`
	CreatedAt    time.Time `json:"created_at"`
}

// Contradiction represents conflicting facts about the same entity+property.
type Contradiction struct {
	ID           string    `json:"id"`
	FactID1      string    `json:"fact_id_1"`
	FactID2      string    `json:"fact_id_2"`
	Entity       string    `json:"entity"`
	Property     string    `json:"property"`
	Value1       string    `json:"value_1"`
	Value2       string    `json:"value_2"`
	Status       string    `json:"status"` // "conflict" or "evolution"
	DiscoveredAt time.Time `json:"discovered_at"`
}

// --- Checkpoint Types ---

// Checkpoint tracks consolidation progress for a namespace.
type Checkpoint struct {
	Namespace          string    `json:"namespace"`
	LastRowID          int64     `json:"last_row_id"`          // _row_id of last processed event
	LastFactRowID      int64     `json:"last_fact_row_id"`     // _row_id of last processed fact
	LastRun            time.Time `json:"last_run"`             // When consolidation last ran
	EventsProcessed    int       `json:"events_processed"`     // Total events processed
	FactsCreated       int       `json:"facts_created"`        // Total facts created
	RelationshipsFound int       `json:"relationships_found"`  // Total relationships extracted
	LLMCalls           int       `json:"llm_calls"`            // Total LLM calls made
	LastError          string    `json:"last_error,omitempty"` // Last error message, if any
}

// ConsolidationMetrics tracks metrics for a single consolidation run.
type ConsolidationMetrics struct {
	Namespace          string        `json:"namespace"`
	StartTime          time.Time     `json:"start_time"`
	EndTime            time.Time     `json:"end_time"`
	Duration           time.Duration `json:"duration_ms"`
	EventsRead         int           `json:"events_read"`
	EventsProcessed    int           `json:"events_processed"`
	ClustersFound      int           `json:"clusters_found"`
	FactsCreated       int           `json:"facts_created"`
	RelationshipsFound int           `json:"relationships_found"`
	LLMCalls           int           `json:"llm_calls"`
	CostEstimate       float64       `json:"cost_estimate"` // Estimated cost in USD
	Errors             []string      `json:"errors,omitempty"`
}

// Report summarizes memory state.
type Report struct {
	Namespace           string                    `json:"namespace"`
	TotalFacts          int                       `json:"total_facts"`
	TotalEvents         int                       `json:"total_events"`
	TotalRelationships  int                       `json:"total_relationships"`
	TotalContradictions int                       `json:"total_contradictions"`
	EntitiesByName      map[string]*EntitySummary `json:"entities_by_name,omitempty"`
	Contradictions      []Contradiction           `json:"contradictions,omitempty"`
	Pipeline            PipelineStatus            `json:"pipeline"`
	GeneratedAt         time.Time                 `json:"generated_at"`
}

// PipelineStatus shows background pipeline health.
type PipelineStatus struct {
	QueueDepth  int       `json:"queue_depth"`
	LastRun     time.Time `json:"last_run"`
	LastSuccess bool      `json:"last_success"`
	LastError   string    `json:"last_error,omitempty"`
}

// EntitySummary aggregates facts about a single entity.
type EntitySummary struct {
	Entity             string                 `json:"entity"`
	FactCount          int                    `json:"fact_count"`
	Properties         map[string][]FactValue `json:"properties,omitempty"`
	ContradictionCount int                    `json:"contradiction_count"`
}

// FactValue represents a single property value.
type FactValue struct {
	Value      string     `json:"value"`
	FactID     string     `json:"fact_id"`
	ValidFrom  time.Time  `json:"valid_from"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
}

// --- Constants ---

const (
	typeEvent        = "event"
	typeFact         = "fact"
	typeRelationship = "relationship"
	typeContext      = "context"
	typeCheckpoint   = "checkpoint"

	FactTypeAtemporal   = "atemporal"
	FactTypeState       = "state"
	FactTypePointInTime = "point-in-time"
)

// --- Record Conversion ---

// memoryFromRecord converts a store.Record to a Memory.
func memoryFromRecord(r store.Record, score float32) Memory {
	memMeta, _ := r.Metadata["_memory"].(map[string]any)

	var timestamp time.Time
	if ts, ok := memMeta["timestamp"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			timestamp = parsed
		}
	}
	if timestamp.IsZero() {
		timestamp = r.CreatedAt
	}

	confidence := float32(0.5)
	if c, ok := memMeta["confidence"].(float64); ok {
		confidence = float32(c)
	}

	observationCount := 1
	if oc, ok := memMeta["observation_count"].(float64); ok {
		observationCount = int(oc)
	}

	return Memory{
		ID:               r.ID,
		Namespace:        r.Namespace,
		Content:          r.Content,
		Confidence:       confidence,
		ObservationCount: observationCount,
		Score:            score,
		CreatedAt:        timestamp,
	}
}

// factFromRecord extracts a Fact from a store.Record.
func factFromRecord(r store.Record) (*Fact, error) {
	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("record metadata missing _memory field")
	}

	recType, ok := memMeta["type"].(string)
	if !ok || recType != typeFact {
		return nil, fmt.Errorf("record is not a fact (type=%q)", recType)
	}

	confidence := float32(0.5)
	if c, ok := memMeta["confidence"].(float64); ok {
		confidence = float32(c)
	}

	observationCount := 1
	if oc, ok := memMeta["observation_count"].(float64); ok {
		observationCount = int(oc)
	}

	factType := FactTypeState
	if ft, ok := memMeta["fact_type"].(string); ok && ft != "" {
		factType = ft
	}

	source, _ := memMeta["source"].(string)

	var validFrom time.Time
	if ts, ok := memMeta["valid_from"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			validFrom = parsed
		}
	}
	if validFrom.IsZero() {
		validFrom = r.CreatedAt
	}

	return &Fact{
		ID:               r.ID,
		Namespace:        r.Namespace,
		Content:          r.Content,
		Type:             factType,
		Confidence:       confidence,
		ObservationCount: observationCount,
		Source:           source,
		ValidFrom:        validFrom,
		Score:            0,
		Metadata:         r.Metadata,
	}, nil
}

// relationshipFromRecord extracts a Relationship from a store.Record.
func relationshipFromRecord(r store.Record) (*Relationship, error) {
	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("record metadata missing _memory field")
	}

	recType, ok := memMeta["type"].(string)
	if !ok || recType != typeRelationship {
		return nil, fmt.Errorf("record is not a relationship (type=%q)", recType)
	}

	fromEntity, _ := memMeta["from_entity"].(string)
	relationType, _ := memMeta["relationship_type"].(string)
	toEntity, _ := memMeta["to_entity"].(string)

	if fromEntity == "" || relationType == "" || toEntity == "" {
		return nil, fmt.Errorf("relationship missing required fields")
	}

	createdAt := r.CreatedAt
	if ts, ok := memMeta["created_at"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			createdAt = parsed
		}
	}

	source, _ := memMeta["source"].(string)
	if source == "" {
		source = "unknown"
	}

	confidence := float32(0.5)
	if c, ok := memMeta["confidence"].(float64); ok {
		confidence = float32(c)
	}

	return &Relationship{
		ID:           r.ID,
		Namespace:    r.Namespace,
		FromEntity:   fromEntity,
		RelationType: relationType,
		ToEntity:     toEntity,
		Confidence:   confidence,
		Source:       source,
		CreatedAt:    createdAt,
	}, nil
}

// checkpointFromRecord extracts a Checkpoint from a store.Record.
func checkpointFromRecord(r store.Record) (*Checkpoint, error) {
	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("record metadata missing _memory field")
	}

	recType, ok := memMeta["type"].(string)
	if !ok || recType != typeCheckpoint {
		return nil, fmt.Errorf("record is not a checkpoint (type=%q)", recType)
	}

namespace, _ := memMeta["namespace"].(string)
	lastRowID, _ := memMeta["last_row_id"].(float64)
	lastFactRowID, _ := memMeta["last_fact_row_id"].(float64)

	var lastRun time.Time
	if ts, ok := memMeta["last_run"].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, ts); err == nil {
			lastRun = parsed
		}
	}
	if lastRun.IsZero() {
		lastRun = r.CreatedAt
	}

	eventsProcessed, _ := memMeta["events_processed"].(float64)
	factsCreated, _ := memMeta["facts_created"].(float64)
	relationshipsFound, _ := memMeta["relationships_found"].(float64)
	llmCalls, _ := memMeta["llm_calls"].(float64)

	lastError, _ := memMeta["last_error"].(string)

	return &Checkpoint{
		Namespace:          namespace,
		LastRowID:          int64(lastRowID),
		LastFactRowID:      int64(lastFactRowID),
		LastRun:            lastRun,
		EventsProcessed:    int(eventsProcessed),
		FactsCreated:       int(factsCreated),
		RelationshipsFound: int(relationshipsFound),
		LLMCalls:           int(llmCalls),
		LastError:          lastError,
	}, nil
}

// checkpointToRecord converts a Checkpoint to a store.Record.
func checkpointToRecord(cp Checkpoint) store.Record {
	now := time.Now().UTC()
	
	memMeta := map[string]any{
		"type":               typeCheckpoint,
		"namespace":          cp.Namespace,
		"last_row_id":        cp.LastRowID,
		"last_fact_row_id":   cp.LastFactRowID,
		"last_run":           cp.LastRun.Format(time.RFC3339),
		"events_processed":   cp.EventsProcessed,
		"facts_created":      cp.FactsCreated,
		"relationships_found": cp.RelationshipsFound,
		"llm_calls":          cp.LLMCalls,
	}
	if cp.LastError != "" {
		memMeta["last_error"] = cp.LastError
	}

	return store.Record{
		ID:        fmt.Sprintf("checkpoint:%s", cp.Namespace),
		Namespace: "_system:checkpoints",
		Content:   fmt.Sprintf("Checkpoint for namespace %s", cp.Namespace),
		Metadata: map[string]any{
			"_memory": memMeta,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}
