package repository

import (
	"time"

	"golang-rest-api-template/pkg/models"

	"gorm.io/gorm"
)

// GormRefreshTokenStore implements RefreshTokenPersistence using GORM.
type GormRefreshTokenStore struct {
	db *gorm.DB
}

// NewGormRefreshTokenStore returns a RefreshTokenPersistence backed by db.
func NewGormRefreshTokenStore(db *gorm.DB) *GormRefreshTokenStore {
	return &GormRefreshTokenStore{db: db}
}

func (s *GormRefreshTokenStore) Create(token *models.RefreshToken) error {
	return s.db.Create(token).Error
}

func (s *GormRefreshTokenStore) FindByHash(tokenHash string) (*models.RefreshToken, error) {
	var t models.RefreshToken
	if err := s.db.Where("token_hash = ?", tokenHash).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *GormRefreshTokenStore) MarkConsumed(id uint, at time.Time) error {
	return s.db.Model(&models.RefreshToken{}).Where("id = ?", id).Updates(map[string]interface{}{
		"consumed_at": at,
	}).Error
}

func (s *GormRefreshTokenStore) RevokeFamily(familyID string, at time.Time) error {
	return s.db.Model(&models.RefreshToken{}).
		Where("family_id = ? AND revoked_at IS NULL", familyID).
		Updates(map[string]interface{}{"revoked_at": at}).Error
}

func (s *GormRefreshTokenStore) RevokeAllForUser(userID uint, at time.Time) error {
	return s.db.Model(&models.RefreshToken{}).
		Where("user_id = ? AND revoked_at IS NULL", userID).
		Updates(map[string]interface{}{"revoked_at": at}).Error
}
