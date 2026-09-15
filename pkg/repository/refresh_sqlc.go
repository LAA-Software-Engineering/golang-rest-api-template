package repository

import (
	"context"
	"time"

	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository/dbsqlc"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SQLCRefreshTokenStore implements RefreshTokenPersistence using sqlc over pgx.
type SQLCRefreshTokenStore struct {
	pool *pgxpool.Pool
	q    *dbsqlc.Queries
}

// NewSQLCRefreshTokenStore returns a RefreshTokenPersistence backed by pool.
func NewSQLCRefreshTokenStore(pool *pgxpool.Pool) *SQLCRefreshTokenStore {
	return &SQLCRefreshTokenStore{pool: pool, q: dbsqlc.New(pool)}
}

func refreshFromDB(t dbsqlc.RefreshToken) models.RefreshToken {
	return models.RefreshToken{
		ID:         uint(t.ID),
		UserID:     uint(t.UserID),
		TokenHash:  t.TokenHash,
		FamilyID:   t.FamilyID,
		ExpiresAt:  t.ExpiresAt,
		ConsumedAt: t.ConsumedAt,
		RevokedAt:  t.RevokedAt,
		CreatedAt:  t.CreatedAt,
		UpdatedAt:  t.UpdatedAt,
	}
}

func (s *SQLCRefreshTokenStore) Create(ctx context.Context, token *models.RefreshToken) error {
	row, err := s.q.CreateRefreshToken(ctx, dbsqlc.CreateRefreshTokenParams{
		UserID:    toInt64(token.UserID),
		TokenHash: token.TokenHash,
		FamilyID:  token.FamilyID,
		ExpiresAt: token.ExpiresAt,
	})
	if err != nil {
		return err
	}
	*token = refreshFromDB(row)
	return nil
}

func (s *SQLCRefreshTokenStore) FindByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error) {
	row, err := s.q.GetRefreshTokenByHash(ctx, tokenHash)
	if err != nil {
		return nil, mapNotFound(err)
	}
	t := refreshFromDB(row)
	return &t, nil
}

// RotateAtomically conditionally consumes oldID and inserts next in one TX.
func (s *SQLCRefreshTokenStore) RotateAtomically(ctx context.Context, oldID uint, at time.Time, next *models.RefreshToken) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := s.q.WithTx(tx)
	consumedAt := at
	n, err := qtx.ConsumeRefreshTokenIfActive(ctx, dbsqlc.ConsumeRefreshTokenIfActiveParams{
		ID:         toInt64(oldID),
		ConsumedAt: &consumedAt,
	})
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrRefreshAlreadyConsumed
	}
	row, err := qtx.CreateRefreshToken(ctx, dbsqlc.CreateRefreshTokenParams{
		UserID:    toInt64(next.UserID),
		TokenHash: next.TokenHash,
		FamilyID:  next.FamilyID,
		ExpiresAt: next.ExpiresAt,
	})
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	*next = refreshFromDB(row)
	return nil
}

func (s *SQLCRefreshTokenStore) RevokeFamily(ctx context.Context, familyID string, at time.Time) error {
	revokedAt := at
	return s.q.RevokeRefreshFamily(ctx, dbsqlc.RevokeRefreshFamilyParams{
		FamilyID:  familyID,
		RevokedAt: &revokedAt,
	})
}

func (s *SQLCRefreshTokenStore) RevokeAllForUser(ctx context.Context, userID uint, at time.Time) error {
	revokedAt := at
	return s.q.RevokeAllRefreshForUser(ctx, dbsqlc.RevokeAllRefreshForUserParams{
		UserID:    toInt64(userID),
		RevokedAt: &revokedAt,
	})
}

var _ RefreshTokenPersistence = (*SQLCRefreshTokenStore)(nil)
