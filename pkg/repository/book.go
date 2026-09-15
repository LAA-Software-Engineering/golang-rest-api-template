package repository

import (
	"context"

	"golang-rest-api-template/pkg/models"
)

//go:generate go run -mod=mod go.uber.org/mock/mockgen@v0.6.0 -destination=mock_persistence.go -package=repository golang-rest-api-template/pkg/repository BookPersistence,UserPersistence

// BookListQuery holds pagination, filter, and sort options for listing books.
type BookListQuery struct {
	Offset     int
	Limit      int
	TitleLike  string
	AuthorLike string
	OwnerID    *uint
	Sort       string // allowlisted: id, title, author, created_at, updated_at, owner_id
	Order      string // asc or desc
}

// BookPersistence is persistence for books without HTTP or Gin.
type BookPersistence interface {
	List(ctx context.Context, q BookListQuery) ([]models.Book, error)
	Create(ctx context.Context, book *models.Book) error
	FirstByID(ctx context.Context, id uint) (*models.Book, error)
	UpdateFields(ctx context.Context, id uint, title, author string) (*models.Book, error)
	PatchFields(ctx context.Context, id uint, title, author *string) (*models.Book, error)
	DeleteByID(ctx context.Context, id uint) error
}
