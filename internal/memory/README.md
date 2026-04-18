# Memory

The memory layer turns stateless LLMs into systems with persistent, retrievable memory.

## What it is

A storage-agnostic layer that stores events with embeddings and manages a working context (active thinking state). Memory sits between the store primitive and any future agent or kernel layer.

Memory does three things well:
- **Store an event** — embed content and persist it with metadata
- **Recall relevant events** — search by semantic similarity
- **Manage a working frame** — track what is actively being thought about

## What it is NOT

- Not a cognitive system
- Not an agent framework
- Not AGI

Memory is a focused primitive, not an orchestrator.

## Architecture

Memory composes two lower-layer primitives:
- **Store** (`internal/store`) — persistence primitive
- **Embedder** (`internal/embedder`) — text to vector transformer

```
memory ──→ store
        ──→ embedder
```

Lower layers know nothing about higher layers. Store knows nothing about embedder or memory. Embedder knows nothing about store or memory.

## Quick Start

```go
// Create dependencies
store, err := postgres.New(ctx, postgres.DSN("..."))
if err != nil {
    return err
}
defer store.Close()

embedder, err := embedder.NewOpenAI("https://api.openai.com/v1", apiKey, "text-embedding-3-small", 1536)
if err != nil {
    return err
}

// Create memory
mem, err := memory.New(store, embedder)
if err != nil {
    return err
}
defer mem.Close()

// Remember an event
eventID, err := mem.Remember(ctx, "user asked about the weather", map[string]any{
    "session": "abc123",
})
if err != nil {
    return err
}

// Recall relevant events
events, err := mem.Recall(ctx, "weather information", 5)
if err != nil {
    return err
}

// Manage working frame
frame, err := mem.Frame(ctx, "user is planning a trip")
if err != nil {
    return err
}
```

## Methods

### Remember

```go
func (m *Memory) Remember(ctx context.Context, content string, metadata map[string]any) (string, error)
```

Stores an event with its embedding. Generates a UUID v4 event ID and returns it on success.

- `content` must not be empty (returns `ErrEmptyContent`)
- `metadata` keys must not start with `"_memory"` (returns `ErrInvalidMetadata`)
- Embedder failure propagates as error
- Store failure propagates as error

### Recall

```go
func (m *Memory) Recall(ctx context.Context, query string, limit int) ([]Event, error)
```

Retrieves events relevant to a query. Embeds the query text, searches store by vector similarity.

- Returns at most `limit` events ordered by relevance
- Returns empty slice (not error) when nothing matches
- `limit` must be > 0 (returns `ErrInvalidLimit` if <= 0)
- Filters explicitly by `_memory.type = "event"`

### Frame

```go
func (m *Memory) Frame(ctx context.Context, input string) (Frame, error)
```

Returns the current working memory state.

- Creates a new frame if none exists (using `input` as Focus)
- Replaces the frame lazily if the existing one has expired
- Does not update Focus on every call — only when creating a new frame
- Default expiry: 1 hour from creation

### Close

```go
func (m *Memory) Close() error
```

Releases resources held by Memory.

## Storage Format

All memory data lives in `store.Record.Metadata`. System keys are namespaced under `"_memory"` to prevent collision with caller metadata.

**Event record:**
```go
map[string]any{
    "_memory": map[string]any{
        "type":      "event",
        "content":   "what happened",
        "timestamp": "2026-04-18T10:00:00Z",
    },
    // caller metadata at root — never under "_memory"
    "session": "abc123",
}
```

**Frame record (ID: `"_memory.working_frame"`):**
```go
map[string]any{
    "_memory": map[string]any{
        "type":       "frame",
        "focus":      "current topic",
        "event_ids":  []string{"evt-1", "evt-2"},
        "created_at": "2026-04-18T10:00:00Z",
        "updated_at": "2026-04-18T10:05:00Z",
        "expires_at": "2026-04-18T11:00:00Z",
    },
}
```

## Dependencies

```
github.com/openai/openai-go v1.12.0  // OpenAI SDK
github.com/google/uuid              // UUID v4 generation
```

Requires `internal/store.Store` interface and `internal/embedder.Embedder` interface.

## No Background Goroutines

Memory never spawns background goroutines. Frame expiry is handled lazily on read — `Frame()` checks expiry when called, not on a timer.