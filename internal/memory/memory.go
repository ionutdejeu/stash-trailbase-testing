package memory

import (
	"context"
	"errors"
	"time"

	"github.com/alash3al/stash/internal/embedder"
	"github.com/alash3al/stash/internal/store"
	"github.com/google/uuid"
)

const (
	frameID       = "_memory.working_frame"
	frameDuration = time.Hour
	typeEvent     = "event"
	typeFrame     = "frame"
)

var errMissingStore = errors.New("memory: store is required")
var errMissingEmbedder = errors.New("memory: embedder is required")

// Memory is the core memory system.
// Concrete type — not an interface.
// Extend it with new methods; do not abstract it.
type Memory struct {
	store    store.Store
	embedder embedder.Embedder
}

// New creates a Memory using the provided store and embedder.
// Both are required. Returns error if either is nil.
func New(s store.Store, e embedder.Embedder) (*Memory, error) {
	if s == nil {
		return nil, errMissingStore
	}
	if e == nil {
		return nil, errMissingEmbedder
	}
	return &Memory{
		store:    s,
		embedder: e,
	}, nil
}

// Remember stores an event with its embedding.
// Generates a UUID v4 event ID before calling store.Put.
// Returns the generated event ID on success.
// content must not be empty.
// metadata keys must not start with "_memory" (returns ErrInvalidMetadata).
func (m *Memory) Remember(ctx context.Context, content string, metadata map[string]any) (string, error) {
	if content == "" {
		return "", ErrEmptyContent
	}
	if err := validateMetadata(metadata); err != nil {
		return "", err
	}

	vec, err := m.embedder.Embed(ctx, content)
	if err != nil {
		return "", err
	}

	eventID := uuid.New().String()
	now := time.Now().UTC()

	memMeta := map[string]any{
		"type":      typeEvent,
		"content":   content,
		"timestamp": now.Format(time.RFC3339),
	}

	recordMeta := map[string]any{
		"_memory": memMeta,
	}
	for k, v := range metadata {
		recordMeta[k] = v
	}

	record := store.Record{
		ID:      eventID,
		Content: content,
		Vectors: map[string]store.Vector{
			m.embedder.Model(): {
				Values: vec,
				Model:  m.embedder.Model(),
			},
		},
		Metadata: recordMeta,
	}

	if err := m.store.Put(ctx, record); err != nil {
		return "", err
	}

	return eventID, nil
}

// Recall retrieves events relevant to a query.
// Embeds the query, searches the store by vector similarity.
// Returns at most limit events ordered by relevance.
// Returns empty slice (not error) when nothing matches.
// limit must be > 0.
func (m *Memory) Recall(ctx context.Context, query string, limit int) ([]Event, error) {
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	vec, err := m.embedder.Embed(ctx, query)
	if err != nil {
		return nil, err
	}

	results, err := m.store.Search(ctx, store.Query{
		Vector:     vec,
		VectorName: m.embedder.Model(),
		TopK:       limit,
		Filter: &store.Predicate{
			Field: "metadata._memory.type",
			Op:    store.OpEq,
			Value: typeEvent,
		},
	})
	if err != nil {
		return nil, err
	}

	events := make([]Event, 0, len(results))
	for _, r := range results {
		e, err := recordToEvent(r)
		if err != nil {
			continue
		}
		events = append(events, e)
	}

	return events, nil
}

// Frame returns the current working memory state.
// Creates a new frame if none exists.
// Replaces the frame (lazy) if the existing one has expired.
// input updates the Focus when a new frame is created.
// Does not start background goroutines.
func (m *Memory) Frame(ctx context.Context, input string) (Frame, error) {
	record, err := m.store.Get(ctx, frameID)
	if errors.Is(err, store.ErrNotFound) {
		return m.createFrame(ctx, input)
	}
	if err != nil {
		return Frame{}, err
	}

	frame, err := recordToFrame(record)
	if err != nil {
		return Frame{}, err
	}

	if time.Now().UTC().After(frame.ExpiresAt) {
		return m.createFrame(ctx, input)
	}

	return frame, nil
}

// Close releases any resources held by Memory.
func (m *Memory) Close() error {
	return nil
}

func (m *Memory) createFrame(ctx context.Context, focus string) (Frame, error) {
	now := time.Now().UTC()
	frame := Frame{
		ID:        frameID,
		Focus:     focus,
		EventIDs:  nil,
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(frameDuration),
	}

	recordMeta := map[string]any{
		"_memory": map[string]any{
			"type":       typeFrame,
			"focus":      focus,
			"event_ids":  frame.EventIDs,
			"created_at": frame.CreatedAt.Format(time.RFC3339),
			"updated_at": frame.UpdatedAt.Format(time.RFC3339),
			"expires_at": frame.ExpiresAt.Format(time.RFC3339),
		},
	}

	record := store.Record{
		ID:       frameID,
		Metadata: recordMeta,
	}

	if err := m.store.Put(ctx, record); err != nil {
		return Frame{}, err
	}

	return frame, nil
}

func validateMetadata(metadata map[string]any) error {
	if metadata == nil {
		return nil
	}
	for k := range metadata {
		if hasMemoryPrefix(k) {
			return ErrInvalidMetadata
		}
	}
	return nil
}

func hasMemoryPrefix(key string) bool {
	return len(key) >= 7 && key[:7] == "_memory"
}

func recordToEvent(r store.Record) (Event, error) {
	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return Event{}, ErrEventNotFound
	}

	content, _ := memMeta["content"].(string)
	tsStr, _ := memMeta["timestamp"].(string)
	var timestamp time.Time
	if tsStr != "" {
		timestamp, _ = time.Parse(time.RFC3339, tsStr)
	}

	callerMeta := make(map[string]any)
	for k, v := range r.Metadata {
		if k != "_memory" {
			callerMeta[k] = v
		}
	}

	return Event{
		ID:        r.ID,
		Content:   content,
		Timestamp: timestamp,
		Metadata:  callerMeta,
	}, nil
}

func recordToFrame(r store.Record) (Frame, error) {
	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return Frame{}, ErrEventNotFound
	}

	focus, _ := memMeta["focus"].(string)
	eventIDs, _ := memMeta["event_ids"].([]string)

	createdAtStr, _ := memMeta["created_at"].(string)
	updatedAtStr, _ := memMeta["updated_at"].(string)
	expiresAtStr, _ := memMeta["expires_at"].(string)

	var createdAt, updatedAt, expiresAt time.Time
	if createdAtStr != "" {
		createdAt, _ = time.Parse(time.RFC3339, createdAtStr)
	}
	if updatedAtStr != "" {
		updatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
	}
	if expiresAtStr != "" {
		expiresAt, _ = time.Parse(time.RFC3339, expiresAtStr)
	}

	return Frame{
		ID:        r.ID,
		Focus:     focus,
		EventIDs:  eventIDs,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		ExpiresAt: expiresAt,
	}, nil
}
