// Package mapdb implements the store.Store interface using in-memory maps.
// It provides vector search, predicate filtering, and transactional isolation.
package mapdb

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/alash3al/stash/internal/store"
)

// Config holds mapdb-specific configuration.
type Config struct {
	// VectorDim is the dimension of all vectors stored in this store.
	// All vectors must have this exact dimension.
	VectorDim int

	// MaxResultSize is the hard cap on Limit in List and Search.
	// If a caller requests a larger limit, it is silently truncated.
	// Zero means the default (10000).
	MaxResultSize int
}

const (
	defaultMaxResultSize = 10000
)

type vectorEntry struct {
	id     string
	vector []float32
	record *store.Record
}

// Store implements store.Store using in-memory maps.
type Store struct {
	mu      sync.RWMutex
	config  Config
	records map[string]*store.Record  // ID -> Record
	vectors map[string][]*vectorEntry // vectorName -> []vectorEntry
	deleted map[string]bool           // soft-deleted IDs
	txState *txState                  // non-nil during transaction
}

type txState struct {
	parent   *Store
	snapshot map[string]*store.Record
	deleted  map[string]bool
	pending  map[string]*store.Record // ID -> Record (nil means delete)
}

// New creates a new mapdb Store.
func New(cfg Config) (store.Store, error) {
	if cfg.VectorDim <= 0 {
		return nil, errors.New("mapdb: VectorDim must be positive")
	}

	if cfg.MaxResultSize <= 0 {
		cfg.MaxResultSize = defaultMaxResultSize
	}

	return &Store{
		config:  cfg,
		records: make(map[string]*store.Record),
		vectors: make(map[string][]*vectorEntry),
		deleted: make(map[string]bool),
	}, nil
}

// Put stores a record, creating or replacing it.
func (s *Store) Put(ctx context.Context, r store.Record) error {
	if err := s.validateRecord(&r); err != nil {
		return err
	}

	if s.txState != nil {
		return s.txPut(ctx, r)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.putLocked(r)
}

// Get retrieves a live record by ID.
func (s *Store) Get(ctx context.Context, id string) (store.Record, error) {
	if s.txState != nil {
		return s.txGet(ctx, id)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	return s.getLocked(id)
}

// Delete soft-deletes a record by ID.
func (s *Store) Delete(ctx context.Context, id string) error {
	if s.txState != nil {
		return s.txDelete(ctx, id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	return s.deleteLocked(id)
}

// Purge hard-deletes a record by ID, removing it permanently.
func (s *Store) Purge(ctx context.Context, id string) error {
	if s.txState != nil {
		return s.txPurge(ctx, id)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	s.purgeLocked(id)
	return nil
}

// PutMany stores multiple records atomically.
func (s *Store) PutMany(ctx context.Context, rs []store.Record) error {
	if s.txState != nil {
		return s.txPutMany(ctx, rs)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, r := range rs {
		if err := s.validateRecord(&r); err != nil {
			return fmt.Errorf("mapdb: invalid record %q: %w", r.ID, err)
		}
	}

	for _, r := range rs {
		if err := s.putLocked(r); err != nil {
			return fmt.Errorf("mapdb: put %q: %w", r.ID, err)
		}
	}

	return nil
}

// DeleteWhere soft-deletes all live records matching the predicate.
func (s *Store) DeleteWhere(ctx context.Context, p *store.Predicate) (int64, error) {
	if s.txState != nil {
		return s.txDeleteWhere(ctx, p)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := s.listLocked(store.Filter{Where: p})
	if err != nil {
		return 0, err
	}

	count := int64(0)
	for _, r := range records {
		if err := s.deleteLocked(r.ID); err == nil {
			count++
		}
	}

	return count, nil
}

// Health checks the store's connectivity and readiness.
func (s *Store) Health(ctx context.Context) error {
	return nil
}

// Migrate applies any required schema migrations.
func (s *Store) Migrate(ctx context.Context) error {
	return nil
}

// Close releases resources associated with the store.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.records = nil
	s.vectors = nil
	s.deleted = nil
	return nil
}

func (s *Store) validateRecord(r *store.Record) error {
	if r.ID == "" {
		return errors.New("mapdb: record ID cannot be empty")
	}

	for name, vec := range r.Vectors {
		if len(vec.Values) != s.config.VectorDim {
			return fmt.Errorf("mapdb: vector %q has dimension %d, expected %d",
				name, len(vec.Values), s.config.VectorDim)
		}
	}

	now := time.Now().UTC()
	if r.CreatedAt.IsZero() {
		r.CreatedAt = now
	}
	if r.UpdatedAt.IsZero() {
		r.UpdatedAt = now
	}

	// Normalize metadata to match JSON marshaling behavior
	r.Metadata = s.normalizeMetadata(r.Metadata)

	return nil
}

func (s *Store) normalizeMetadata(metadata map[string]any) map[string]any {
	if metadata == nil {
		return nil
	}

	normalized := make(map[string]any, len(metadata))
	for k, v := range metadata {
		normalized[k] = s.normalizeValue(v)
	}
	return normalized
}

func (s *Store) normalizeValue(v any) any {
	switch val := v.(type) {
	case int:
		return float64(val)
	case int8:
		return float64(val)
	case int16:
		return float64(val)
	case int32:
		return float64(val)
	case int64:
		return float64(val)
	case uint:
		return float64(val)
	case uint8:
		return float64(val)
	case uint16:
		return float64(val)
	case uint32:
		return float64(val)
	case uint64:
		return float64(val)
	case float32:
		return float64(val)
	case []any:
		normalized := make([]any, len(val))
		for i, item := range val {
			normalized[i] = s.normalizeValue(item)
		}
		return normalized
	case map[string]any:
		return s.normalizeMetadata(val)
	default:
		return v
	}
}

func (s *Store) putLocked(r store.Record) error {
	// Remove from vectors if existing
	if existing, exists := s.records[r.ID]; exists {
		s.removeFromVectorsLocked(r.ID, existing)
	}

	// Update record
	recordCopy := r
	s.records[r.ID] = &recordCopy
	delete(s.deleted, r.ID)

	// Add to vectors
	for name, vec := range r.Vectors {
		entry := &vectorEntry{
			id:     r.ID,
			vector: vec.Values,
			record: &recordCopy,
		}
		s.vectors[name] = append(s.vectors[name], entry)
	}

	return nil
}

func (s *Store) getLocked(id string) (store.Record, error) {
	if s.deleted[id] {
		return store.Record{}, store.ErrNotFound
	}

	record, exists := s.records[id]
	if !exists {
		return store.Record{}, store.ErrNotFound
	}

	return *record, nil
}

func (s *Store) deleteLocked(id string) error {
	if s.deleted[id] {
		return nil
	}

	record, exists := s.records[id]
	if !exists {
		return nil
	}

	s.deleted[id] = true
	now := time.Now().UTC()
	record.DeletedAt = &now

	return nil
}

func (s *Store) purgeLocked(id string) {
	if record, exists := s.records[id]; exists {
		s.removeFromVectorsLocked(id, record)
	}
	delete(s.records, id)
	delete(s.deleted, id)
}

func (s *Store) removeFromVectorsLocked(id string, r *store.Record) {
	for name := range r.Vectors {
		entries := s.vectors[name]
		for i, entry := range entries {
			if entry.id == id {
				s.vectors[name] = append(entries[:i], entries[i+1:]...)
				break
			}
		}
	}
}
