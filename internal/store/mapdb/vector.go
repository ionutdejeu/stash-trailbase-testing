package mapdb

import (
	"math"
	"sort"

	"github.com/alash3al/stash/internal/store"
)

// cosineSimilarity returns the cosine similarity between two vectors.
// Both vectors must have the same length.
func cosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float32
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	if normA == 0 || normB == 0 {
		return 0
	}

	return dot / float32(math.Sqrt(float64(normA)*float64(normB)))
}

type searchResult struct {
	record *store.Record
	score  float32
}

// searchVectors performs vector similarity search.
func (s *Store) searchVectors(vector []float32, vectorName string, limit int) []*store.Record {
	if limit <= 0 || len(vector) != s.config.VectorDim {
		return nil
	}

	entries := s.vectors[vectorName]
	if len(entries) == 0 {
		return nil
	}

	results := make([]searchResult, 0, len(entries))
	for _, entry := range entries {
		if s.deleted[entry.id] {
			continue
		}
		score := cosineSimilarity(vector, entry.vector)
		results = append(results, searchResult{entry.record, score})
	}

	// Sort by similarity (descending)
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Apply limit
	if limit > len(results) {
		limit = len(results)
	}
	if limit > s.config.MaxResultSize {
		limit = s.config.MaxResultSize
	}

	records := make([]*store.Record, limit)
	for i := 0; i < limit; i++ {
		records[i] = results[i].record
	}

	return records
}
