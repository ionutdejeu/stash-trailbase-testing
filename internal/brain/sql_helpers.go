package brain

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

var sqliteTimeLayouts = []string{
	time.RFC3339Nano,
	"2006-01-02 15:04:05.999999999 -0700 MST",
	"2006-01-02 15:04:05 -0700 MST",
	"2006-01-02 15:04:05.999999999-07:00",
	"2006-01-02 15:04:05.999999999",
	"2006-01-02 15:04:05",
}

type rowScanner interface {
	Scan(dest ...any) error
}

type rowsScanner interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
	Close() error
}

func inClause(startArg int, values []int64) (string, []any) {
	placeholders := make([]string, len(values))
	args := make([]any, len(values))
	for index, value := range values {
		placeholders[index] = fmt.Sprintf("$%d", startArg+index)
		args[index] = value
	}
	return strings.Join(placeholders, ", "), args
}

func rowsAffected(result sql.Result) (int64, error) {
	if result == nil {
		return 0, nil
	}
	return result.RowsAffected()
}

func isNoRows(err error) bool {
	return errors.Is(err, sql.ErrNoRows)
}

func parseSQLiteTime(value any) (time.Time, error) {
	switch typed := value.(type) {
	case time.Time:
		return typed, nil
	case string:
		return parseSQLiteTimeString(typed)
	case []byte:
		return parseSQLiteTimeString(string(typed))
	default:
		return time.Time{}, fmt.Errorf("unsupported SQLite time type %T", value)
	}
}

func parseOptionalSQLiteTime(value any) (*time.Time, error) {
	if value == nil {
		return nil, nil
	}
	parsed, err := parseSQLiteTime(value)
	if err != nil {
		return nil, err
	}
	return &parsed, nil
}

func parseSQLiteTimeString(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, fmt.Errorf("empty SQLite time value")
	}
	for _, layout := range sqliteTimeLayouts {
		var (
			parsed time.Time
			err    error
		)
		if strings.Contains(layout, "-07:00") || strings.HasPrefix(layout, time.RFC3339) {
			parsed, err = time.Parse(layout, value)
		} else {
			parsed, err = time.ParseInLocation(layout, value, time.UTC)
		}
		if err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported SQLite time format %q", value)
}
