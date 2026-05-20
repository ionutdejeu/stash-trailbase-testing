package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/pressly/goose/v3"
	_ "modernc.org/sqlite"
)

type discardLogger struct{}

func (discardLogger) Printf(string, ...any) {}
func (discardLogger) Fatalf(string, ...any) {}

//go:embed migrations/*.sql
var embedMigrations embed.FS

// Open creates a SQLite connection suitable for a TrailBase main database,
// runs migrations, and validates the embedding model metadata.
func Open(ctx context.Context, dsn string, expectedModel string, vectorDim int) (*sql.DB, error) {
	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}

	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if _, err := sqlDB.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}
	if _, err := sqlDB.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("set busy_timeout: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("sql.PingContext: %w", err)
	}

	goose.SetBaseFS(embedMigrations)
	goose.SetLogger(discardLogger{})

	if err := goose.SetDialect("sqlite3"); err != nil {
		return nil, fmt.Errorf("goose.SetDialect: %w", err)
	}

	if err := baselineExistingSchema(ctx, sqlDB); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("baseline goose history: %w", err)
	}

	if err := ensureParentDir(dsn); err != nil {
		sqlDB.Close()
		return nil, err
	}

	if err := goose.Up(sqlDB, "migrations"); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("goose.Up: %w", err)
	}

	if err := validateDimensionLock(ctx, sqlDB, vectorDim); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("dimension lock: %w", err)
	}

	if err := storeEmbeddingModelMetadata(ctx, sqlDB, expectedModel); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("store embedding model metadata: %w", err)
	}

	return sqlDB, nil
}

// validateDimensionLock ensures the vector dimension stored in the database matches the config.
// This is the actual storage constraint; the specific embedding model can vary as long as dimensions match.
func validateDimensionLock(ctx context.Context, db *sql.DB, expectedDim int) error {
	var storedDimStr string
	err := db.QueryRowContext(ctx,
		"SELECT value FROM settings WHERE key = 'vector_dimension'",
	).Scan(&storedDimStr)

	if err != nil {
		if err != sql.ErrNoRows {
			return err
		}
		_, err := db.ExecContext(ctx,
			"INSERT INTO settings (key, value, updated_at) VALUES ('vector_dimension', $1, CURRENT_TIMESTAMP) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP",
			fmt.Sprintf("%d", expectedDim),
		)
		return err
	}

	if storedDimStr != "" {
		storedDim, err := strconv.Atoi(storedDimStr)
		if err != nil {
			return fmt.Errorf("invalid stored dimension value: %w", err)
		}
		if storedDim != expectedDim {
			return fmt.Errorf("vector dimension mismatch: database has %d, config expects %d. You can switch between different embedding models as long as they output the same dimension. Change STASH_VECTOR_DIM to match the database, or delete the database and restart", storedDim, expectedDim)
		}
	}

	return nil
}

// storeEmbeddingModelMetadata records which embedding model is being used, for audit/monitoring purposes.
// This does not affect storage constraints (which are based on vector dimension only).
func storeEmbeddingModelMetadata(ctx context.Context, db *sql.DB, model string) error {
	_, err := db.ExecContext(ctx,
		"INSERT INTO settings (key, value, updated_at) VALUES ('embedding_model', $1, CURRENT_TIMESTAMP) ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = CURRENT_TIMESTAMP",
		model,
	)
	return err
}

func ensureParentDir(dsn string) error {
	dir := filepath.Dir(dsn)
	if dir == "." || dir == "" {
		return nil
	}
	return nil
}

func baselineExistingSchema(ctx context.Context, db *sql.DB) error {
	// Only baseline databases that already contain the Stash schema.
	hasNamespaces, err := tableExists(ctx, db, "namespaces")
	if err != nil {
		return err
	}
	hasFacts, err := tableExists(ctx, db, "facts")
	if err != nil {
		return err
	}
	hasSettings, err := tableExists(ctx, db, "settings")
	if err != nil {
		return err
	}
	hasStructuredFacts, err := columnExists(ctx, db, "facts", "entity")
	if err != nil {
		return err
	}
	hasDecayCheckpoint, err := columnExists(ctx, db, "consolidation_progress", "last_decay_run")
	if err != nil {
		return err
	}
	hasGoalCheckpoint, err := columnExists(ctx, db, "consolidation_progress", "last_goal_progress_fact_id")
	if err != nil {
		return err
	}
	if !hasNamespaces || !hasFacts || !hasSettings || !hasStructuredFacts || !hasDecayCheckpoint || !hasGoalCheckpoint {
		return nil
	}

	if _, err := goose.EnsureDBVersionContext(ctx, db); err != nil {
		return err
	}

	versions, err := embeddedMigrationVersions()
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		return nil
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, version := range versions {
		if version == 0 {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			fmt.Sprintf("INSERT INTO %s (version_id, is_applied) SELECT ?, ? WHERE NOT EXISTS (SELECT 1 FROM %s WHERE version_id = ? AND is_applied = ?)", goose.TableName(), goose.TableName()),
			version,
			true,
			version,
			true,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func tableExists(ctx context.Context, db *sql.DB, tableName string) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx,
		"SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type='table' AND name = ?)",
		tableName,
	).Scan(&exists)
	return exists, err
}

func columnExists(ctx context.Context, db *sql.DB, tableName string, columnName string) (bool, error) {
	rows, err := db.QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return false, err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var columnType string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == columnName {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

func embeddedMigrationVersions() ([]int64, error) {
	entries, err := fs.ReadDir(embedMigrations, "migrations")
	if err != nil {
		return nil, err
	}

	versions := make([]int64, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".sql") {
			continue
		}
		prefix := name
		if idx := strings.IndexByte(name, '_'); idx >= 0 {
			prefix = name[:idx]
		}
		version, err := strconv.ParseInt(prefix, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse embedded migration version %q: %w", name, err)
		}
		versions = append(versions, version)
	}

	sort.Slice(versions, func(i, j int) bool {
		return versions[i] < versions[j]
	})
	return versions, nil
}
