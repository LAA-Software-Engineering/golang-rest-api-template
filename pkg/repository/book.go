package repository

import "golang-rest-api-template/pkg/models"

// BookPersistence is persistence for books without HTTP or Gin.
type BookPersistence interface {
	List(offset, limit int) ([]models.Book, error)
	Create(book *models.Book) error
	FirstByID(id uint) (*models.Book, error)
	UpdateFields(id uint, title, author string) (*models.Book, error)
	DeleteByID(id uint) error
}
