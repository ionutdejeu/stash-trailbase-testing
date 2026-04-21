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

	_, err = mem.Remember(context.Background(), "", nil)
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

	_, err = mem.Remember(context.Background(), "content", map[string]any{
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
	eventID, err := mem.Remember(ctx, "user asked about the weather", map[string]any{
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

	_, err = mem.Remember(context.Background(), "content", nil)
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

	_, err = mem.Remember(ctx, "content", nil)
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
	events, err := mem.Recall(ctx, "weather", 5)
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
		_, err := mem.Remember(ctx, "event content", nil)
		if err != nil {
			t.Fatalf("Remember failed: %v", err)
		}
	}

	events, err := mem.Recall(ctx, "event", 3)
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

	_, err = mem.Recall(context.Background(), "query", 0)
	if !errors.Is(err, ErrInvalidLimit) {
		t.Errorf("expected ErrInvalidLimit for limit=0, got %v", err)
	}

	_, err = mem.Recall(context.Background(), "query", -1)
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
	eventID, err := mem.Remember(ctx, "test content", map[string]any{
		"session": "test-session",
	})
	if err != nil {
		t.Fatalf("Remember failed: %v", err)
	}

	events, err := mem.Recall(ctx, "test", 1)
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
	wm, err := mem.WorkingMemory(ctx, "weather conversation")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	if wm.ID != "_memory.context" {
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
	wm1, err := mem.WorkingMemory(ctx, "first focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	wm2, err := mem.WorkingMemory(ctx, "second focus")
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

	wm1, err := mem.WorkingMemory(ctx, "first focus")
	if err != nil {
		t.Fatalf("WorkingMemory failed: %v", err)
	}

	wm2, err := mem.WorkingMemory(ctx, "second focus")
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
			_, _ = mem.Remember(ctx, "concurrent event", nil)
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

func (f *failingStore) DeleteWhere(ctx context.Context, p *store.Predicate) (int64, error) {
	return f.inner.DeleteWhere(ctx, p)
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

func (f *failingStore) Count(ctx context.Context, p *store.Predicate) (int64, error) {
	return f.inner.Count(ctx, p)
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

func (e *expiredStore) DeleteWhere(ctx context.Context, p *store.Predicate) (int64, error) {
	return e.inner.DeleteWhere(ctx, p)
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

func (e *expiredStore) Count(ctx context.Context, p *store.Predicate) (int64, error) {
	return e.inner.Count(ctx, p)
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
