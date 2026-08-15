package repository

import "golang-rest-api-template/pkg/models"

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
	List(q BookListQuery) ([]models.Book, error)
	Create(book *models.Book) error
	FirstByID(id uint) (*models.Book, error)
	UpdateFields(id uint, title, author string) (*models.Book, error)
	PatchFields(id uint, title, author *string) (*models.Book, error)
	DeleteByID(id uint) error
}
