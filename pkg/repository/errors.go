package repository

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrNotFound is returned by the persistence layer when a lookup matches no row.
// It decouples callers from the underlying driver (pgx) error so services can use
// errors.Is instead of matching pgx.ErrNoRows directly.
var ErrNotFound = errors.New("repository: record not found")

// ErrUserUsernameConflict is returned by UserPersistence.Create when the insert
// violates the unique username constraint. Callers should use errors.Is
// instead of matching driver-specific strings.
var ErrUserUsernameConflict = errors.New("repository: username already taken")

// mapNotFound converts a driver no-rows error into ErrNotFound, leaving other
// errors untouched.
func mapNotFound(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// IsNotFound reports whether err is a missing-row lookup (users, books, refresh
// tokens, or any other single-row query).
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsBookNotFound reports whether err is a missing-book lookup.
func IsBookNotFound(err error) bool {
	return IsNotFound(err)
}

// IsUserNotFound reports whether err is a missing-user lookup. Prefer IsNotFound
// for non-user entities; this alias is kept for call sites.
func IsUserNotFound(err error) bool {
	return IsNotFound(err)
}

// isUsernameUniqueConstraintError reports whether err is a Postgres unique
// constraint violation (SQLSTATE 23505) on the username constraint specifically.
// Scoping to the username constraint (rather than any 23505) avoids misreporting a
// future unique column's violation as a username conflict. A driver-string
// fallback covers non-pgconn errors (e.g. wrapped errors in tests).
func isUsernameUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505" && strings.Contains(pgErr.ConstraintName, "username")
	}
	s := strings.ToLower(err.Error())
	if !strings.Contains(s, "username") {
		return false
	}
	if strings.Contains(s, "unique constraint") {
		return true
	}
	return strings.Contains(s, "duplicate key") && strings.Contains(s, "unique")
}
