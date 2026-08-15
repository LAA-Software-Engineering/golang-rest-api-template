package repository

import (
	"errors"
	"strings"

	"golang-rest-api-template/pkg/models"

	"gorm.io/gorm"
)

// Allowed book list sort columns (API query values map 1:1 to these identifiers).
var bookListSortColumns = map[string]string{
	"id":         "id",
	"title":      "title",
	"author":     "author",
	"created_at": "created_at",
	"updated_at": "updated_at",
	"owner_id":   "owner_id",
}

// GormBookStore implements BookPersistence using GORM.
type GormBookStore struct {
	db *gorm.DB
}

// NewGormBookStore returns a BookPersistence backed by db.
func NewGormBookStore(db *gorm.DB) *GormBookStore {
	return &GormBookStore{db: db}
}

func escapeLikePattern(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}

func (s *GormBookStore) List(q BookListQuery) ([]models.Book, error) {
	db := s.db.Model(&models.Book{})

	if q.TitleLike != "" {
		pattern := "%" + strings.ToLower(escapeLikePattern(q.TitleLike)) + "%"
		db = db.Where("LOWER(title) LIKE ? ESCAPE '\\'", pattern)
	}
	if q.AuthorLike != "" {
		pattern := "%" + strings.ToLower(escapeLikePattern(q.AuthorLike)) + "%"
		db = db.Where("LOWER(author) LIKE ? ESCAPE '\\'", pattern)
	}
	if q.OwnerID != nil {
		db = db.Where("owner_id = ?", *q.OwnerID)
	}

	sortCol := bookListSortColumns[q.Sort]
	if sortCol == "" {
		sortCol = "id"
	}
	order := strings.ToLower(strings.TrimSpace(q.Order))
	if order != "desc" {
		order = "asc"
	}

	if order == "desc" {
		switch sortCol {
		case "title":
			db = db.Order("title DESC")
		case "author":
			db = db.Order("author DESC")
		case "created_at":
			db = db.Order("created_at DESC")
		case "updated_at":
			db = db.Order("updated_at DESC")
		case "owner_id":
			db = db.Order("owner_id DESC")
		default:
			db = db.Order("id DESC")
		}
	} else {
		switch sortCol {
		case "title":
			db = db.Order("title ASC")
		case "author":
			db = db.Order("author ASC")
		case "created_at":
			db = db.Order("created_at ASC")
		case "updated_at":
			db = db.Order("updated_at ASC")
		case "owner_id":
			db = db.Order("owner_id ASC")
		default:
			db = db.Order("id ASC")
		}
	}

	var out []models.Book
	err := db.Offset(q.Offset).Limit(q.Limit).Find(&out).Error
	return out, err
}

func (s *GormBookStore) Create(book *models.Book) error {
	return s.db.Create(book).Error
}

func (s *GormBookStore) FirstByID(id uint) (*models.Book, error) {
	var book models.Book
	if err := s.db.First(&book, id).Error; err != nil {
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

// PatchFields updates only non-nil pointer fields (partial update).
func (s *GormBookStore) PatchFields(id uint, title, author *string) (*models.Book, error) {
	book, err := s.FirstByID(id)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{}
	if title != nil {
		updates["title"] = *title
	}
	if author != nil {
		updates["author"] = *author
	}
	if len(updates) == 0 {
		return book, nil
	}
	if err := s.db.Model(book).Updates(updates).Error; err != nil {
		return nil, err
	}
	if title != nil {
		book.Title = *title
	}
	if author != nil {
		book.Author = *author
	}
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
