package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
)

func TestIsBookNotFound(t *testing.T) {
	assert.True(t, IsBookNotFound(ErrNotFound))
	assert.False(t, IsBookNotFound(nil))
	assert.False(t, IsBookNotFound(errors.New("other")))
	assert.True(t, IsBookNotFound(errors.Join(ErrNotFound, errors.New("wrap"))))
}

func TestIsUserNotFound(t *testing.T) {
	assert.True(t, IsUserNotFound(ErrNotFound))
	assert.False(t, IsUserNotFound(nil))
	assert.False(t, IsUserNotFound(errors.New("other")))
	assert.True(t, IsUserNotFound(errors.Join(ErrNotFound, errors.New("wrap"))))
}

func TestErrUserUsernameConflictIsDistinct(t *testing.T) {
	assert.NotNil(t, ErrUserUsernameConflict)
	assert.False(t, errors.Is(ErrUserUsernameConflict, ErrNotFound))
}

func TestMapNotFound(t *testing.T) {
	assert.ErrorIs(t, mapNotFound(pgx.ErrNoRows), ErrNotFound)
	assert.Nil(t, mapNotFound(nil))
	other := errors.New("boom")
	assert.Equal(t, other, mapNotFound(other))
}

func TestIsUsernameUniqueConstraintError(t *testing.T) {
	t.Parallel()
	assert.True(t, isUsernameUniqueConstraintError(errors.New("UNIQUE constraint failed: users.username")))
	assert.True(t, isUsernameUniqueConstraintError(errors.New(`duplicate key value violates unique constraint "users_username_key"`)))
	assert.False(t, isUsernameUniqueConstraintError(nil))
	assert.False(t, isUsernameUniqueConstraintError(errors.New("connection reset")))
}
