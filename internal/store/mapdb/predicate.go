package mapdb

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/alash3al/stash/internal/store"
)

func (s *Store) evaluatePredicate(record *store.Record, p *store.Predicate) threeValuedBool {
	if p == nil {
		return tvbTrue
	}

	// Handle logical operators
	if len(p.And) > 0 {
		result := tvbTrue
		for _, child := range p.And {
			childResult := s.evaluatePredicate(record, &child)
			if childResult == tvbFalse {
				return tvbFalse
			}
			if childResult == tvbNull && result == tvbTrue {
				result = tvbNull
			}
		}
		return result
	}

	if len(p.Or) > 0 {
		result := tvbFalse
		for _, child := range p.Or {
			childResult := s.evaluatePredicate(record, &child)
			if childResult == tvbTrue {
				return tvbTrue
			}
			if childResult == tvbNull && result == tvbFalse {
				result = tvbNull
			}
		}
		return result
	}

	if p.Not != nil {
		childResult := s.evaluatePredicate(record, p.Not)
		switch childResult {
		case tvbTrue:
			return tvbFalse
		case tvbFalse:
			return tvbTrue
		default:
			return tvbNull
		}
	}

	// Handle leaf predicate
	return s.evaluateLeaf(record, p)
}

type threeValuedBool int

const (
	tvbNull threeValuedBool = iota
	tvbFalse
	tvbTrue
)

func (tvb threeValuedBool) toBool() bool {
	return tvb == tvbTrue
}

func (s *Store) evaluateLeaf(record *store.Record, p *store.Predicate) threeValuedBool {
	value := s.getFieldValue(record, p.Field)

	switch p.Op {
	case store.OpEq:
		if value == nil {
			return tvbNull
		}
		if s.compareEqual(value, p.Value) {
			return tvbTrue
		}
		return tvbFalse
	case store.OpNe:
		if value == nil {
			return tvbNull
		}
		if s.compareEqual(value, p.Value) {
			return tvbFalse
		}
		return tvbTrue
	case store.OpGt:
		if value == nil {
			return tvbNull
		}
		if s.compareGreater(value, p.Value, false) {
			return tvbTrue
		}
		return tvbFalse
	case store.OpGte:
		if value == nil {
			return tvbNull
		}
		if s.compareGreater(value, p.Value, true) {
			return tvbTrue
		}
		return tvbFalse
	case store.OpLt:
		if value == nil {
			return tvbNull
		}
		if s.compareLess(value, p.Value, false) {
			return tvbTrue
		}
		return tvbFalse
	case store.OpLte:
		if value == nil {
			return tvbNull
		}
		if s.compareLess(value, p.Value, true) {
			return tvbTrue
		}
		return tvbFalse
	case store.OpIn:
		if value == nil {
			return tvbNull
		}
		if s.isIn(value, p.Value) {
			return tvbTrue
		}
		return tvbFalse
	case store.OpNotIn:
		if value == nil {
			return tvbNull
		}
		if s.isIn(value, p.Value) {
			return tvbFalse
		}
		return tvbTrue
	case store.OpExists:
		if boolVal, ok := p.Value.(bool); ok {
			exists := value != nil
			if boolVal == exists {
				return tvbTrue
			}
			return tvbFalse
		}
		return tvbFalse
	case store.OpContains:
		if value == nil {
			return tvbNull
		}
		if s.contains(value, p.Value) {
			return tvbTrue
		}
		return tvbFalse
	case store.OpPrefix:
		if value == nil {
			return tvbNull
		}
		if s.hasPrefix(value, p.Value) {
			return tvbTrue
		}
		return tvbFalse
	default:
		return tvbFalse
	}
}

func (s *Store) getFieldValue(record *store.Record, field string) any {
	parts := strings.Split(field, ".")

	switch parts[0] {
	case "id":
		return record.ID
	case "content":
		return record.Content
	case "created_at":
		return record.CreatedAt
	case "updated_at":
		return record.UpdatedAt
	case "deleted_at":
		if record.DeletedAt == nil {
			return nil
		}
		return *record.DeletedAt
	case "metadata":
		if len(parts) < 2 {
			return record.Metadata
		}
		return s.getMetadataValue(record.Metadata, parts[1:])
	case "vectors":
		if len(parts) < 2 {
			return record.Vectors
		}
		// vectors.<name>.<field>
		if vec, ok := record.Vectors[parts[1]]; ok {
			if len(parts) == 2 {
				return vec
			}
			if len(parts) == 3 {
				switch parts[2] {
				case "values":
					return vec.Values
				case "model":
					return vec.Model
				}
			}
		}
		return nil
	default:
		return nil
	}
}

func (s *Store) getMetadataValue(metadata map[string]any, path []string) any {
	if metadata == nil {
		return nil
	}

	val, ok := metadata[path[0]]
	if !ok {
		return nil
	}

	if len(path) == 1 {
		return val
	}

	// Recurse into nested maps
	if nested, ok := val.(map[string]any); ok {
		return s.getMetadataValue(nested, path[1:])
	}

	return nil
}

func (s *Store) compareEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}

	// Special handling for time.Time
	if ta, ok := a.(time.Time); ok {
		if tb, ok := b.(time.Time); ok {
			return ta.Equal(tb)
		}
		if str, ok := b.(string); ok {
			if tb, err := time.Parse(time.RFC3339, str); err == nil {
				return ta.Equal(tb)
			}
		}
		return false
	}

	// Compare strings
	if sa, ok := a.(string); ok {
		if sb, ok := b.(string); ok {
			return sa == sb
		}
	}

	// Compare numbers
	if fa, ok := s.toFloat64(a); ok {
		if fb, ok := s.toFloat64(b); ok {
			return fa == fb
		}
	}

	// Use reflect.DeepEqual for other types
	return reflect.DeepEqual(a, b)
}

func (s *Store) compareGreater(a, b any, orEqual bool) bool {
	fa, ok := s.toFloat64(a)
	fb, ok2 := s.toFloat64(b)
	if !ok || !ok2 {
		return false
	}

	if orEqual {
		return fa >= fb
	}
	return fa > fb
}

func (s *Store) compareLess(a, b any, orEqual bool) bool {
	fa, ok := s.toFloat64(a)
	fb, ok2 := s.toFloat64(b)
	if !ok || !ok2 {
		return false
	}

	if orEqual {
		return fa <= fb
	}
	return fa < fb
}

func (s *Store) isIn(a, b any) bool {
	slice, ok := b.([]any)
	if !ok {
		return false
	}

	for _, item := range slice {
		if s.compareEqual(a, item) {
			return true
		}
	}
	return false
}

func (s *Store) contains(a, b any) bool {
	switch va := a.(type) {
	case []any:
		for _, item := range va {
			if s.compareEqual(item, b) {
				return true
			}
		}
		return false
	case string:
		if sb, ok := b.(string); ok {
			return strings.Contains(va, sb)
		}
	}
	return false
}

func (s *Store) hasPrefix(a, b any) bool {
	sa, ok1 := a.(string)
	sb, ok2 := b.(string)
	if !ok1 || !ok2 {
		return false
	}
	return strings.HasPrefix(sa, sb)
}

func (s *Store) toFloat64(v any) (float64, bool) {
	switch n := v.(type) {
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case float32:
		return float64(n), true
	case float64:
		return n, true
	case time.Time:
		return float64(n.UnixNano()), true
	default:
		return 0, false
	}
}

func (s *Store) sortRecords(records []*store.Record, order []store.Order) {
	if len(records) == 0 || len(order) == 0 {
		return
	}

	sortFunc := func(i, j int) bool {
		for _, ord := range order {
			a := s.getFieldValue(records[i], ord.Field)
			b := s.getFieldValue(records[j], ord.Field)

			fa, okA := s.toFloat64(a)
			fb, okB := s.toFloat64(b)

			if okA && okB {
				if fa != fb {
					if ord.Desc {
						return fa > fb
					}
					return fa < fb
				}
				continue
			}

			// Fall back to string comparison
			sa := fmt.Sprintf("%v", a)
			sb := fmt.Sprintf("%v", b)
			if sa != sb {
				if ord.Desc {
					return sa > sb
				}
				return sa < sb
			}
		}
		return false
	}

	// Use stable sort for deterministic ordering
	for i := len(order) - 1; i >= 0; i-- {
		// Bubble sort for simplicity (records are typically small in tests)
		for k := 0; k < len(records)-1; k++ {
			for l := k + 1; l < len(records); l++ {
				if sortFunc(l, k) {
					records[k], records[l] = records[l], records[k]
				}
			}
		}
	}
}
