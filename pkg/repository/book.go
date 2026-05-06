package repository

import "golang-rest-api-template/pkg/models"

//go:generate go run -mod=mod github.com/golang/mock/mockgen@v1.6.0 -destination=mock_persistence.go -package=repository golang-rest-api-template/pkg/repository BookPersistence,UserPersistence

// BookPersistence is persistence for books without HTTP or Gin.
type BookPersistence interface {
	List(offset, limit int) ([]models.Book, error)
	Create(book *models.Book) error
	FirstByID(id uint) (*models.Book, error)
	UpdateFields(id uint, title, author string) (*models.Book, error)
	DeleteByID(id uint) error
}
