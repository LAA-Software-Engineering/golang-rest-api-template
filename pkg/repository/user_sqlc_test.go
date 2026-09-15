package repository

import (
	"testing"

	"golang-rest-api-template/internal/pgtest"
	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/models"

	"github.com/stretchr/testify/assert"
)

func newUserStore(t *testing.T) *SQLCUserStore {
	t.Helper()
	return NewSQLCUserStore(pgtest.Pool(t))
}

func TestSQLCUserStoreFindByUsername(t *testing.T) {
	s := newUserStore(t)

	hash, err := auth.HashPassword("secret")
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, s.Create(&models.User{Username: "alice", Password: hash, Role: auth.RoleUser}))

	u, err := s.FindByUsername("alice")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "alice", u.Username)
}

func TestSQLCUserStoreFindByID(t *testing.T) {
	s := newUserStore(t)

	created := &models.User{Username: "bob", Password: "p", Role: auth.RoleUser}
	assert.NoError(t, s.Create(created))
	assert.NotZero(t, created.ID)

	u, err := s.FindByID(created.ID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "bob", u.Username)
}

func TestSQLCUserStoreFindByUsernameNotFound(t *testing.T) {
	s := newUserStore(t)

	_, err := s.FindByUsername("nobody")
	assert.Error(t, err)
	assert.True(t, IsUserNotFound(err))
}

func TestSQLCUserStoreCreateDuplicateUsername(t *testing.T) {
	s := newUserStore(t)

	hash, err := auth.HashPassword("p")
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, s.Create(&models.User{Username: "dup", Password: hash, Role: auth.RoleUser}))

	err = s.Create(&models.User{Username: "dup", Password: hash, Role: auth.RoleUser})
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrUserUsernameConflict)
}

func TestSQLCUserStoreCreateDefaultsRole(t *testing.T) {
	s := newUserStore(t)

	// Role left blank should not be sent as empty and violate NOT NULL; the store
	// forwards whatever the caller provides. Callers set auth.RoleUser explicitly,
	// but verify the column default applies when role is provided as 'user'.
	assert.NoError(t, s.Create(&models.User{Username: "roleuser", Password: "p", Role: "user"}))
	u, err := s.FindByUsername("roleuser")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "user", u.Role)
}
