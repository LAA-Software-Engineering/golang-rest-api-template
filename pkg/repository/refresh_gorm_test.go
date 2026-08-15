package repository

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"golang-rest-api-template/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestGormRefreshTokenStoreLifecycle(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "refresh.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.RefreshToken{}))

	store := NewGormRefreshTokenStore(db)
	now := time.Now().UTC().Truncate(time.Second)
	row := &models.RefreshToken{
		UserID:    1,
		TokenHash: "abc123hash",
		FamilyID:  "family-1",
		ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, store.Create(row))
	assert.NotZero(t, row.ID)

	found, err := store.FindByHash("abc123hash")
	require.NoError(t, err)
	assert.Equal(t, uint(1), found.UserID)

	next := &models.RefreshToken{
		UserID:    1,
		TokenHash: "next-hash",
		FamilyID:  "family-1",
		ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, store.RotateAtomically(found.ID, now, next))

	found, err = store.FindByHash("abc123hash")
	require.NoError(t, err)
	require.NotNil(t, found.ConsumedAt)

	_, err = store.FindByHash("next-hash")
	require.NoError(t, err)

	err = store.RotateAtomically(found.ID, now, &models.RefreshToken{
		UserID: 1, TokenHash: "again", FamilyID: "family-1", ExpiresAt: now.Add(time.Hour),
	})
	assert.ErrorIs(t, err, ErrRefreshAlreadyConsumed)

	require.NoError(t, store.RevokeFamily("family-1", now))
	found, err = store.FindByHash("abc123hash")
	require.NoError(t, err)
	require.NotNil(t, found.RevokedAt)
}

func TestGormRefreshTokenRotateAtomicallyConcurrent(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&models.RefreshToken{}))
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)

	store := NewGormRefreshTokenStore(db)
	now := time.Now().UTC().Truncate(time.Second)
	row := &models.RefreshToken{
		UserID: 1, TokenHash: "race-hash", FamilyID: "fam-race", ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, store.Create(row))

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		i := i
		go func() {
			defer wg.Done()
			next := &models.RefreshToken{
				UserID:    1,
				TokenHash: fmt.Sprintf("winner-%d", i),
				FamilyID:  "fam-race",
				ExpiresAt: now.Add(time.Hour),
			}
			errs <- store.RotateAtomically(row.ID, now, next)
		}()
	}
	wg.Wait()
	close(errs)

	var okCount, conflictCount int
	for e := range errs {
		switch {
		case e == nil:
			okCount++
		case errors.Is(e, ErrRefreshAlreadyConsumed):
			conflictCount++
		default:
			t.Fatalf("unexpected error: %v", e)
		}
	}
	assert.Equal(t, 1, okCount)
	assert.Equal(t, 7, conflictCount)
}

func TestIsNotFound(t *testing.T) {
	assert.True(t, IsNotFound(gorm.ErrRecordNotFound))
	assert.True(t, IsUserNotFound(gorm.ErrRecordNotFound))
	assert.False(t, IsNotFound(errors.New("other")))
}
