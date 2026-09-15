package repository

import (
	"context"
	"fmt"
	"strings"

	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository/dbsqlc"

	"github.com/jackc/pgx/v5/pgxpool"
)

// BookListSortFields is the allowlisted set of sort query values for listing books.
var BookListSortFields = map[string]struct{}{
	"id": {}, "title": {}, "author": {}, "created_at": {}, "updated_at": {}, "owner_id": {},
}

// Static ORDER BY clauses keyed by "field|asc" / "field|desc" (no user-string concat).
var bookListOrderBy = map[string]string{
	"id|asc":          "id ASC",
	"id|desc":         "id DESC",
	"title|asc":       "title ASC",
	"title|desc":      "title DESC",
	"author|asc":      "author ASC",
	"author|desc":     "author DESC",
	"created_at|asc":  "created_at ASC",
	"created_at|desc": "created_at DESC",
	"updated_at|asc":  "updated_at ASC",
	"updated_at|desc": "updated_at DESC",
	"owner_id|asc":    "owner_id ASC",
	"owner_id|desc":   "owner_id DESC",
}

// SQLCBookStore implements BookPersistence using sqlc-generated queries over pgx.
type SQLCBookStore struct {
	pool *pgxpool.Pool
	q    *dbsqlc.Queries
}

// NewSQLCBookStore returns a BookPersistence backed by pool.
func NewSQLCBookStore(pool *pgxpool.Pool) *SQLCBookStore {
	return &SQLCBookStore{pool: pool, q: dbsqlc.New(pool)}
}

func escapeLikePattern(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}

func bookFromDB(b dbsqlc.Book) models.Book {
	return models.Book{
		ID:        uint(b.ID),
		OwnerID:   uint(b.OwnerID),
		Title:     b.Title,
		Author:    b.Author,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}
}

// List builds a parameterized query for the optional filters and the allowlisted
// sort/order, then paginates. Dynamic sort/filter cannot be expressed as a single
// static sqlc query, so this one method is hand-written pgx.
func (s *SQLCBookStore) List(q BookListQuery) ([]models.Book, error) {
	ctx := context.Background()

	var sb strings.Builder
	sb.WriteString("SELECT id, owner_id, title, author, created_at, updated_at FROM books")

	var (
		conds []string
		args  []any
	)
	if q.TitleLike != "" {
		args = append(args, "%"+strings.ToLower(escapeLikePattern(q.TitleLike))+"%")
		conds = append(conds, fmt.Sprintf(`LOWER(title) LIKE $%d ESCAPE '\'`, len(args)))
	}
	if q.AuthorLike != "" {
		args = append(args, "%"+strings.ToLower(escapeLikePattern(q.AuthorLike))+"%")
		conds = append(conds, fmt.Sprintf(`LOWER(author) LIKE $%d ESCAPE '\'`, len(args)))
	}
	if q.OwnerID != nil {
		args = append(args, int64(*q.OwnerID))
		conds = append(conds, fmt.Sprintf("owner_id = $%d", len(args)))
	}
	if len(conds) > 0 {
		sb.WriteString(" WHERE ")
		sb.WriteString(strings.Join(conds, " AND "))
	}

	sort := strings.ToLower(strings.TrimSpace(q.Sort))
	if _, ok := BookListSortFields[sort]; !ok {
		sort = "id"
	}
	order := strings.ToLower(strings.TrimSpace(q.Order))
	if order != "desc" {
		order = "asc"
	}
	clause := bookListOrderBy[sort+"|"+order]
	if clause == "" {
		clause = "id ASC"
	}
	sb.WriteString(" ORDER BY ")
	sb.WriteString(clause)
	// Stable pagination when the primary column has ties.
	if sort != "id" {
		sb.WriteString(", id ASC")
	}

	if q.Limit >= 0 {
		args = append(args, q.Limit)
		fmt.Fprintf(&sb, " LIMIT $%d", len(args))
	}
	if q.Offset > 0 {
		args = append(args, q.Offset)
		fmt.Fprintf(&sb, " OFFSET $%d", len(args))
	}

	rows, err := s.pool.Query(ctx, sb.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []models.Book{}
	for rows.Next() {
		var b dbsqlc.Book
		if err := rows.Scan(&b.ID, &b.OwnerID, &b.Title, &b.Author, &b.CreatedAt, &b.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, bookFromDB(b))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *SQLCBookStore) Create(book *models.Book) error {
	row, err := s.q.CreateBook(context.Background(), dbsqlc.CreateBookParams{
		OwnerID: int64(book.OwnerID),
		Title:   book.Title,
		Author:  book.Author,
	})
	if err != nil {
		return err
	}
	*book = bookFromDB(row)
	return nil
}

func (s *SQLCBookStore) FirstByID(id uint) (*models.Book, error) {
	row, err := s.q.GetBook(context.Background(), int64(id))
	if err != nil {
		return nil, mapNotFound(err)
	}
	b := bookFromDB(row)
	return &b, nil
}

func (s *SQLCBookStore) UpdateFields(id uint, title, author string) (*models.Book, error) {
	row, err := s.q.UpdateBookFields(context.Background(), dbsqlc.UpdateBookFieldsParams{
		ID:     int64(id),
		Title:  title,
		Author: author,
	})
	if err != nil {
		return nil, mapNotFound(err)
	}
	b := bookFromDB(row)
	return &b, nil
}

// PatchFields updates only non-nil pointer fields (partial update).
func (s *SQLCBookStore) PatchFields(id uint, title, author *string) (*models.Book, error) {
	if title == nil && author == nil {
		return s.FirstByID(id)
	}
	row, err := s.q.PatchBookFields(context.Background(), dbsqlc.PatchBookFieldsParams{
		ID:     int64(id),
		Title:  title,
		Author: author,
	})
	if err != nil {
		return nil, mapNotFound(err)
	}
	b := bookFromDB(row)
	return &b, nil
}

func (s *SQLCBookStore) DeleteByID(id uint) error {
	n, err := s.q.DeleteBook(context.Background(), int64(id))
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ensure interface compliance
var _ BookPersistence = (*SQLCBookStore)(nil)
