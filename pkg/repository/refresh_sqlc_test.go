package repository

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang-rest-api-template/internal/pgtest"
	"golang-rest-api-template/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newRefreshStore(t *testing.T) *SQLCRefreshTokenStore {
	t.Helper()
	return NewSQLCRefreshTokenStore(pgtest.Pool(t))
}

func TestSQLCRefreshTokenStoreLifecycle(t *testing.T) {
	store := newRefreshStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	row := &models.RefreshToken{
		UserID:    1,
		TokenHash: "abc123hash",
		FamilyID:  "family-1",
		ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, store.Create(context.Background(), row))
	assert.NotZero(t, row.ID)

	found, err := store.FindByHash(context.Background(), "abc123hash")
	require.NoError(t, err)
	assert.Equal(t, uint(1), found.UserID)

	next := &models.RefreshToken{
		UserID:    1,
		TokenHash: "next-hash",
		FamilyID:  "family-1",
		ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, store.RotateAtomically(context.Background(), found.ID, now, next))

	found, err = store.FindByHash(context.Background(), "abc123hash")
	require.NoError(t, err)
	require.NotNil(t, found.ConsumedAt)

	_, err = store.FindByHash(context.Background(), "next-hash")
	require.NoError(t, err)

	err = store.RotateAtomically(context.Background(), found.ID, now, &models.RefreshToken{
		UserID: 1, TokenHash: "again", FamilyID: "family-1", ExpiresAt: now.Add(time.Hour),
	})
	assert.ErrorIs(t, err, ErrRefreshAlreadyConsumed)

	require.NoError(t, store.RevokeFamily(context.Background(), "family-1", now))
	found, err = store.FindByHash(context.Background(), "abc123hash")
	require.NoError(t, err)
	require.NotNil(t, found.RevokedAt)
}

func TestSQLCRefreshTokenFindByHashNotFound(t *testing.T) {
	store := newRefreshStore(t)
	_, err := store.FindByHash(context.Background(), "missing")
	assert.True(t, IsNotFound(err))
}

func TestSQLCRefreshTokenRevokeAllForUser(t *testing.T) {
	store := newRefreshStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	require.NoError(t, store.Create(context.Background(), &models.RefreshToken{
		UserID: 7, TokenHash: "u7-a", FamilyID: "f-a", ExpiresAt: now.Add(time.Hour),
	}))
	require.NoError(t, store.Create(context.Background(), &models.RefreshToken{
		UserID: 7, TokenHash: "u7-b", FamilyID: "f-b", ExpiresAt: now.Add(time.Hour),
	}))

	require.NoError(t, store.RevokeAllForUser(context.Background(), 7, now))

	for _, h := range []string{"u7-a", "u7-b"} {
		row, err := store.FindByHash(context.Background(), h)
		require.NoError(t, err)
		require.NotNil(t, row.RevokedAt)
	}
}

func TestSQLCRefreshTokenRotateAtomicallyConcurrent(t *testing.T) {
	store := newRefreshStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	row := &models.RefreshToken{
		UserID: 1, TokenHash: "race-hash", FamilyID: "fam-race", ExpiresAt: now.Add(time.Hour),
	}
	require.NoError(t, store.Create(context.Background(), row))

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
			errs <- store.RotateAtomically(context.Background(), row.ID, now, next)
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
