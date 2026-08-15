package repository

import (
	"errors"
	"fmt"
	"strings"

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

func (s *GormUserStore) FindByID(id uint) (*models.User, error) {
	var u models.User
	if err := s.db.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

func (s *GormUserStore) Create(user *models.User) error {
	err := s.db.Create(user).Error
	if err == nil {
		return nil
	}
	if isUsernameUniqueConstraintError(err) {
		return fmt.Errorf("%w: %v", ErrUserUsernameConflict, err)
	}
	return err
}

func isUsernameUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "unique constraint failed") {
		return true
	}
	return strings.Contains(s, "duplicate key") && strings.Contains(s, "unique")
}

// IsNotFound reports whether err is a GORM record-not-found result (users,
// refresh tokens, or other First/ lookups).
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// IsUserNotFound reports whether err is a missing-user lookup from GORM.
// Prefer IsNotFound for non-user entities; this alias is kept for call sites.
func IsUserNotFound(err error) bool {
	return IsNotFound(err)
}
