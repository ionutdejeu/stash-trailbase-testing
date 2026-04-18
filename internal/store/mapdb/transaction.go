package mapdb

import (
	"context"

	"github.com/alash3al/stash/internal/store"
)

// WithTx runs fn inside a transaction.
func (s *Store) WithTx(ctx context.Context, fn func(tx store.Store) error) error {
	// Begin transaction
	tx := &Store{
		config:  s.config,
		records: s.copyRecords(),
		vectors: s.copyVectors(),
		deleted: s.copyDeleted(),
		txState: &txState{
			parent:   s,
			snapshot: s.copyRecords(),
			deleted:  s.copyDeleted(),
			pending:  make(map[string]*store.Record),
		},
	}

	// Run the transaction
	err := fn(tx)
	if err != nil {
		return err
	}

	// Commit transaction
	return tx.commit()
}

func (s *Store) copyRecords() map[string]*store.Record {
	copy := make(map[string]*store.Record, len(s.records))
	for k, v := range s.records {
		recordCopy := *v
		copy[k] = &recordCopy
	}
	return copy
}

func (s *Store) copyVectors() map[string][]*vectorEntry {
	copy := make(map[string][]*vectorEntry, len(s.vectors))
	for name, entries := range s.vectors {
		entriesCopy := make([]*vectorEntry, len(entries))
		for i, entry := range entries {
			entryCopy := *entry
			entriesCopy[i] = &entryCopy
		}
		copy[name] = entriesCopy
	}
	return copy
}

func (s *Store) copyDeleted() map[string]bool {
	copy := make(map[string]bool, len(s.deleted))
	for k, v := range s.deleted {
		copy[k] = v
	}
	return copy
}

func (s *Store) commit() error {
	if s.txState == nil {
		return nil
	}

	s.txState.parent.mu.Lock()
	defer s.txState.parent.mu.Unlock()

	// Apply pending changes
	for id, record := range s.txState.pending {
		if record == nil {
			// Delete
			s.txState.parent.deleted[id] = true
			if existing, exists := s.txState.parent.records[id]; exists {
				s.txState.parent.removeFromVectorsLocked(id, existing)
			}
		} else {
			// Put
			s.txState.parent.putLocked(*record)
		}
	}

	return nil
}

// Transaction methods

func (s *Store) txPut(ctx context.Context, r store.Record) error {
	s.txState.pending[r.ID] = &r
	return nil
}

func (s *Store) txGet(ctx context.Context, id string) (store.Record, error) {
	// Check pending changes first
	if pending, ok := s.txState.pending[id]; ok {
		if pending == nil {
			return store.Record{}, store.ErrNotFound
		}
		return *pending, nil
	}

	// Check snapshot
	if s.txState.deleted[id] {
		return store.Record{}, store.ErrNotFound
	}

	record, exists := s.txState.snapshot[id]
	if !exists {
		return store.Record{}, store.ErrNotFound
	}

	return *record, nil
}

func (s *Store) txDelete(ctx context.Context, id string) error {
	s.txState.pending[id] = nil
	return nil
}

func (s *Store) txPurge(ctx context.Context, id string) error {
	delete(s.txState.pending, id)
	delete(s.txState.snapshot, id)
	delete(s.txState.deleted, id)
	return nil
}

func (s *Store) txPutMany(ctx context.Context, rs []store.Record) error {
	for _, r := range rs {
		if err := s.validateRecord(&r); err != nil {
			return err
		}
		s.txState.pending[r.ID] = &r
	}
	return nil
}

func (s *Store) txDeleteWhere(ctx context.Context, p *store.Predicate) (int64, error) {
	// Collect matching records from snapshot
	var toDelete []string
	for id, record := range s.txState.snapshot {
		if s.txState.deleted[id] {
			continue
		}
		if s.evaluatePredicate(record, p).toBool() {
			toDelete = append(toDelete, id)
		}
	}

	count := int64(0)
	for _, id := range toDelete {
		s.txState.pending[id] = nil
		count++
	}

	return count, nil
}

func (s *Store) txSearch(ctx context.Context, q store.Query) ([]store.Record, error) {
	// For simplicity, create a temporary store with transaction state
	temp := &Store{
		config:  s.config,
		records: s.txState.snapshot,
		vectors: s.txState.parent.vectors, // Use parent vectors (won't be modified)
		deleted: s.txState.deleted,
	}

	// Apply pending changes
	for id, record := range s.txState.pending {
		if record == nil {
			temp.deleted[id] = true
			delete(temp.records, id)
		} else {
			temp.records[id] = record
			temp.deleted[id] = false
		}
	}

	return temp.Search(ctx, q)
}

func (s *Store) txList(ctx context.Context, f store.Filter) ([]store.Record, error) {
	// For simplicity, create a temporary store with transaction state
	temp := &Store{
		config:  s.config,
		records: s.txState.snapshot,
		vectors: s.txState.parent.vectors,
		deleted: s.txState.deleted,
	}

	// Apply pending changes
	for id, record := range s.txState.pending {
		if record == nil {
			temp.deleted[id] = true
			delete(temp.records, id)
		} else {
			temp.records[id] = record
			temp.deleted[id] = false
		}
	}

	return temp.List(ctx, f)
}

func (s *Store) txCount(ctx context.Context, p *store.Predicate) (int64, error) {
	temp := &Store{
		config:  s.config,
		records: s.txState.snapshot,
		vectors: s.txState.parent.vectors,
		deleted: s.txState.deleted,
	}

	// Apply pending changes
	for id, record := range s.txState.pending {
		if record == nil {
			temp.deleted[id] = true
			delete(temp.records, id)
		} else {
			temp.records[id] = record
			temp.deleted[id] = false
		}
	}

	return temp.Count(ctx, p)
}
