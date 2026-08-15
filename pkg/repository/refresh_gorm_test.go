package repository

import (
	"path/filepath"
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

	require.NoError(t, store.MarkConsumed(found.ID, now))
	found, err = store.FindByHash("abc123hash")
	require.NoError(t, err)
	require.NotNil(t, found.ConsumedAt)

	require.NoError(t, store.RevokeFamily("family-1", now))
	found, err = store.FindByHash("abc123hash")
	require.NoError(t, err)
	require.NotNil(t, found.RevokedAt)
}
