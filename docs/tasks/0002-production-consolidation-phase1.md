# Task: Production-Grade Consolidation - Phase 1

**Status:** Ready for Execution  
**Date:** 2026-04-24  
**Priority:** High  
**Estimated:** 3-5 days

## 1. Context
Consolidation is currently a prototype with correctness issues (duplicate relationships) and minimal observability. Phase 1 focuses on fixing critical correctness problems and adding basic monitoring.

**Goal:** Fix idempotency, add metrics, validate configuration.

## 2. Boundaries
- **In Scope:**
  - Idempotent relationship extraction (no duplicates)
  - Structured logging with counters
  - Configuration validation
  - Unit tests for new functionality
- **Non-Goals:**
  - Performance optimizations (Phase 2)
  - Advanced monitoring (Phase 4)
  - Graceful shutdown (Phase 3)
- **Constraints:**
  - Maintain backward compatibility where possible
  - Follow AGENTS.md rules (no logging in libraries)
  - PostgreSQL remains the only store implementation

## 3. Approach & Review

### 3.1 Idempotency & Deduplication
**Problem:** `ExtractRelationships` creates duplicate relationships on re-run.

**Solution:** Store source fact ID in relationship metadata for deduplication.

**Implementation:**
1. Modify `storeRelationship` to accept optional `sourceFactID`
2. Add `source_fact_id` to relationship metadata
3. In `ExtractRelationships`, check if relationship already exists before creating
4. Skip facts that have already been processed

**Self-Critique:**
- Storing fact ID in metadata adds ~36 bytes per relationship (UUID string)
- Alternative: Mark fact as processed (mutates facts, requires Update method)
- Chosen approach: Store fact ID - no mutation, simple deduplication

### 3.2 Basic Metrics & Logging  
**Problem:** No visibility into consolidation performance.

**Solution:** Structured logging with counters at CLI level.

**Implementation:**
1. Enhance `ConsolidationMetrics` struct with more fields
2. Return metrics from `Consolidate` and `ExtractRelationships`
3. CLI logs metrics as structured JSON
4. Add rate limit hit counter

**Self-Critique:**
- Metrics only available via logs, not Prometheus (Phase 4)
- Simple but sufficient for initial monitoring
- Follows "return errors, don't log" rule - CLI handles logging

### 3.3 Configuration Validation
**Problem:** Invalid config causes runtime errors.

**Solution:** Validate in `NewWithConfig`.

**Implementation:**
1. Add validation for:
   - `BatchSize > 0`
   - `MaxLLMCallsPerHour > 0`
   - `SimilarityThreshold` between 0 and 1
   - `Window > 0`
2. Return descriptive error messages
3. Use sensible defaults for invalid values (or fail fast)

**Self-Critique:**
- Fail-fast vs use-defaults debate
- Chosen: Fail-fast with clear error messages
- Prevents silent misconfiguration

## 4. Next-Step Handoff

### Implementation Notes
- **Idempotency:** Key change is in `storeRelationship` and relationship existence check
- **Metrics:** Brain returns metrics, CLI formats and logs
- **Validation:** Early validation prevents runtime surprises

### Files / Areas Likely Affected
- `internal/brain/brain.go` - `storeRelationship`, `ExtractRelationships`, `Consolidate`
- `internal/brain/types.go` - Enhance `ConsolidationMetrics`
- `cmd/cli/consolidate.go` - Log metrics
- `internal/brain/store/postgres/search.go` - Relationship existence check

### Risks / Watchouts
1. **Breaking change:** `storeRelationship` signature change affects callers
2. **Performance:** Checking relationship existence adds database query
3. **Data migration:** Existing duplicate relationships won't be cleaned up

### Verification Plan
1. **Idempotency test:** Run `ExtractRelationships` twice, verify no new relationships created
2. **Metrics test:** Verify logs contain expected metrics
3. **Validation test:** Invalid config returns error
4. **Integration test:** Full consolidation flow with mock LLM

### Acceptance Criteria
1. ✅ Running `ExtractRelationships` twice creates zero new relationships
2. ✅ CLI logs show `events_processed`, `facts_created`, `llm_calls`, `duration_seconds`
3. ✅ Invalid config (batch_size=0) returns error during Brain creation
4. ✅ Rate limit hits are logged as warnings
5. ✅ All existing tests pass
6. ✅ Code compiles without warnings

## 5. Execution Steps

### Step 1: Idempotent Relationship Extraction
- [ ] Modify `storeRelationship` to accept `sourceFactID string`
- [ ] Store `source_fact_id` in relationship metadata
- [ ] Add method to check if relationship exists for fact
- [ ] Update `ExtractRelationships` to skip processed facts
- [ ] Write unit test for idempotency

### Step 2: Enhanced Metrics
- [ ] Add fields to `ConsolidationMetrics`: `Duration time.Duration`, `RateLimitHits int`
- [ ] Return metrics from `Consolidate` and `ExtractRelationships`
- [ ] Update CLI to log metrics as structured JSON
- [ ] Add rate limit hit tracking

### Step 3: Configuration Validation
- [ ] Add validation to `NewWithConfig`
- [ ] Test with invalid values
- [ ] Update documentation with validation rules

### Step 4: Testing & Verification
- [ ] Write integration test for full consolidation flow
- [ ] Test idempotency with duplicate runs
- [ ] Verify metrics in logs
- [ ] Run existing test suite

## 6. Progress Notes
- [2026-04-24] Baseline commit created. Ready to start Phase 1.

## 7. Outcome
- **Final Result:** TBD
- **Lessons Learned:** TBD
- **Next Phase:** Phase 2 - Performance & Scalability