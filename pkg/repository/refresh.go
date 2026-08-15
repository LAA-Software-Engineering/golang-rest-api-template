package repository

import (
	"errors"
	"time"

	"golang-rest-api-template/pkg/models"
)

// ErrRefreshAlreadyConsumed is returned when a conditional consume loses a race
// (token was already consumed or revoked).
var ErrRefreshAlreadyConsumed = errors.New("repository: refresh token already consumed")

// RefreshTokenPersistence stores opaque refresh tokens (hashed) for rotation and revocation.
type RefreshTokenPersistence interface {
	Create(token *models.RefreshToken) error
	FindByHash(tokenHash string) (*models.RefreshToken, error)
	// RotateAtomically marks oldID consumed only if still active and inserts next
	// in one transaction. Returns ErrRefreshAlreadyConsumed if the conditional
	// consume does not update exactly one row.
	RotateAtomically(oldID uint, at time.Time, next *models.RefreshToken) error
	RevokeFamily(familyID string, at time.Time) error
	RevokeAllForUser(userID uint, at time.Time) error
}
