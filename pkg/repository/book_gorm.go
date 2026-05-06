package repository

import (
	"errors"

	"golang-rest-api-template/pkg/database"
	"golang-rest-api-template/pkg/models"

	"gorm.io/gorm"
)

// GormBookStore implements BookPersistence using database.Database (GORM).
type GormBookStore struct {
	db database.Database
}

// NewGormBookStore returns a BookPersistence backed by db.
func NewGormBookStore(db database.Database) *GormBookStore {
	return &GormBookStore{db: db}
}

func (s *GormBookStore) List(offset, limit int) ([]models.Book, error) {
	var out []models.Book
	err := s.db.Offset(offset).Limit(limit).Find(&out).Error
	return out, err
}

func (s *GormBookStore) Create(book *models.Book) error {
	return s.db.Create(book).Error
}

func (s *GormBookStore) FirstByID(id uint) (*models.Book, error) {
	var book models.Book
	if err := s.db.FirstByID(&book, id).Error(); err != nil {
		return nil, err
	}
	return &book, nil
}

func (s *GormBookStore) UpdateFields(id uint, title, author string) (*models.Book, error) {
	book, err := s.FirstByID(id)
	if err != nil {
		return nil, err
	}
	if err := s.db.Model(book).Updates(models.Book{Title: title, Author: author}).Error; err != nil {
		return nil, err
	}
	book.Title = title
	book.Author = author
	return book, nil
}

func (s *GormBookStore) DeleteByID(id uint) error {
	book, err := s.FirstByID(id)
	if err != nil {
		return err
	}
	return s.db.Delete(book).Error
}

// IsBookNotFound reports whether err is a missing-book lookup from GORM.
func IsBookNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
