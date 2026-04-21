# Task: Implement CLI Commands

**Status:** Ready for Execution  
**Date:** 2026-04-19

## 1. Context

**Goal:** Create a usable CLI that exposes memory functionality to end users.

**Why:** The core components (store, embedder, memory, config/bootstrap) are built but not accessible. Users need commands to store, retrieve, and manage memory. The CLI is the primary interface for a tool like Stash.

## 2. Boundaries

**In Scope:**
- Rename internal types/methods for clarity: `Frame` → `WorkingMemory`
- Implement 4 CLI commands: `remember`, `recall`, `context`, `env`
- Command design: positional args, flags, JSON support
- Update documentation with examples

**Non-Goals:**
- Advanced filtering syntax (keep recall simple)
- Bulk operations (batch import/export)
- Administrative commands (reset, backup, restore)
- HTTP API or other interfaces
- New memory features beyond existing `Remember`, `Recall`, `WorkingMemory`

**Constraints:**
- Maintain compatibility with existing `.env` configuration
- Use `urfave/cli/v3` (already in use)
- Follow AGENTS.md rules (no scope creep, clean code)
- CLI must work with real Postgres + OpenAI (not just fake embedder)

## 3. Approach & Review

**Proposed Approach:**

### Phase 1: Internal Rename (prerequisite)
1. Rename `Frame` type to `WorkingMemory` in `internal/memory/types.go`
2. Rename `Frame()` method to `WorkingMemory()` in `internal/memory/memory.go`
3. Update all references in code and tests
4. Verify tests still pass

### Phase 2: CLI Command Implementation
Implement four commands:

1. **`stash remember`** - Store an event
   ```
   stash remember "Postgres uses MVCC for concurrency control" \
     --metadata '{"topic": "databases", "source": "docs"}'
   ```
   - Positional: `content` (required)
   - Flags: `--metadata` (JSON string, optional)
   - Output: Event ID on success

2. **`stash recall`** - Search for relevant events
   ```
   stash recall "how does postgres handle concurrency?" --limit 5 --json
   ```
   - Positional: `query` (required)
   - Flags: `--limit` (default: 10), `--json` (output as JSON)
   - Output: List of events with similarity scores

3. **`stash context`** - View/update working memory
   ```
   stash context  # show current
   stash context --update "postgres internals"  # update focus
   ```
   - Flags: `--update` (string, optional - creates new working memory)
   - Output: Current working memory state

4. **`stash env`** - Already implemented, keep as-is

### Phase 3: Polish
1. Add `--help` text for each command
2. Create usage examples in README
3. Verify end-to-end flow works

**Self-Critique:**
- Internal rename creates churn but improves clarity long-term
- Minimal command set might need expansion later
- No pagination for `recall` results (simple `--limit` only)
- `--metadata` requires JSON string (could be awkward for users)

**Decision:** Proceed with minimal MVP. We can expand based on user feedback.

**Explicit Assumptions:**
1. Memory layer (`Remember`, `Recall`, `WorkingMemory()`) works correctly
2. Bootstrap system initializes components properly
3. Users have `.env` file with valid OpenAI API key and Postgres DSN
4. `urfave/cli/v3` is the right framework (already in use)

## 4. Component Specifications

### Internal Changes (`internal/memory/`)

**`types.go`:**
```go
// WorkingMemory represents working memory — what is actively being thought about.
// Single global working memory for MVP, stored with fixed ID "_memory.context".
// Stored as a store.Record with _memory.type = "context".
type WorkingMemory struct {
    ID        string
    Focus     string    // current topic or query
    EventIDs  []string  // IDs of events currently in working memory
    CreatedAt time.Time
    UpdatedAt time.Time
    ExpiresAt time.Time // working memory is time-bounded
}
```

**`memory.go`:**
```go
// WorkingMemory returns the current working memory state.
// Creates a new working memory if none exists.
// Replaces the working memory (lazy) if the existing one has expired.
// input updates the Focus when a new working memory is created.
// Does not start background goroutines.
func (m *Memory) WorkingMemory(ctx context.Context, input string) (WorkingMemory, error)
```

**`errors.go`:**
- Update `ErrFrameExpired` → `ErrContextExpired`

**Constants:**
- `frameID` → `contextID` (`"_memory.context"`)
- `typeFrame` → `typeContext` (`"context"`)
- `frameDuration` → `contextDuration` (1 hour, unchanged value)

### CLI Commands (`cmd/cli/`)

**Command structure:**
```go
&cli.Command{
    Name:   "remember",
    Usage:  "Store an event in memory",
    Action: rememberCmd,
    Flags: []cli.Flag{
        &cli.StringFlag{
            Name:  "metadata",
            Usage: "JSON metadata for the event",
        },
    },
    Args: true,
}
```

**Output formats:**
- **Human-readable default:** bullet list
- **JSON format:** With `--json` flag, output valid JSON
- **Error handling:** Clear error messages, non-zero exit code on failure

**Command details:**

**`remember` command:**
- Validates content not empty (returns `ErrEmptyContent`)
- Validates metadata JSON if provided (returns `ErrInvalidMetadata` if invalid JSON or `_memory.*` keys)
- Calls `memory.Remember()`
- Prints event ID: `Event stored: evt_uuid`

**`recall` command:**
- Validates query not empty
- Validates limit > 0 (returns `ErrInvalidLimit`)
- Calls `memory.Recall()`
- Prints results as bullet list with similarity scores (0.00-1.00)
- With `--json`: outputs `[]Event` JSON array

**`context` command:**
- Without `--update`: calls `memory.WorkingMemory("")`, prints state
- With `--update`: calls `memory.WorkingMemory(input)`, prints "Context updated"
- Shows: Focus, Event IDs (truncated if many), Expires time
- Human-readable format (not table)

## 5. Next-Step Handoff

**Implementation Notes:**
- Start with internal rename, verify tests pass
- Implement commands one at a time: remember → recall → context
- Use bootstrap context from CLI metadata (already set up in `main.go`)
- Add comprehensive flag validation
- Write integration tests for CLI commands

**Files / Areas Likely Affected:**
- `internal/memory/types.go` - rename Frame → WorkingMemory
- `internal/memory/memory.go` - rename Frame() → WorkingMemory(), update constants
- `internal/memory/errors.go` - update error names (ErrFrameExpired → ErrContextExpired)
- `internal/memory/memory_test.go` - update test references and constants
- `cmd/cli/main.go` - add command definitions
- `cmd/cli/remember.go` - new file
- `cmd/cli/recall.go` - new file  
- `cmd/cli/context.go` - new file
- `cmd/cli/env.go` - already exists
- `docs/` - update examples

**Risks / Watchouts:**
1. **Internal rename breaks something** - Run tests after each change
2. **CLI validation insufficient** - Users might pass invalid JSON, empty strings
3. **Output format unclear** - Need clear examples in help text
4. **Performance with many events** - `recall` might be slow with large dataset (out of scope for MVP)

**Verification Plan:**
1. `go test ./internal/memory/...` passes after rename
2. `go build ./cmd/cli` succeeds
3. Manual test: `stash remember → stash recall → stash context` works
4. `stash --help` shows all commands with clear usage
5. `--json` flag produces valid JSON parseable by `jq`

**Acceptance Criteria:**
1. `stash remember "content"` stores event, prints ID, exits 0
2. `stash remember ""` fails with error, exits 1
3. `stash recall "query"` returns ≤10 events with scores
4. `stash recall "query" --limit 3` returns ≤3 events
5. `stash recall "query" --json` outputs valid JSON array
6. `stash context` shows current working memory state
7. `stash context --update "focus"` updates working memory
8. `stash env` works (already implemented)
9. All existing tests pass after internal rename
10. `go vet ./cmd/cli/...` clean

## 6. Execution Steps

- [ ] **Phase 1: Internal Rename**
  - [ ] Rename `Frame` → `WorkingMemory` in `types.go`
  - [ ] Rename `Frame()` → `WorkingMemory()` in `memory.go`
  - [ ] Update constants: `frameID` → `contextID`, `typeFrame` → `typeContext`, `frameDuration` → `contextDuration`
  - [ ] Update error names in `errors.go`: `ErrFrameExpired` → `ErrContextExpired`
  - [ ] Update all references in `memory_test.go`
  - [ ] Run tests: `go test ./internal/memory/...`

- [ ] **Phase 2: CLI Commands**
  - [ ] Create `cmd/cli/remember.go` with `rememberCmd`
  - [ ] Create `cmd/cli/recall.go` with `recallCmd`
  - [ ] Create `cmd/cli/context.go` with `contextCmd`
  - [ ] Update `cmd/cli/main.go` to register commands
  - [ ] Add flag validation and error handling
  - [ ] Implement JSON output option for `recall`

- [ ] **Phase 3: Testing & Polish**
  - [ ] Build CLI: `go build ./cmd/cli`
  - [ ] Manual test with real Postgres + OpenAI
  - [ ] Update `--help` text
  - [ ] Add usage examples to README
  - [ ] Verify `go vet ./cmd/cli/...` clean

## 7. Progress Notes

- [2026-04-19] Task created from product planning discussion

## 8. Outcome

**Final Result:** A usable CLI that allows users to store events, search memory, and manage working context through simple commands. The system transitions from "collection of components" to "usable tool".