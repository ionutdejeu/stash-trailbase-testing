package memory

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/alash3al/stash/internal/embedder"
	"github.com/alash3al/stash/internal/store"
	storemapdb "github.com/alash3al/stash/internal/store/mapdb"
)

func startStore(t *testing.T) (store.Store, func()) {
	cfg := storemapdb.Config{
		VectorDim: 8,
	}

	s, err := storemapdb.New(cfg)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	cleanup := func() {
		s.Close()
	}

	return s, cleanup
}

func TestRemember_EmptyContent(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	_, err = mem.Remember(context.Background(), "test-ns", "", nil)
	if !errors.Is(err, ErrEmptyContent) {
		t.Errorf("expected ErrEmptyContent, got %v", err)
	}
}

func TestRemember_InvalidMetadata(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	_, err = mem.Remember(context.Background(), "test-ns", "content", map[string]any{
		"_memory.key": "value",
	})
	if !errors.Is(err, ErrInvalidMetadata) {
		t.Errorf("expected ErrInvalidMetadata, got %v", err)
	}
}

func TestRemember_StoresEvent(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	eventID, err := mem.Remember(ctx, "test-ns", "user asked about the weather", map[string]any{
		"session": "abc123",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	record, err := s.Get(ctx, eventID)
	if err != nil {
		t.Fatalf("store.Get failed: %v", err)
	}

	memMeta, ok := record.Metadata["_memory"].(map[string]any)
	if !ok {
		t.Fatal("missing _memory metadata")
	}

	if memMeta["type"] != "event" {
		t.Errorf("expected type=event, got %v", memMeta["type"])
	}
	if memMeta["content"] != "user asked about the weather" {
		t.Errorf("expected content, got %v", memMeta["content"])
	}
	if memMeta["timestamp"] == nil {
		t.Error("expected timestamp to be set")
	}

	if record.Metadata["session"] != "abc123" {
		t.Errorf("expected session metadata, got %v", record.Metadata["session"])
	}
}

func TestRemember_EmbedderError(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	failingEmbedder := &failingFakeEmbedder{}
	mem, err := New(s, failingEmbedder)
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	_, err = mem.Remember(context.Background(), "test-ns", "content", nil)
	if err == nil {
		t.Error("expected error from embedder")
	}
}

func TestRemember_StoreError(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	ctx := context.Background()
	// Replace store with failing store for test
	mem = &Memory{
		store:    &failingStore{inner: mem.store},
		embedder: mem.embedder,
	}

	_, err = mem.Remember(ctx, "test-ns", "content", nil)
	if err == nil {
		t.Error("expected error from store")
	}
}

func TestRecall_EmptyOnNoEvents(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	events, err := mem.Recall(ctx, []string{"test-ns"}, "weather", 5)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected empty slice, got %d events", len(events))
	}
}

func TestRecall_ReturnsAtMostLimit(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		_, err := mem.Remember(ctx, "test-ns", "event content", nil)
		if err != nil {
			t.Fatalf("Remember failed: %v", err)
		}
	}

	events, err := mem.Recall(ctx, []string{"test-ns"}, "event", 3)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(events) > 3 {
		t.Errorf("expected at most 3 events, got %d", len(events))
	}
}

func TestRecall_InvalidLimit(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	_, err = mem.Recall(context.Background(), []string{"test-ns"}, "query", 0)
	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit for limit=0, got %v", err)
	}

	_, err = mem.Recall(context.Background(), []string{"test-ns"}, "query", -1)
	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit for limit=-1, got %v", err)
	}
}

func TestRecall_ReturnsCorrectFields(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	eventID, err := mem.Remember(ctx, "test-ns", "test content", map[string]any{
		"session": "test-session",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	events, err := mem.Recall(ctx, []string{"test-ns"}, "test", 1)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}

	e := events[0]
	if e.ID != eventID {
		t.Errorf("expected ID %s, got %s", eventID, e.ID)
	}
	if e.Content != "test content" {
		t.Errorf("expected content, got %s", e.Content)
	}
	if e.Metadata == nil {
		t.Error("expected metadata to be set")
	}
	if e.Metadata["session"] != "test-session" {
		t.Errorf("expected session in metadata, got %v", e.Metadata["session"])
	}
}

func TestWorkingMemory_CreatesNewWorkingMemory(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	wm, err := mem.WorkingMemory(ctx, "test-ns", "weather conversation")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	if wm.ID != "test-ns:_memory.context" {
		t.Errorf("expected working memory ID, got %s", wm.ID)
	}
	if wm.Focus != "weather conversation" {
		t.Errorf("expected focus, got %s", wm.Focus)
	}
}

func TestWorkingMemory_UpdatesWhenInputProvided(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	wm1, err := mem.WorkingMemory(ctx, "test-ns", "first focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	wm2, err := mem.WorkingMemory(ctx, "test-ns", "second focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	if wm1.ID != wm2.ID {
		t.Errorf("expected same working memory ID, got %s vs %s", wm1.ID, wm2.ID)
	}
	if wm1.CreatedAt.Unix() != wm2.CreatedAt.Unix() {
		t.Errorf("expected same created_at (same second), got %v vs %v", wm1.CreatedAt, wm2.CreatedAt)
	}
	if wm2.Focus != "second focus" {
		t.Errorf("expected focus to update to 'second focus', got %s", wm2.Focus)
	}
	if !wm2.UpdatedAt.After(wm1.UpdatedAt) && wm2.UpdatedAt.Equal(wm1.UpdatedAt) {
		t.Errorf("expected updated_at to advance, got %v vs %v", wm2.UpdatedAt, wm1.UpdatedAt)
	}
}
	

func TestWorkingMemory_CreatesNewWhenExpired(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Replace store with expired store for test
	mem = &Memory{
		store:    &expiredStore{inner: mem.store},
		embedder: mem.embedder,
	}

	wm1, err := mem.WorkingMemory(ctx, "test-ns", "first focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	wm2, err := mem.WorkingMemory(ctx, "test-ns", "second focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	if wm1.ID == wm2.ID && wm1.CreatedAt.Equal(wm2.CreatedAt) {
		t.Error("expected new working memory after expiry")
	}
}

func TestClose_ReturnsNil(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}

	if err := mem.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestRemember_ConcurrentNoRace(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = mem.Remember(ctx, "test-ns", "concurrent event", nil)
		}()
	}

	wg.Wait()
}

type failingFakeEmbedder struct{}

func (f *failingFakeEmbedder) Embed(context.Context, string) ([]float32, error) {
	return nil, errors.New("embedder failed")
}

func (f *failingFakeEmbedder) Model() string {
	return "failing"
}

func (f *failingFakeEmbedder) Dims() int {
	return 8
}

// test helpers replacing NewFailing and NewExpired from memory.go

type failingStore struct {
	inner store.Store
}

func (f *failingStore) Put(ctx context.Context, r store.Record) error {
	return errors.New("store put failed")
}

func (f *failingStore) Get(ctx context.Context, id string) (store.Record, error) {
	return f.inner.Get(ctx, id)
}

func (f *failingStore) Delete(ctx context.Context, id string) error {
	return f.inner.Delete(ctx, id)
}

func (f *failingStore) Purge(ctx context.Context, id string) error {
	return f.inner.Purge(ctx, id)
}

func (f *failingStore) PutMany(ctx context.Context, rs []store.Record) error {
	return f.inner.PutMany(ctx, rs)
}

func (f *failingStore) DeleteWhere(ctx context.Context, namespaces []string, p *store.Predicate) (int64, error) {
	return f.inner.DeleteWhere(ctx, namespaces, p)
}

func (f *failingStore) Search(ctx context.Context, q store.Query) ([]store.SearchResult, error) {
	return f.inner.Search(ctx, q)
}

func (f *failingStore) List(ctx context.Context, f2 store.Filter) ([]store.Record, error) {
	return f.inner.List(ctx, f2)
}

func (f *failingStore) Iterate(ctx context.Context, f2 store.Filter) (<-chan store.Record, <-chan error) {
	return f.inner.Iterate(ctx, f2)
}

func (f *failingStore) Count(ctx context.Context, namespaces []string, p *store.Predicate) (int64, error) {
	return f.inner.Count(ctx, namespaces, p)
}

func (f *failingStore) WithTx(ctx context.Context, fn func(tx store.Store) error) error {
	return f.inner.WithTx(ctx, fn)
}

func (f *failingStore) Health(ctx context.Context) error {
	return f.inner.Health(ctx)
}

func (f *failingStore) Migrate(ctx context.Context) error {
	return f.inner.Migrate(ctx)
}

func (f *failingStore) Close() error {
	return f.inner.Close()
}

type expiredStore struct {
	inner store.Store
}

func (e *expiredStore) Get(ctx context.Context, id string) (store.Record, error) {
	r, err := e.inner.Get(ctx, id)
	if err != nil {
		return r, err
	}

	memMeta, ok := r.Metadata["_memory"].(map[string]any)
	if !ok {
		return r, nil
	}

	expiresAtStr, ok := memMeta["expires_at"].(string)
	if !ok {
		return r, nil
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		return r, nil
	}

	if time.Now().UTC().Before(expiresAt) {
		return r, nil
	}

	return store.Record{}, store.ErrNotFound
}

func (e *expiredStore) Put(ctx context.Context, r store.Record) error {
	return e.inner.Put(ctx, r)
}

func (e *expiredStore) Delete(ctx context.Context, id string) error {
	return e.inner.Delete(ctx, id)
}

func (e *expiredStore) Purge(ctx context.Context, id string) error {
	return e.inner.Purge(ctx, id)
}

func (e *expiredStore) PutMany(ctx context.Context, rs []store.Record) error {
	return e.inner.PutMany(ctx, rs)
}

func (e *expiredStore) DeleteWhere(ctx context.Context, namespaces []string, p *store.Predicate) (int64, error) {
	return e.inner.DeleteWhere(ctx, namespaces, p)
}

func (e *expiredStore) Search(ctx context.Context, q store.Query) ([]store.SearchResult, error) {
	return e.inner.Search(ctx, q)
}

func (e *expiredStore) List(ctx context.Context, f store.Filter) ([]store.Record, error) {
	return e.inner.List(ctx, f)
}

func (e *expiredStore) Iterate(ctx context.Context, f store.Filter) (<-chan store.Record, <-chan error) {
	return e.inner.Iterate(ctx, f)
}

func (e *expiredStore) Count(ctx context.Context, namespaces []string, p *store.Predicate) (int64, error) {
	return e.inner.Count(ctx, namespaces, p)
}

func (e *expiredStore) WithTx(ctx context.Context, fn func(tx store.Store) error) error {
	return e.inner.WithTx(ctx, fn)
}

func (e *expiredStore) Health(ctx context.Context) error {
	return e.inner.Health(ctx)
}

func (e *expiredStore) Migrate(ctx context.Context) error {
	return e.inner.Migrate(ctx)
}

func (e *expiredStore) Close() error {
	return e.inner.Close()
}

func TestRecallWhere_WithFilter(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Create events with different metadata
	_, err = mem.Remember(ctx, "test-ns", "high severity bug", map[string]any{
		"severity": "high",
		"component": "api",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "low priority issue", map[string]any{
		"severity": "low",
		"component": "gateway",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "high priority gateway fix", map[string]any{
		"severity": "high",
		"component": "gateway",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Test: search with single filter (severity=high)
	filter := &store.Predicate{
		Field: "metadata.severity",
		Op:    store.OpEq,
		Value: "high",
	}

	events, err := mem.RecallWhere(ctx, []string{"test-ns"}, "bug", filter, 10)
	if err != nil {
		t.Fatalf("RecallWhere failed: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected events with severity=high, got none")
	}

	// All returned events should have severity=high in metadata
	for _, e := range events {
		if e.Metadata["severity"] != "high" {
			t.Errorf("expected severity=high, got %v", e.Metadata["severity"])
		}
	}
}

func TestRecallWhere_MultipleFilters(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Create events
	_, err = mem.Remember(ctx, "test-ns", "high severity api bug", map[string]any{
		"severity": "high",
		"component": "api",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "low severity api issue", map[string]any{
		"severity": "low",
		"component": "api",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "high severity gateway issue", map[string]any{
		"severity": "high",
		"component": "gateway",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Filter: severity=high AND component=api
	filter := &store.Predicate{
		And: []store.Predicate{
			{
				Field: "metadata.severity",
				Op:    store.OpEq,
				Value: "high",
			},
			{
				Field: "metadata.component",
				Op:    store.OpEq,
				Value: "api",
			},
		},
	}

	events, err := mem.RecallWhere(ctx, []string{"test-ns"}, "bug", filter, 10)
	if err != nil {
		t.Fatalf("RecallWhere failed: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected events matching both filters, got none")
	}

	// All returned events should match both filters
	for _, e := range events {
		if e.Metadata["severity"] != "high" {
			t.Errorf("expected severity=high, got %v", e.Metadata["severity"])
		}
		if e.Metadata["component"] != "api" {
			t.Errorf("expected component=api, got %v", e.Metadata["component"])
		}
	}
}

func TestRecallWhere_NilFilter(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Create multiple events
	_, err = mem.Remember(ctx, "test-ns", "event one", map[string]any{
		"severity": "high",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, "test-ns", "event two", map[string]any{
		"severity": "low",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Call RecallWhere with nil filter (should be same as Recall)
	events, err := mem.RecallWhere(ctx, []string{"test-ns"}, "event", nil, 10)
	if err != nil {
		t.Fatalf("RecallWhere with nil filter failed: %v", err)
	}

	if len(events) != 2 {
		t.Errorf("expected 2 events, got %d", len(events))
	}
}

func TestRecallWhere_InvalidLimit(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	filter := &store.Predicate{
		Field: "metadata.severity",
		Op:    store.OpEq,
		Value: "high",
	}

	_, err = mem.RecallWhere(ctx, []string{"test-ns"}, "query", filter, 0)
	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit, got %v", err)
	}

	_, err = mem.RecallWhere(ctx, []string{"test-ns"}, "query", filter, -1)
	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit, got %v", err)
	}
}

func TestLinkEvents_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create two events
	event1ID, err := mem.Remember(ctx, ns, "Event A", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	event2ID, err := mem.Remember(ctx, ns, "Event B", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Link them
	relationID, err := mem.LinkEvents(ctx, ns, event1ID, event2ID, RelationTypeContradicts, nil)
	if err != nil {
		t.Fatalf("LinkEvents failed: %v", err)
	}

	if relationID == "" {
		t.Fatal("expected relation ID, got empty string")
	}

	// Verify relationship was stored
	rel, err := s.Get(ctx, relationID)
	if err != nil {
		t.Fatalf("relationship not found in store: %v", err)
	}

	memMeta, ok := rel.Metadata["_memory"].(map[string]any)
	if !ok {
		t.Fatal("missing _memory metadata in relationship")
	}

	if memMeta["type"] != "relationship" {
		t.Errorf("expected type=relationship, got %v", memMeta["type"])
	}
	if memMeta["from_event_id"] != event1ID {
		t.Errorf("expected from_event_id=%s, got %v", event1ID, memMeta["from_event_id"])
	}
	if memMeta["to_event_id"] != event2ID {
		t.Errorf("expected to_event_id=%s, got %v", event2ID, memMeta["to_event_id"])
	}
	if memMeta["relation_type"] != RelationTypeContradicts {
		t.Errorf("expected relation_type=%s, got %v", RelationTypeContradicts, memMeta["relation_type"])
	}
}

func TestLinkEvents_SelfLink_Error(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	eventID, err := mem.Remember(ctx, ns, "Event", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Try to link event to itself
	_, err = mem.LinkEvents(ctx, ns, eventID, eventID, RelationTypeContradicts, nil)
	if err == nil {
		t.Fatal("expected error for self-link, got nil")
	}
}

func TestLinkEvents_NonexistentEvent_Error(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	eventID, err := mem.Remember(ctx, ns, "Event", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Try to link to nonexistent event
	_, err = mem.LinkEvents(ctx, ns, eventID, "nonexistent-id", RelationTypeContradicts, nil)
	if err == nil {
		t.Fatal("expected error for nonexistent event, got nil")
	}
}

func TestFindRelated_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create three events
	event1ID, err := mem.Remember(ctx, ns, "Event A contradicts others", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	event2ID, err := mem.Remember(ctx, ns, "Event B contradicted by A", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	_, err = mem.Remember(ctx, ns, "Event C unrelated", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Link: event1 contradicts event2
	_, err = mem.LinkEvents(ctx, ns, event1ID, event2ID, RelationTypeContradicts, nil)
	if err != nil {
		t.Fatalf("LinkEvents failed: %v", err)
	}

	// Find what event1 contradicts
	relatedEvents, err := mem.FindRelated(ctx, ns, event1ID, RelationTypeContradicts)
	if err != nil {
		t.Fatalf("FindRelated failed: %v", err)
	}

	if len(relatedEvents) != 1 {
		t.Errorf("expected 1 related event, got %d", len(relatedEvents))
	}

	if relatedEvents[0].ID != event2ID {
		t.Errorf("expected related event ID %s, got %s", event2ID, relatedEvents[0].ID)
	}
}

func TestFindRelated_MultipleRelations(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create events
	eventAID, _ := mem.Remember(ctx, ns, "Event A", nil)
	eventBID, _ := mem.Remember(ctx, ns, "Event B", nil)
	eventCID, _ := mem.Remember(ctx, ns, "Event C", nil)
	eventDID, _ := mem.Remember(ctx, ns, "Event D", nil)

	// Create multiple relationships from A
	mem.LinkEvents(ctx, ns, eventAID, eventBID, RelationTypeContradicts, nil)
	mem.LinkEvents(ctx, ns, eventAID, eventCID, RelationTypeContradicts, nil)
	mem.LinkEvents(ctx, ns, eventAID, eventDID, RelationTypeCausedBy, nil) // Different type

	// Find all contradictions from A
	contradictions, err := mem.FindRelated(ctx, ns, eventAID, RelationTypeContradicts)
	if err != nil {
		t.Fatalf("FindRelated failed: %v", err)
	}

	if len(contradictions) != 2 {
		t.Errorf("expected 2 contradictions, got %d", len(contradictions))
	}

	// Find causes from A
	causes, err := mem.FindRelated(ctx, ns, eventAID, RelationTypeCausedBy)
	if err != nil {
		t.Fatalf("FindRelated failed: %v", err)
	}

	if len(causes) != 1 {
		t.Errorf("expected 1 cause, got %d", len(causes))
	}

	if causes[0].ID != eventDID {
		t.Errorf("expected event D, got %s", causes[0].ID)
	}
}

func TestLinkEvents_InvalidMetadata(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	eventAID, _ := mem.Remember(ctx, ns, "Event A", nil)
	eventBID, _ := mem.Remember(ctx, ns, "Event B", nil)

	// Try to use _memory-prefixed metadata
	_, err = mem.LinkEvents(ctx, ns, eventAID, eventBID, RelationTypeContradicts, map[string]any{
		"_memory.foo": "bar",
	})

	if !errors.Is(err, ErrInvalidMetadata) {
		t.Errorf("expected ErrInvalidMetadata, got %v", err)
	}
}

func TestRememberWithTTL_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	eventID, err := mem.RememberWithTTL(ctx, ns, "temporary event", 1*time.Hour, nil)
	if err != nil {
		t.Fatalf("RememberWithTTL failed: %v", err)
	}

	if eventID == "" {
		t.Fatal("expected event ID, got empty string")
	}

	// Verify event was stored with expiration
	record, err := s.Get(ctx, eventID)
	if err != nil {
		t.Fatalf("store.Get failed: %v", err)
	}

	memMeta, ok := record.Metadata["_memory"].(map[string]any)
	if !ok {
		t.Fatal("missing _memory metadata")
	}

	if memMeta["expires_at"] == nil {
		t.Fatal("expires_at not set in metadata")
	}

	// Parse and verify expires_at is roughly 1 hour in future
	expiresAtStr, ok := memMeta["expires_at"].(string)
	if !ok {
		t.Fatal("expires_at is not a string")
	}

	expiresAt, err := time.Parse(time.RFC3339, expiresAtStr)
	if err != nil {
		t.Fatalf("failed to parse expires_at: %v", err)
	}

	now := time.Now().UTC()
	if expiresAt.Before(now) || expiresAt.After(now.Add(2*time.Hour)) {
		t.Errorf("expires_at is not approximately 1 hour from now: %v", expiresAt)
	}
}

func TestRememberWithTTL_InvalidTTL(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Zero TTL
	_, err = mem.RememberWithTTL(ctx, ns, "content", 0, nil)
	if err == nil {
		t.Fatal("expected error for zero TTL, got nil")
	}

	// Negative TTL
	_, err = mem.RememberWithTTL(ctx, ns, "content", -1*time.Hour, nil)
	if err == nil {
		t.Fatal("expected error for negative TTL, got nil")
	}
}

func TestRecall_FiltersExpiredEvents(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create permanent event
	permanentID, err := mem.Remember(ctx, ns, "permanent event", nil)
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	// Create very short-lived event
	expiredID, err := mem.RememberWithTTL(ctx, ns, "expired event", 10*time.Millisecond, nil)
	if err != nil {
		t.Fatalf("RememberWithTTL failed: %v", err)
	}

	// Create future-expiring event
	futureID, err := mem.RememberWithTTL(ctx, ns, "future event", 1*time.Hour, nil)
	if err != nil {
		t.Fatalf("RememberWithTTL failed: %v", err)
	}

	// Sleep to let short-lived event expire
	time.Sleep(50 * time.Millisecond)

	// Search for "event" - should return permanent and future, not expired
	events, err := mem.Recall(ctx, []string{ns}, "event", 10)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}

	// Check that expired event is not in results
	for _, e := range events {
		if e.ID == expiredID {
			t.Errorf("expired event should not be in Recall results")
		}
	}

	// Verify permanent and future are present
	foundPermanent := false
	foundFuture := false
	for _, e := range events {
		if e.ID == permanentID {
			foundPermanent = true
		}
		if e.ID == futureID {
			foundFuture = true
		}
	}

	if !foundPermanent {
		t.Error("permanent event should be in Recall results")
	}
	if !foundFuture {
		t.Error("future-expiring event should be in Recall results")
	}
}

func TestPurgeExpired_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Create some events
	permanentID, _ := mem.Remember(ctx, ns, "permanent", nil)
	expiredID1, _ := mem.RememberWithTTL(ctx, ns, "expired 1", 10*time.Millisecond, nil)
	expiredID2, _ := mem.RememberWithTTL(ctx, ns, "expired 2", 10*time.Millisecond, nil)
	futureID, _ := mem.RememberWithTTL(ctx, ns, "future", 1*time.Hour, nil)

	// Sleep to ensure expiration
	time.Sleep(50 * time.Millisecond)

	// Purge expired
	count, err := mem.PurgeExpired(ctx, []string{ns})
	if err != nil {
		t.Fatalf("PurgeExpired failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 events purged, got %d", count)
	}

	// Verify permanent and future still exist
	if _, err := s.Get(ctx, permanentID); err != nil {
		t.Errorf("permanent event should still exist: %v", err)
	}
	if _, err := s.Get(ctx, futureID); err != nil {
		t.Errorf("future event should still exist: %v", err)
	}

	// Verify expired events are gone (hard deleted)
	if _, err := s.Get(ctx, expiredID1); err == nil {
		t.Error("expired event 1 should be deleted")
	}
	if _, err := s.Get(ctx, expiredID2); err == nil {
		t.Error("expired event 2 should be deleted")
	}
}

func TestPurgeExpired_MultipleNamespaces(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()

	// Create expired events in two namespaces
	mem.RememberWithTTL(ctx, "ns1", "expired in ns1", 10*time.Millisecond, nil)
	mem.RememberWithTTL(ctx, "ns2", "expired in ns2", 10*time.Millisecond, nil)

	time.Sleep(50 * time.Millisecond)

	// Purge both namespaces
	count, err := mem.PurgeExpired(ctx, []string{"ns1", "ns2"})
	if err != nil {
		t.Fatalf("PurgeExpired failed: %v", err)
	}

	if count != 2 {
		t.Errorf("expected 2 events purged, got %d", count)
	}
}

func TestRememberMany_Success(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"

	// Prepare batch
	events := []BulkRemember{
		{Content: "Event 1", Metadata: map[string]any{"type": "note"}},
		{Content: "Event 2", Metadata: nil},
		{Content: "Event 3", Metadata: map[string]any{"priority": "high"}},
	}

	count, err := mem.RememberMany(ctx, ns, events)
	if err != nil {
		t.Fatalf("RememberMany failed: %v", err)
	}

	if count != 3 {
		t.Errorf("expected 3 events stored, got %d", count)
	}

	// Verify all events are searchable
	results, err := mem.Recall(ctx, []string{ns}, "Event", 10)
	if err != nil {
		t.Fatalf("Recall failed: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 events in recall, got %d", len(results))
	}
}

func TestRememberMany_EmptyBatch(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	count, err := mem.RememberMany(context.Background(), "test-ns", []BulkRemember{})
	if err != nil {
		t.Fatalf("RememberMany failed: %v", err)
	}

	if count != 0 {
		t.Errorf("expected 0 events, got %d", count)
	}
}

func TestRememberMany_TooLarge(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	// Create batch larger than 10k
	events := make([]BulkRemember, 10001)
	for i := 0; i < 10001; i++ {
		events[i] = BulkRemember{Content: "Event"}
	}

	_, err = mem.RememberMany(context.Background(), "test-ns", events)
	if err == nil {
		t.Fatal("expected error for batch > 10k, got nil")
	}
}

func TestRememberMany_InvalidContent(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	events := []BulkRemember{
		{Content: "Valid event"},
		{Content: ""}, // Invalid: empty
		{Content: "Another valid"},
	}

	_, err = mem.RememberMany(context.Background(), "test-ns", events)
	if err == nil {
		t.Fatal("expected error for empty content, got nil")
	}
}

func TestRememberMany_WithTTL(t *testing.T) {
	s, cleanup := startStore(t)
	defer cleanup()

	mem, err := New(s, embedder.NewFake())
	if err != nil {
		t.Fatalf("failed to create memory: %v", err)
	}
	defer mem.Close()

	ctx := context.Background()
	ns := "test-ns"
	ttl := 1 * time.Hour

	// Create one more event without TTL to verify count
	mem.Remember(ctx, ns, "Event permanent", nil)

	events := []BulkRemember{
		{Content: "Event with TTL", TTL: &ttl},
	}

	count, err := mem.RememberMany(ctx, ns, events)
	if err != nil {
		t.Fatalf("RememberMany failed: %v", err)
	}

	if count != 1 {
		t.Errorf("expected 1 event stored, got %d", count)
	}

	// Verify total count (1 + 1 original)
	allResults, _ := mem.Recall(ctx, []string{ns}, "Event", 10)
	if len(allResults) < 1 {
		t.Error("expected at least 1 result")
	}

	// Verify at least one has TTL set
	hasTTL := false
	for _, e := range allResults {
		if e.ExpiresAt != nil {
			hasTTL = true
			break
		}
	}

	if !hasTTL {
		t.Error("expected at least one event with TTL")
	}
}
