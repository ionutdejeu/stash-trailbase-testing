# Task: Build `internal/memory` — The Memory Layer

**Status:** Active
**Date:** 2026-04-18

---

## 1. Context

**Goal:** Build the memory layer that turns stateless LLMs into systems with persistent, retrievable memory.

**What this is:** A storage-agnostic layer that stores events with embeddings and manages a working context (active thinking state). It sits between the store primitive and any future agent or kernel layer.

**What this is NOT:** A cognitive system. Not an agent framework. Not AGI. A focused primitive that does three things well: store an event, recall relevant events, manage a working frame.

---

## 2. Architecture

```
internal/
├── store/              # persistence primitive (already built)
├── embedder/           # NEW: text → vector service
│   ├── embedder.go     # Embedder interface only
│   ├── openai.go       # OpenAI implementation
│   └── fake.go         # Fake implementation (tests)
└── memory/             # NEW: memory layer
    ├── memory.go       # Memory concrete type + constructor + methods
    ├── types.go        # Event, Frame
    ├── errors.go       # sentinel errors
    ├── README.md       # write this first
    └── memory_test.go  # plumbing tests using fake embedder
```

**Dependency direction (one-way, strictly enforced):**

```
internal/memory    →   internal/store      ✅
internal/memory    →   internal/embedder   ✅
internal/embedder  →   (nothing internal)  ✅
internal/store     →   (nothing internal)  ✅
```

Reverse imports are never allowed. Lower layers know nothing about higher layers.

**Composition pattern:**
- **Store** = persistence primitive
- **Embedder** = text → vector transformer
- **Memory** = composer: uses store + embedder, adds memory semantics

---

## 3. Boundaries

**In scope (MVP):**
- Episodic memory: store events with embeddings, recall by semantic similarity
- Working frame: a single active state tracking current focus and recent event IDs
- `internal/embedder`: interface + OpenAI implementation + Fake implementation

**Non-goals (explicitly out):**
- LLM integration — memory is LLM-agnostic
- Agent framework — memory is a primitive, not an orchestrator
- Multi-tenant / multi-user — single memory space for MVP
- Schema changes or new tables — metadata-only storage via `store.Store`
- Semantic memory (concepts, facts) — Phase 3
- Procedural memory (skills) — Phase 4
- Graph relationships — Phase 4
- Consolidation, forgetting, decay — Phase 2
- Real-time streaming or pub/sub
- Background goroutines of any kind

---

## 4. Package: `internal/embedder`

### `embedder.go` — Interface

```go
// Embedder converts text into a fixed-dimension vector.
// Implementations: OpenAI (production), Fake (tests).
type Embedder interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    Model() string  // full model string as passed at construction
                    // e.g. "openai/text-embedding-3-small", "nomic-embed-text"
                    // used as the vector key in store.Record.Vectors
    Dims() int      // vector dimensions, e.g. 1536, 768
}
```

### File layout

```
internal/embedder/
├── embedder.go   # Embedder interface
├── openai.go     # OpenAI implementation
└── fake.go       # Fake implementation
```

### `openai.go` — OpenAI implementation

Named `OpenAI` because it uses the OpenAI Go SDK. Works with any
OpenAI-compatible endpoint: OpenAI directly, OpenRouter, Ollama,
Together, local vLLM, etc.

```go
// OpenAI uses the OpenAI-compatible SDK to generate embeddings.
// Works with any OpenAI-compatible endpoint: api.openai.com,
// openrouter.ai, local Ollama, Together, vLLM, etc.
// The model string is passed as-is to the API — no stripping or
// transformation. Use the format your endpoint expects:
//   OpenRouter:    "openai/text-embedding-3-small"
//   OpenAI direct: "text-embedding-3-small"
//   Ollama:        "nomic-embed-text"
type OpenAI struct {
    client *openai.Client
    model  string
    dims   int
}

// NewOpenAI creates an OpenAI embedder.
// baseURL: the API endpoint (e.g. "https://openrouter.ai/api/v1")
// apiKey:  the API key for the endpoint
// model:   required — the model string for this endpoint (no default)
// dims:    required — the vector dimension for this model (no default)
// Returns error if model or apiKey is empty, or dims <= 0.
func NewOpenAI(baseURL, apiKey, model string, dims int) (*OpenAI, error)
```

**model and dims are required. No defaults. No silent fallbacks.**
If the caller does not know the model or dims, construction fails with
a clear error. This prevents silent mismatch between stored vectors
and query vectors.

**SDK:** `github.com/openai/openai-go` — check latest stable version on
pkg.go.dev before adding to `go.mod`. Do not assume a version number.

**Embedding call:**
```go
resp, err := o.client.Embeddings.New(ctx, openai.EmbeddingNewParams{
    Input: openai.EmbeddingNewParamsInputUnionArrayOfStrings([]string{text}),
    Model: o.model,  // passed as-is, no transformation
})
// Convert resp.Data[0].Embedding ([]float64) to []float32
```

### `fake.go` — Fake implementation

```go
// Fake returns deterministic vectors for testing.
// Same input always produces the same output.
// No external calls. No API key required.
// NOT suitable for semantic correctness testing — only plumbing tests.
type Fake struct{}

// NewFake creates a Fake embedder.
// No configuration required — no model, no dims, no API key.
func NewFake() *Fake
// Fake.Model() returns "fake"
// Fake.Dims() returns 8
```

Implementation: hash the input string using `fnv32a`, use hash bytes
to produce 8 deterministic float32 values. Fast, no dependencies beyond
stdlib.

---

## 5. Package: `internal/memory`

### Types (`types.go`)

```go
// Event represents something that happened at a specific point in time.
// Stored as a store.Record with _memory.type = "event".
type Event struct {
    ID        string
    Content   string         // what happened (text)
    Timestamp time.Time      // when it happened
    Metadata  map[string]any // caller-provided context
                             // must not contain keys starting with "_memory"
}

// Frame represents working memory — what is actively being thought about.
// Single global frame for MVP, stored with fixed ID "_memory.working_frame".
// Stored as a store.Record with _memory.type = "frame".
type Frame struct {
    ID        string
    Focus     string    // current topic or query
    EventIDs  []string  // IDs of events currently in working memory
    CreatedAt time.Time
    UpdatedAt time.Time
    ExpiresAt time.Time // working memory is time-bounded
}
```

**Why `Frame` not `Context`:** `context.Context` is a fundamental Go
type used in every method signature. Using `Context` as both a method
name and return type in the same package creates immediate confusion for
humans and agents. `Frame` is short, distinct, and unambiguous.

### Metadata storage format

All memory data lives in `Record.Metadata`. System keys are namespaced
under `"_memory"` to prevent collision with caller-provided metadata.

```go
// Event record
map[string]any{
    "_memory": map[string]any{
        "type":        "event",
        "content":     "user asked about the weather",
        "timestamp":   "2026-04-18T10:00:00Z",
        "importance":  0.7,
        "accessed_at": "2026-04-18T10:00:00Z",
    },
    // caller metadata at root — never under "_memory"
    "session": "abc123",
    "source":  "chat",
}

// Frame record
map[string]any{
    "_memory": map[string]any{
        "type":       "frame",
        "focus":      "weather conversation",
        "event_ids":  []string{"evt-uuid-1", "evt-uuid-2"},
        "created_at": "2026-04-18T10:00:00Z",
        "updated_at": "2026-04-18T10:05:00Z",
        "expires_at": "2026-04-18T11:00:00Z",
    },
}
```

**Collision rule:** Caller metadata keys must not start with `"_memory"`.
Memory validates this on every `Remember` call and returns
`ErrInvalidMetadata` if violated.

### Vector storage

Vectors stored in `store.Record.Vectors` using the model string as the key:

```go
store.Record{
    ID:      eventID,
    Content: content,
    Vectors: map[string]store.Vector{
        m.embedder.Model(): {      // e.g. "openai/text-embedding-3-small"
            Values: vec,
            Model:  m.embedder.Model(),
        },
    },
    Metadata: metadata,
}
```

### Core type (`memory.go`)

```go
// Memory is the core memory system.
// Concrete type — not an interface.
// Extend it with new methods; do not abstract it.
type Memory struct {
    store    store.Store
    embedder embedder.Embedder  // interface, not *embedder.OpenAI
}

// New creates a Memory using the provided store and embedder.
// Both are required. Returns error if either is nil.
func New(s store.Store, e embedder.Embedder) (*Memory, error)
```

### Methods

```go
// Remember stores an event with its embedding.
// Generates a UUID v4 event ID before calling store.Put.
// Returns the generated event ID on success.
// content must not be empty.
// metadata keys must not start with "_memory" (returns ErrInvalidMetadata).
func (m *Memory) Remember(ctx context.Context, content string, metadata map[string]any) (string, error)

// Recall retrieves events relevant to a query.
// Embeds the query, searches the store by vector similarity.
// Returns at most limit events ordered by relevance.
// Returns empty slice (not error) when nothing matches.
// limit must be > 0.
func (m *Memory) Recall(ctx context.Context, query string, limit int) ([]Event, error)

// Frame returns the current working memory state.
// Creates a new frame if none exists.
// Replaces the frame (lazy) if the existing one has expired.
// input updates the Focus when a new frame is created.
// Does not start background goroutines.
func (m *Memory) Frame(ctx context.Context, input string) (Frame, error)

// Close releases any resources held by Memory.
func (m *Memory) Close() error
```

### ID strategy

- **Event IDs:** UUID v4 via `github.com/google/uuid`. Generated in
  `Remember` before `store.Put`.
- **Frame ID:** Fixed string `"_memory.working_frame"`. Single global
  frame for MVP.
- **Vector key:** `m.embedder.Model()` — the full model string as
  provided at embedder construction.

### Working frame expiry (lazy)

No background goroutines. `Frame()` handles expiry on read:

1. Call `store.Get("_memory.working_frame")`.
2. If `ErrNotFound` → create new frame, `store.Put`, return it.
3. If found and `ExpiresAt` is past → create new frame, `store.Put`
   (upsert overwrites), return new frame.
4. If found and not expired → return existing frame.

Default frame duration: **1 hour** from creation. Expose as a
constructor option if needed, but default is mandatory.

### Sentinel errors (`errors.go`)

```go
var (
    ErrEventNotFound   = errors.New("memory: event not found")
    ErrFrameExpired    = errors.New("memory: working frame has expired")
    ErrInvalidMetadata = errors.New("memory: caller metadata must not use _memory namespace")
    ErrEmptyContent    = errors.New("memory: content must not be empty")
    ErrInvalidLimit    = errors.New("memory: limit must be greater than zero")
)
```

---

## 6. Tests (`memory_test.go`)

**Philosophy:** Test plumbing correctness, not semantic correctness.
Semantic quality (does Recall return the *right* events?) is verified
via CLI scripts with a real OpenAI key and real Postgres. Go tests verify
behavior the compiler cannot: metadata correctness, error handling,
edge cases.

**Setup:** `embedder.NewFake()` + real Postgres via `testcontainers-go`.
Do not mock the store — it already works and tests should use it.

**Required test cases:**

- `Remember` stores an event; `store.Get` returns a record with correct
  `_memory` metadata structure (type, content, timestamp present).
- `Remember` returns `ErrEmptyContent` when content is empty string.
- `Remember` returns `ErrInvalidMetadata` when caller passes a
  `"_memory.*"` key in metadata.
- `Remember` returns an error when the embedder fails.
- `Remember` returns an error when the store fails.
- `Recall` returns empty slice (not error) when no events exist.
- `Recall` returns at most `limit` results.
- `Recall` returns `ErrInvalidLimit` when limit is 0 or negative.
- `Recall` returns events with correctly unmarshaled fields (ID, Content,
  Timestamp, Metadata).
- `Frame` creates a new frame when none exists.
- `Frame` returns the same frame on a second call within expiry window.
- `Frame` creates a new frame when existing one is expired.
- `Close` returns nil on a clean Memory instance.
- Concurrent `Remember` calls do not race (run with `-race` flag).

**What NOT to test in Go:**
- Whether `Recall` returns semantically relevant results.
- OpenAI API behavior beyond "embedder.Embed returns error →
  Remember propagates it."

---

## 7. Dependencies

Add to `go.mod`:

```
github.com/openai/openai-go   # OpenAI SDK — verify latest stable on pkg.go.dev
github.com/google/uuid        # UUID v4 generation
```

**Do not assume a version number for `openai-go`.** Check pkg.go.dev
for the latest stable release before adding.

No CGO. Pure Go only.

---

## 8. Order of work

Build in this exact order. Do not skip steps. Do not parallelize.

**Step 1: Write `internal/memory/README.md`.**
Two pages. What memory is, what it is not, the three methods, a usage
example (Remember → Recall → Frame). Write this before any code. If
you cannot write it clearly, the design is not ready.

**Step 2: `internal/embedder` package.**
- `embedder.go`: define `Embedder` interface with `Embed`, `Model`, `Dims`.
- `fake.go`: implement `Fake` (deterministic, 8-dim, stdlib only).
- `openai.go`: implement `OpenAI` (real SDK, model string passed as-is,
  returns error if model/apiKey empty or dims <= 0).
- `go build ./internal/embedder/...` passes.

**Step 3: Types and errors.**
- `internal/memory/types.go` — `Event`, `Frame`.
- `internal/memory/errors.go` — sentinel errors.
- Compile. Read the types as a user. Refine if anything is awkward.

**Step 4: Constructor and method signatures.**
- `internal/memory/memory.go` — `Memory` struct, `New()`, method
  signatures only (no bodies).
- Compile. Verify the API feels clean before implementing.

**Step 5: Implement methods one at a time.**
Order: `Remember` → `Recall` → `Frame` → `Close`.
After each: write the corresponding test, run it, make it pass before
moving to the next method.

**Step 6: Polish.**
- Godoc on every exported symbol in both packages.
- README example must compile as-is.
- `go vet ./internal/...` clean.
- `go test -race ./internal/memory/...` passes.

**Step 7: CLI verification script.**
- `scripts/test_memory.sh` — requires `OPENAI_API_KEY` env var.
- Runs Remember → Recall → Frame against real Postgres + real OpenAI
  (or OpenRouter).
- Verifies semantic recall: events you stored come back when you query
  for them.

---

## 9. Questions to ask before starting

Ask before writing code:

- What is the latest stable version of `github.com/openai/openai-go`?
  Check pkg.go.dev — do not assume.
- What is the default frame expiry duration? (Suggested: 1 hour. Confirm.)
- Should `Frame` update `Focus` on every call, or only when creating a
  new frame? (Suggested: only on new frame. Confirm.)
- Should `Recall` filter by `_memory.type = "event"` explicitly, or is
  searching all records acceptable for MVP?
  (Suggested: filter explicitly. Confirm.)

---

## 10. Hard rules (never violate)

- ❌ Never import `internal/memory` or `internal/embedder` from
  `internal/store`.
- ❌ Never import `internal/store` from `internal/embedder`.
- ❌ Never use `*embedder.OpenAI` as a field type in `Memory` —
  use `embedder.Embedder` (the interface).
- ❌ Never strip or transform the model string before passing to the
  SDK — pass it as-is.
- ❌ Never accept or write `_memory.*` keys in caller metadata.
- ❌ Never add methods, types, or features not in this spec without asking.
- ❌ Never claim tests pass without running them.
- ❌ Never log from inside either package — return errors.
- ❌ Never start background goroutines.
- ❌ Never use `panic` for runtime errors.
- ❌ Never say "done" until `go test -race ./internal/memory/...` passes.

---

## 11. Definition of done

- [ ] `internal/memory/README.md` written and example compiles.
- [ ] `internal/embedder` compiles: `Embedder` interface, `OpenAI` impl,
  `Fake` impl.
- [ ] `internal/memory` compiles cleanly.
- [ ] All tests in `memory_test.go` pass:
  `go test -race ./internal/memory/...`
- [ ] `go vet ./internal/...` clean.
- [ ] Godoc on every exported symbol in both packages.
- [ ] `_memory.*` keys rejected from caller metadata (validated, error
  returned).
- [ ] `scripts/test_memory.sh` runs end-to-end with real API + Postgres.
- [ ] No rule violations against `AGENTS.md`.

---

## 12. Progress notes

- [2026-04-18] Task created from architectural planning.
- [2026-04-18] Refined: storage-agnostic, metadata-only, 3-method MVP.
- [2026-04-18] Review pass: Embedder interface added, Frame replaces
  Context, `_memory` namespace, lazy expiry, minimal test suite,
  `internal/` throughout, AGI framing removed.
- [2026-04-18] Final pass: OpenAI impl named for the SDK (not the vendor),
  model string passed as-is (no stripping — breaks OpenRouter otherwise),
  `provider/model` format documented, `Model()`/`Dims()` on interface,
  vector key = `m.embedder.Model()`.
- [2026-04-18] Last corrections: no default model or dims — both required
  at construction, fail loudly if missing. Embedder split into separate
  files: `embedder.go` (interface), `openai.go` (OpenAI impl),
  `fake.go` (Fake impl).