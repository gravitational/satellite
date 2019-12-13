package sqlite

import (
	"github.com/gravitational/trace"
	"github.com/mattn/go-sqlite3"
)

// isErrConstraintUnique returns true if error is a
// sqlite3.ErrorConstraintUnique error.
func isErrConstraintUnique(err error) bool {
	err = trace.Unwrap(err)
	sqliteErr, ok := err.(sqlite3.Error)
	if !ok {
		return false
	}
	return sqliteErr.ExtendedCode == sqlite3.ErrConstraintUnique
}
