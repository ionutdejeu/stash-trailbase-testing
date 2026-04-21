package mapdb

import (
	"context"
	"strings"

	"github.com/alash3al/stash/internal/store"
)

// Search performs vector or text similarity search.
func (s *Store) Search(ctx context.Context, q store.Query) ([]store.SearchResult, error) {
	if s.txState != nil {
		return s.txSearch(ctx, q)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	var results []store.SearchResult

	// Vector search
	if q.Vector != nil && q.VectorName != "" {
		limit := q.TopK
		if limit <= 0 || limit > s.config.MaxResultSize {
			limit = s.config.MaxResultSize
		}
		results = s.searchVectors(q.Vector, q.VectorName, limit)
	}

	// Text search (basic substring matching)
	if q.Text != "" && len(results) == 0 {
		records := s.searchText(q.Text, q.TopK)
		results = make([]store.SearchResult, len(records))
		for i, record := range records {
			results[i] = store.SearchResult{Record: *record}
		}
	}

	// Filter results if predicate provided
	if q.Filter != nil && len(results) > 0 {
		filtered := make([]store.SearchResult, 0, len(results))
		for _, result := range results {
			if s.evaluatePredicate(&result.Record, q.Filter).toBool() {
				filtered = append(filtered, result)
			}
		}
		results = filtered
	}

	return results, nil
}

// List returns live records matching the filter.
func (s *Store) List(ctx context.Context, f store.Filter) ([]store.Record, error) {
	if s.txState != nil {
		return s.txList(ctx, f)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	records, err := s.listLocked(f)
	if err != nil {
		return nil, err
	}

	result := make([]store.Record, len(records))
	for i, record := range records {
		result[i] = *record
	}

	return result, nil
}

// Iterate streams live records matching the filter via channels.
func (s *Store) Iterate(ctx context.Context, f store.Filter) (<-chan store.Record, <-chan error) {
	recordCh := make(chan store.Record, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(recordCh)
		defer close(errCh)

		if s.txState != nil {
			records, err := s.txList(ctx, f)
			if err != nil {
				errCh <- err
				return
			}
			for _, record := range records {
				select {
				case <-ctx.Done():
					errCh <- ctx.Err()
					return
				case recordCh <- record:
				}
			}
			return
		}

		s.mu.RLock()
		records, err := s.listLocked(f)
		s.mu.RUnlock()

		if err != nil {
			errCh <- err
			return
		}

		for _, record := range records {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case recordCh <- *record:
			}
		}
	}()

	return recordCh, errCh
}

// Count returns the number of live records matching the predicate.
func (s *Store) Count(ctx context.Context, p *store.Predicate) (int64, error) {
	if s.txState != nil {
		return s.txCount(ctx, p)
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	records, err := s.listLocked(store.Filter{Where: p})
	if err != nil {
		return 0, err
	}

	return int64(len(records)), nil
}

func (s *Store) listLocked(f store.Filter) ([]*store.Record, error) {
	var results []*store.Record

	// Collect all live records
	for id, record := range s.records {
		if s.deleted[id] {
			continue
		}
		if f.Where == nil || s.evaluatePredicate(record, f.Where).toBool() {
			results = append(results, record)
		}
	}

	// Apply sorting
	if len(f.Order) > 0 {
		s.sortRecords(results, f.Order)
	}

	// Apply limit and offset
	limit := f.Limit
	if limit <= 0 || limit > s.config.MaxResultSize {
		limit = s.config.MaxResultSize
	}

	offset := f.Offset
	if offset < 0 {
		offset = 0
	}

	if offset >= len(results) {
		return nil, nil
	}

	end := offset + limit
	if end > len(results) {
		end = len(results)
	}

	return results[offset:end], nil
}

func (s *Store) searchText(text string, limit int) []*store.Record {
	if limit <= 0 {
		limit = s.config.MaxResultSize
	}

	var results []*store.Record
	lowerText := strings.ToLower(text)

	for id, record := range s.records {
		if s.deleted[id] {
			continue
		}
		if strings.Contains(strings.ToLower(record.Content), lowerText) {
			results = append(results, record)
		}
		if len(results) >= limit {
			break
		}
	}

	return results
}
