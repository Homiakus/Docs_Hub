package repository

import (
	"database/sql"
	"errors"
)

// ErrNotFound is the storage-agnostic not-found contract exposed to the
// application layer. New repository adapters should wrap/return this sentinel
// rather than leaking driver-specific errors.
var ErrNotFound = errors.New("repository: not found")

// ErrConflict represents an integrity conflict that is independent of the
// storage driver, for example a child entity referencing a parent in another
// aggregate.
var ErrConflict = errors.New("repository: conflict")

// IsNotFound is a migration compatibility helper. The current SQLite adapters
// historically returned sql.ErrNoRows directly; recognize it here so the
// application layer does not depend on database/sql while adapters converge on
// ErrNotFound.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, sql.ErrNoRows)
}

func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}
