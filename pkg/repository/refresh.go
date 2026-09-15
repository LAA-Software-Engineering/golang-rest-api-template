package repository

import (
	"context"
	"errors"
	"time"

	"golang-rest-api-template/pkg/models"
)

// ErrRefreshAlreadyConsumed is returned when a conditional consume loses a race
// (token was already consumed or revoked).
var ErrRefreshAlreadyConsumed = errors.New("repository: refresh token already consumed")

// RefreshTokenPersistence stores opaque refresh tokens (hashed) for rotation and revocation.
type RefreshTokenPersistence interface {
	Create(ctx context.Context, token *models.RefreshToken) error
	FindByHash(ctx context.Context, tokenHash string) (*models.RefreshToken, error)
	// RotateAtomically marks oldID consumed only if still active and inserts next
	// in one transaction. Returns ErrRefreshAlreadyConsumed if the conditional
	// consume does not update exactly one row.
	RotateAtomically(ctx context.Context, oldID uint, at time.Time, next *models.RefreshToken) error
	RevokeFamily(ctx context.Context, familyID string, at time.Time) error
	RevokeAllForUser(ctx context.Context, userID uint, at time.Time) error
}
