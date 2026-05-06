package repository

import (
	"testing"

	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/models"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testUserDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	if !assert.NoError(t, db.AutoMigrate(&models.User{})) {
		t.FailNow()
	}
	return db
}

func TestGormUserStoreFindByUsername(t *testing.T) {
	db := testUserDB(t)
	s := NewGormUserStore(db)

	hash, err := auth.HashPassword("secret")
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, s.Create(&models.User{Username: "alice", Password: hash}))

	u, err := s.FindByUsername("alice")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "alice", u.Username)
}

func TestGormUserStoreFindByUsernameNotFound(t *testing.T) {
	db := testUserDB(t)
	s := NewGormUserStore(db)

	_, err := s.FindByUsername("nobody")
	assert.Error(t, err)
	assert.True(t, IsUserNotFound(err))
}

func TestGormUserStoreCreateDuplicateUsername(t *testing.T) {
	db := testUserDB(t)
	s := NewGormUserStore(db)

	hash, err := auth.HashPassword("p")
	if !assert.NoError(t, err) {
		return
	}
	assert.NoError(t, s.Create(&models.User{Username: "dup", Password: hash}))

	err = s.Create(&models.User{Username: "dup", Password: hash})
	assert.Error(t, err)
}
