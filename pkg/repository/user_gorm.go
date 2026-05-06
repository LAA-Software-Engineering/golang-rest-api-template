package repository

import (
	"errors"

	"golang-rest-api-template/pkg/models"

	"gorm.io/gorm"
)

// GormUserStore implements UserPersistence using GORM.
type GormUserStore struct {
	db *gorm.DB
}

// NewGormUserStore returns a UserPersistence backed by db.
func NewGormUserStore(db *gorm.DB) *GormUserStore {
	return &GormUserStore{db: db}
}

func (s *GormUserStore) FindByUsername(username string) (*models.User, error) {
	var u models.User
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *GormUserStore) Create(user *models.User) error {
	return s.db.Create(user).Error
}

// IsUserNotFound reports whether err is a missing-user lookup from GORM.
func IsUserNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
