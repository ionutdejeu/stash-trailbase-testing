package brain

import (
	"context"
	"time"

	"github.com/alash3al/stash/internal/brain/store"
)

// Reflect returns a comprehensive report on memory state.
func (b *Brain) Reflect(ctx context.Context, namespace string) (*Report, error) {
	var namespaces []string
	if namespace != "" {
		namespaces = []string{namespace}
	}

	// Count events
	eventCount, err := b.store.Count(ctx, namespaces, &store.Predicate{
		Field: "metadata._memory.type",
		Op:    store.OpEq,
		Value: typeEvent,
	})
	if err != nil {
		return nil, err
	}

	// Count facts
	factCount, err := b.store.Count(ctx, namespaces, &store.Predicate{
		Field: "metadata._memory.type",
		Op:    store.OpEq,
		Value: typeFact,
	})
	if err != nil {
		return nil, err
	}

	// Count relationships
	relCount, err := b.store.Count(ctx, namespaces, &store.Predicate{
		Field: "metadata._memory.type",
		Op:    store.OpEq,
		Value: typeRelationship,
	})
	if err != nil {
		return nil, err
	}

	// Find contradictions
	contradictions, err := b.Contradict(ctx, namespace)
	if err != nil {
		return nil, err
	}

	// Build entity summaries from facts
	facts, err := b.queryFacts(ctx, namespace)
	if err != nil {
		return nil, err
	}

	entities := make(map[string]*EntitySummary)
	for _, fact := range facts {
		entity, _ := fact.Metadata["entity"].(string)
		if entity == "" {
			continue
		}

		if _, ok := entities[entity]; !ok {
			entities[entity] = &EntitySummary{
				Entity:     entity,
				Properties: make(map[string][]FactValue),
			}
		}

		entities[entity].FactCount++

		property, _ := fact.Metadata["property"].(string)
		value, _ := fact.Metadata["value"].(string)
		if property != "" && value != "" {
			entities[entity].Properties[property] = append(
				entities[entity].Properties[property],
				FactValue{
					Value:     value,
					FactID:    fact.ID,
					ValidFrom: fact.ValidFrom,
				},
			)
		}
	}

	// Count contradictions per entity
	for _, c := range contradictions {
		if es, ok := entities[c.Entity]; ok {
			es.ContradictionCount++
		}
	}

// Pipeline stats removed - consolidation is now explicit
	return &Report{
		Namespace:           namespace,
		TotalFacts:          int(factCount),
		TotalEvents:         int(eventCount),
		TotalRelationships:  int(relCount),
		TotalContradictions: len(contradictions),
		EntitiesByName:      entities,
		Contradictions:      contradictions,
		Pipeline: PipelineStatus{
			QueueDepth:  0,
			LastRun:     time.Time{},
			LastSuccess: false,
			LastError:   "consolidation is now explicit (use 'stash consolidate' commands)",
		},
		GeneratedAt: time.Now(),
	}, nil
}
