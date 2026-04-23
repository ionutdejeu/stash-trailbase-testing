# Task: Reflection (Memory Analysis & Reporting)

**Status:** Completed  
**Date:** 2026-04-23
**Completed:** 2026-04-23

---

## 1. Context

**Goal:** Enable periodic reflection on memory state. Answer: "What do we know? What's inconsistent? What's missing?" via a structured report.

**Why:** Memory without introspection is a black box. Reflection surfaces the state of knowledge, enabling humans to review, consolidate further, or update outdated facts. Foundation for Phase 2 maintenance loop: consolidate → detect contradictions → reflect → decide.

**What this is:** `Memory.Reflect()` method returning `ReflectionReport` with facts grouped by entity, contradictions found, gaps identified, and statistics.

**What this is NOT:**
- Auto-resolution of contradictions (caller decides)
- Confidence scoring or probability (Phase 3+)
- Automatic consolidation or deletion
- Scheduled/periodic runs (caller invokes manually)
- Semantic analysis beyond entity grouping
- Fact suggestions or merging

---

## 2. Boundaries

**In scope:**
- `ReflectionReport` type with rich metadata about memory state
- `Memory.Reflect(ctx, namespace) (*ReflectionReport, error)` method
- Entity grouping: collect all facts about each entity
- Contradiction surfacing: use existing FindContradictions()
- Gap detection: entities with few facts (threshold: 1-2 facts)
- Statistics: total facts, contradictions, entities, date ranges
- Sorting: by entity name for readable output
- Tests: 8+ cases covering different memory states

**Not in scope:**
- Confidence scoring
- Auto-actions or merging
- Scheduled reflection
- Semantic enrichment
- Temporal consolidation recommendations
- User-facing visualization (CLI is separate task)

---

## 3. Design

### 3.1 ReflectionReport Type

**New type in `internal/memory/types.go`:**

```go
// ReflectionReport summarizes the state of memory in a namespace.
// Produced by Memory.Reflect() for human review and decision-making.
type ReflectionReport struct {
	Namespace          string                    // namespace analyzed
	TotalFacts         int                       // count of facts
	TotalContradictions int                      // count of contradictions
	TotalEntities      int                       // count of unique entities
	EntitiesByName     map[string]*EntitySummary // facts grouped by entity
	Contradictions     []Contradiction           // all contradictions found
	Gaps               []EntityGap               // entities with few facts
	DateRange          *DateRange                // when facts span
	GeneratedAt        time.Time                 // when report was generated
}

// EntitySummary aggregates facts about a single entity.
type EntitySummary struct {
	Entity       string                    // entity name
	FactCount    int                       // number of facts about this entity
	Properties   map[string][]FactValue    // property → [values] (what we know)
	Contradictions int                     // contradictions involving this entity
	FirstFact    time.Time                 // oldest fact
	LastFact     time.Time                 // newest fact
	Sources      map[string]int            // source → count (where facts came from)
}

// FactValue represents a fact about entity/property.
type FactValue struct {
	Value      string     // the fact value
	FactID     string     // which fact this came from
	ValidFrom  time.Time  // when true
	ValidUntil *time.Time // when stopped being true
	Source     string     // where this came from
}

// EntityGap represents an entity with few facts (potential gap in knowledge).
type EntityGap struct {
	Entity     string // entity name
	FactCount  int    // how many facts we have
	Properties int    // how many distinct properties
}

// DateRange spans a time period.
type DateRange struct {
	From time.Time  // earliest fact
	To   *time.Time // latest fact (nil if ongoing)
}
```

### 3.2 Reflect Method

**New method in `internal/memory/memory.go`:**

```go
// Reflect produces a structured report of memory state in a namespace.
// Groups facts by entity, detects contradictions, identifies gaps.
// Used for human review: what do we know, what's inconsistent, what's missing?
//
// Reflection is observation-only: no facts are modified, no auto-actions.
// Caller reviews the report and decides what to do.
//
// Returns error only if store access fails; always returns a report (possibly empty).
func (m *Memory) Reflect(ctx context.Context, namespace string) (*ReflectionReport, error)
```

**Implementation:**

1. Query all facts in namespace
2. For each fact:
   - Extract entity, property, value, temporal info, source
   - Group by entity
   - Track statistics (count, date range, sources)
3. Find all contradictions in namespace
4. Identify gaps: entities with < 2 facts
5. Sort entities by name
6. Build and return ReflectionReport

**Edge cases:**
- No facts in namespace → return empty report (not an error)
- Facts without entity/property → exclude from entity grouping (count separately?)
- Multiple values for same entity/property → list all (evolution tracking)
- Ongoing facts (nil ValidUntil) → note as "current" in report

### 3.3 Report Structure Example

```json
{
  "namespace": "user-123",
  "total_facts": 45,
  "total_contradictions": 2,
  "total_entities": 8,
  "date_range": {
    "from": "2026-04-01T00:00:00Z",
    "to": null  // ongoing
  },
  "entities_by_name": {
    "Mohamed": {
      "entity": "Mohamed",
      "fact_count": 12,
      "properties": {
        "language": [
          {
            "value": "French",
            "fact_id": "uuid-1",
            "valid_from": "2026-04-01T00:00:00Z",
            "valid_until": null,
            "source": "consolidation"
          },
          {
            "value": "Spanish",
            "fact_id": "uuid-2",
            "valid_from": "2026-04-15T00:00:00Z",
            "valid_until": "2026-04-20T00:00:00Z",
            "source": "consolidation"
          }
        ]
      },
      "contradictions": 1,
      "first_fact": "2026-04-01T00:00:00Z",
      "last_fact": "2026-04-23T00:00:00Z",
      "sources": {
        "consolidation": 12
      }
    }
  },
  "contradictions": [
    {
      "fact_id_1": "uuid-1",
      "fact_id_2": "uuid-3",
      "entity": "Mohamed",
      "property": "language",
      "value_1": "French",
      "value_2": "Spanish",
      "status": "conflict"
    }
  ],
  "gaps": [
    {
      "entity": "Ali",
      "fact_count": 1,
      "properties": 1
    }
  ],
  "generated_at": "2026-04-23T21:45:00Z"
}
```

### 3.4 Gap Detection Logic

**Threshold: entities with 1-2 facts are "gaps"** (limited knowledge)

```go
// entityGap identifies entities with few facts
func entityGap(entity string, factCount int) bool {
	return factCount <= 2  // Arbitrary threshold; tune later
}
```

Rationale: 1-2 facts about an entity = barely know them. 3+ facts = reasonable starting knowledge.

### 3.5 Source Tracking

**Count facts by source:**
- `consolidation` — synthesized from events
- `user` — manually added
- `import` — from batch import
- `other` — unknown

Helps caller understand confidence: consolidation (multiple events agree) vs user (single statement).

---

## 4. Implementation Notes

**File changes:**
- `internal/memory/types.go` — add ReflectionReport, EntitySummary, FactValue, EntityGap, DateRange
- `internal/memory/memory.go` — add Reflect method
- `internal/memory/memory_test.go` — 8+ tests
- No new dependencies

**Reflect implementation outline:**

```go
func (m *Memory) Reflect(ctx context.Context, namespace string) (*ReflectionReport, error) {
	// 1. Query all facts
	facts := queryAllFacts(ctx, namespace)
	
	// 2. Group by entity
	entities := make(map[string]*EntitySummary)
	for _, fact := range facts {
		entity := fact.Metadata["entity"]
		if entity == "" {
			continue // Skip facts without entity
		}
		if _, ok := entities[entity]; !ok {
			entities[entity] = &EntitySummary{...}
		}
		// Add fact to entity summary
		addFactToEntity(entities[entity], fact)
	}
	
	// 3. Find contradictions
	contradictions, _ := m.FindContradictions(ctx, namespace)
	
	// 4. Identify gaps
	gaps := findGaps(entities)
	
	// 5. Calculate statistics
	stats := calculateStats(facts, entities, contradictions)
	
	// 6. Build and return report
	return &ReflectionReport{
		Namespace:          namespace,
		TotalFacts:         len(facts),
		TotalContradictions: len(contradictions),
		TotalEntities:      len(entities),
		EntitiesByName:     entities,
		Contradictions:     contradictions,
		Gaps:               gaps,
		DateRange:          dateRange(facts),
		GeneratedAt:        time.Now().UTC(),
	}, nil
}
```

**Sorting:**
- Entities sorted by name alphabetically
- Properties within entity sorted by name
- Facts within property sorted by ValidFrom (oldest first)
- Gaps sorted by fact count (fewest first)

---

## 5. Acceptance Criteria

### ReflectionReport Type
- [ ] Type defined with all fields
- [ ] EntitySummary captures facts grouped by entity
- [ ] FactValue captures temporal and source metadata
- [ ] EntityGap identifies entities with <= 2 facts
- [ ] DateRange tracks earliest/latest facts

### Reflect Method
- [ ] Method exists: `Reflect(ctx, namespace) (*ReflectionReport, error)`
- [ ] Returns non-nil report even if no facts
- [ ] Groups facts by entity correctly
- [ ] Tracks all properties per entity
- [ ] Counts facts, contradictions, entities accurately
- [ ] Identifies gaps (fact_count <= 2)
- [ ] Includes contradictions from FindContradictions()
- [ ] Calculates date range (from earliest, to latest/ongoing)
- [ ] Tracks sources (consolidation, user, import)
- [ ] Sorts entities by name, facts by temporal order

### Statistics
- [ ] TotalFacts = count of facts
- [ ] TotalContradictions = from FindContradictions()
- [ ] TotalEntities = unique entities
- [ ] Date ranges correct (ongoing = nil)

### Testing
- [ ] TestReflect_EmptyNamespace: empty report
- [ ] TestReflect_SingleEntity: one entity, multiple properties
- [ ] TestReflect_MultipleEntities: multiple entities, proper grouping
- [ ] TestReflect_IncludesContradictions: contradictions surfaced
- [ ] TestReflect_IdentifiesGaps: entities with few facts marked
- [ ] TestReflect_SourceTracking: sources counted per entity
- [ ] TestReflect_TemporalOrder: facts sorted by valid time
- [ ] TestReflect_FactsWithoutEntity: graceful handling
- [ ] `go vet` and `staticcheck` pass
- [ ] No new dependencies

---

## 6. Explicit Assumptions

- Gap threshold: entities with 1-2 facts are gaps (tunable later)
- Entity/property come from fact metadata (caller responsible for setting)
- Facts without entity/property are excluded from entity grouping
- Reflection doesn't modify memory (observation-only)
- Caller is responsible for acting on report (consolidate more, update, delete)
- Date range from earliest fact to latest fact (or nil if ongoing)
- Contradictions include all overlapping incompatibilities (from FindContradictions)

---

## 7. Risks & Mitigations

| Risk | Mitigation |
|------|-----------|
| O(n) iteration on large fact sets | Typical namespaces << 10k facts; optimize later if needed |
| Facts without entity metadata | Gracefully skip in entity grouping; don't error |
| Ongoing facts (nil ValidUntil) | Treat as current in date range; track correctly |
| Report gets stale | Caller refreshes by calling Reflect again (stateless) |
| Gaps threshold too aggressive | Set conservatively (2 facts); tune based on usage |

---

## 8. Definition of Done

- Code compiles without warnings
- All 8+ tests pass
- Report structure is JSON-serializable (for future CLI output)
- EntitySummary correctly aggregates facts
- Gap detection working
- Source tracking working
- `go vet` and `staticcheck` pass
- Ready for review

---

## 9. Next Steps After 0011

- **Task 0012:** Reinforcement (facts observed repeatedly → higher confidence)
- **Phase 2 CLI:** Batch CLI commands for consolidate, reflect, contradictions
- **Phase 3:** Semantic facts, entity relationships, confidence scoring
