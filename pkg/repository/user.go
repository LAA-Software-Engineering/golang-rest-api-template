package repository

import "golang-rest-api-template/pkg/models"

// UserPersistence loads and stores users without HTTP or Gin.
type UserPersistence interface {
	FindByUsername(username string) (*models.User, error)
	Create(user *models.User) error
}
