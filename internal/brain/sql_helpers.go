package brain

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

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
