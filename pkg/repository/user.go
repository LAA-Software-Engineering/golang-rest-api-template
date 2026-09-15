package repository

import (
	"context"

	"golang-rest-api-template/pkg/models"
)

// UserPersistence loads and stores users without HTTP or Gin.
type UserPersistence interface {
	FindByUsername(ctx context.Context, username string) (*models.User, error)
	FindByID(ctx context.Context, id uint) (*models.User, error)
	Create(ctx context.Context, user *models.User) error
}
