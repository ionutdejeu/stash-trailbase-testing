package brain

import (
	"context"
	"testing"
	"time"

	internaldb "github.com/ionutdejeu/stash-trailbase-testing/internal/db"
)

func TestGetOrCreateConsolidationProgressParsesSQLiteTimestamps(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := internaldb.Open(ctx, ":memory:", "test-model", 1536)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer sqlDB.Close()

	b := &Brain{pool: sqlDB}
	namespaceID, err := b.CreateNamespace(ctx, "/test", "test", "")
	if err != nil {
		t.Fatalf("CreateNamespace: %v", err)
	}

	cp, err := b.GetOrCreateConsolidationProgress(ctx, namespaceID)
	if err != nil {
		t.Fatalf("GetOrCreateConsolidationProgress initial: %v", err)
	}
	if cp.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be parsed")
	}
	if cp.LastRun != nil {
		t.Fatal("expected initial last_run to be nil")
	}

	decayRun := time.Date(2026, 5, 19, 17, 53, 4, 0, time.UTC)
	cp.LastDecayRun = &decayRun
	if err := b.SaveConsolidationProgress(ctx, *cp); err != nil {
		t.Fatalf("SaveConsolidationProgress: %v", err)
	}

	updated, err := b.GetOrCreateConsolidationProgress(ctx, namespaceID)
	if err != nil {
		t.Fatalf("GetOrCreateConsolidationProgress after save: %v", err)
	}
	if updated.LastDecayRun == nil || !updated.LastDecayRun.Equal(decayRun) {
		t.Fatalf("expected last_decay_run %v, got %v", decayRun, updated.LastDecayRun)
	}
	if updated.LastRun == nil {
		t.Fatal("expected last_run to be populated after save")
	}
	if updated.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be populated after save")
	}
}

func TestListNamespacesParsesSQLiteTimestamps(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := internaldb.Open(ctx, ":memory:", "test-model", 1536)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer sqlDB.Close()

	b := &Brain{pool: sqlDB}
	if _, err := b.CreateNamespace(ctx, "/alpha", "alpha", ""); err != nil {
		t.Fatalf("CreateNamespace /alpha: %v", err)
	}
	if _, err := b.CreateNamespace(ctx, "/beta", "beta", ""); err != nil {
		t.Fatalf("CreateNamespace /beta: %v", err)
	}

	namespaces, err := b.ListNamespaces(ctx, nil, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	if len(namespaces) < 2 {
		t.Fatalf("expected at least 2 namespaces, got %d", len(namespaces))
	}
	for _, namespace := range namespaces {
		if namespace.CreatedAt.IsZero() {
			t.Fatalf("expected created_at for namespace %q", namespace.Slug)
		}
		if namespace.UpdatedAt.IsZero() {
			t.Fatalf("expected updated_at for namespace %q", namespace.Slug)
		}
	}
}

func TestGoalScansSQLiteTimestamps(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := internaldb.Open(ctx, ":memory:", "test-model", 1536)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer sqlDB.Close()

	b := &Brain{pool: sqlDB}
	namespaceID, err := b.CreateNamespace(ctx, "/web-app", "web app", "goal test")
	if err != nil {
		t.Fatalf("CreateNamespace: %v", err)
	}

	goal, err := b.CreateGoal(ctx, namespaceID, "Build a web app", nil, 5)
	if err != nil {
		t.Fatalf("CreateGoal: %v", err)
	}
	if goal.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be parsed")
	}
	if goal.UpdatedAt.IsZero() {
		t.Fatal("expected updated_at to be parsed")
	}

	loaded, err := b.GetGoal(ctx, goal.ID)
	if err != nil {
		t.Fatalf("GetGoal: %v", err)
	}
	if loaded.CreatedAt.IsZero() || loaded.UpdatedAt.IsZero() {
		t.Fatal("expected loaded goal timestamps to be parsed")
	}

	goals, err := b.ListGoals(ctx, []string{"/web-app"}, "", nil, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("ListGoals: %v", err)
	}
	if len(goals) != 1 {
		t.Fatalf("expected 1 goal, got %d", len(goals))
	}
	if goals[0].CreatedAt.IsZero() || goals[0].UpdatedAt.IsZero() {
		t.Fatal("expected listed goal timestamps to be parsed")
	}
}

func TestFailureScansSQLiteTimestamps(t *testing.T) {
	ctx := context.Background()
	sqlDB, err := internaldb.Open(ctx, ":memory:", "test-model", 1536)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	defer sqlDB.Close()

	b := &Brain{pool: sqlDB}
	namespaceID, err := b.CreateNamespace(ctx, "/web-app", "web app", "failure test")
	if err != nil {
		t.Fatalf("CreateNamespace: %v", err)
	}

	failure, err := b.CreateFailure(ctx, namespaceID, "Building a web app", "Don't know", "yes", nil)
	if err != nil {
		t.Fatalf("CreateFailure: %v", err)
	}
	if failure.CreatedAt.IsZero() {
		t.Fatal("expected created_at to be parsed")
	}

	loaded, err := b.GetFailure(ctx, failure.ID)
	if err != nil {
		t.Fatalf("GetFailure: %v", err)
	}
	if loaded.CreatedAt.IsZero() {
		t.Fatal("expected loaded failure created_at to be parsed")
	}

	failures, err := b.ListFailures(ctx, []string{"/web-app"}, nil, Pagination{Limit: 10})
	if err != nil {
		t.Fatalf("ListFailures: %v", err)
	}
	if len(failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(failures))
	}
	if failures[0].CreatedAt.IsZero() {
		t.Fatal("expected listed failure created_at to be parsed")
	}
}
