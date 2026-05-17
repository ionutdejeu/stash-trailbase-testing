package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"path/filepath"
	"strconv"

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
