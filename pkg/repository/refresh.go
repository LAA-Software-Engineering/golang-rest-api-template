package repository

import (
	"time"

	"golang-rest-api-template/pkg/models"
)

// RefreshTokenPersistence stores opaque refresh tokens (hashed) for rotation and revocation.
type RefreshTokenPersistence interface {
	Create(token *models.RefreshToken) error
	FindByHash(tokenHash string) (*models.RefreshToken, error)
	MarkConsumed(id uint, at time.Time) error
	RevokeFamily(familyID string, at time.Time) error
	RevokeAllForUser(userID uint, at time.Time) error
}
