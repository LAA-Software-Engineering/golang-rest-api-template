package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/httperr"
	"golang-rest-api-template/pkg/httpresp"
	"golang-rest-api-template/pkg/middleware"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"
	"golang-rest-api-template/pkg/service"

	"github.com/gin-gonic/gin"
)

const (
	findBooksMinLimit = 1
	findBooksMaxLimit = 100
)

var bookListSortAllowlist = map[string]struct{}{
	"id": {}, "title": {}, "author": {}, "created_at": {}, "updated_at": {}, "owner_id": {},
}

// BookHandler defines Gin handlers for book routes (HTTP layer only; persistence
// lives in pkg/repository).
type BookHandler interface {
	Healthcheck(c *gin.Context)
	FindBooks(c *gin.Context)
	CreateBook(c *gin.Context)
	FindBook(c *gin.Context)
	UpdateBook(c *gin.Context)
	PatchBook(c *gin.Context)
	DeleteBook(c *gin.Context)
}

type bookHandler struct {
	svc *service.BookService
}

// NewBookHandler wires persistence and cache into book HTTP handlers.
func NewBookHandler(store repository.BookPersistence, redisClient cache.Cache) *bookHandler {
	return &bookHandler{svc: service.NewBookService(store, redisClient)}
}

func parseIDParam(c *gin.Context) (uint, bool) {
	if c == nil {
		return 0, false
	}
	value := c.Param("id")
	id, err := strconv.ParseUint(value, 10, strconv.IntSize)
	if err != nil {
		httperr.Write(c, http.StatusBadRequest, "Invalid id format")
		return 0, false
	}
	return uint(id), true
}

func parseOffsetLimit(c *gin.Context) (offset, limit int, ok bool) {
	offsetQuery := strings.TrimSpace(c.DefaultQuery("offset", "0"))
	limitQuery := strings.TrimSpace(c.DefaultQuery("limit", "10"))

	o, err := strconv.Atoi(offsetQuery)
	if err != nil {
		httperr.Write(c, http.StatusBadRequest, "Invalid offset format")
		return 0, 0, false
	}
	l, err := strconv.Atoi(limitQuery)
	if err != nil {
		httperr.Write(c, http.StatusBadRequest, "Invalid limit format")
		return 0, 0, false
	}
	if o < 0 {
		httperr.Write(c, http.StatusBadRequest, "offset must be >= 0")
		return 0, 0, false
	}
	if l < findBooksMinLimit {
		httperr.Write(c, http.StatusBadRequest, "limit must be at least 1")
		return 0, 0, false
	}
	if l > findBooksMaxLimit {
		l = findBooksMaxLimit
	}
	return o, l, true
}

// parseBookListQuery parses pagination, filter, and sort query params for GET /books.
func parseBookListQuery(c *gin.Context) (repository.BookListQuery, bool) {
	var q repository.BookListQuery
	offset, limit, ok := parseOffsetLimit(c)
	if !ok {
		return q, false
	}
	q.Offset = offset
	q.Limit = limit
	q.TitleLike = strings.TrimSpace(c.Query("title_like"))
	q.AuthorLike = strings.TrimSpace(c.Query("author_like"))

	ownerRaw := strings.TrimSpace(c.Query("owner_id"))
	if ownerRaw != "" {
		id, err := strconv.ParseUint(ownerRaw, 10, strconv.IntSize)
		if err != nil {
			httperr.Write(c, http.StatusBadRequest, "Invalid owner_id format")
			return q, false
		}
		ownerID := uint(id)
		q.OwnerID = &ownerID
	}

	sort := strings.TrimSpace(c.DefaultQuery("sort", "id"))
	if _, allowed := bookListSortAllowlist[sort]; !allowed {
		httperr.Write(c, http.StatusBadRequest, "Invalid sort field")
		return q, false
	}
	q.Sort = sort

	order := strings.ToLower(strings.TrimSpace(c.DefaultQuery("order", "asc")))
	if order != "asc" && order != "desc" {
		httperr.Write(c, http.StatusBadRequest, "Invalid order; must be asc or desc")
		return q, false
	}
	q.Order = order
	return q, true
}

// contextUserID returns the authenticated users.id set by middleware.JWTAuth.
func contextUserID(c *gin.Context) (uint, bool) {
	if c == nil {
		return 0, false
	}
	v, ok := c.Get(middleware.ContextUserID)
	if !ok {
		return 0, false
	}
	id, ok := v.(uint)
	return id, ok && id > 0
}

// @BasePath /api/v1

// Healthcheck godoc
// @Summary ping example
// @Schemes
// @Description do ping
// @Tags example
// @Accept json
// @Produce json
// @Success 200 {object} models.HealthOKBody "Health payload in standard envelope"
// @Router / [get]
func (h *bookHandler) Healthcheck(c *gin.Context) {
	httpresp.OK(c, "ok")
}

// FindBooks godoc
// @Summary Get all books with pagination, filter, and sort
// @Description Get a list of books with optional pagination (offset/limit), filters (title_like, author_like, owner_id), and sort (sort/order). List entries are keyed by a monotonic cache generation (no Redis KEYS) and a canonical query after parsing (leading zeros and surrounding whitespace in query params do not fragment the cache). Concurrent cache misses for the same query and generation are coalesced (singleflight) so only one database read and Redis write runs per cache key.
// @Tags books
// @Security ApiKeyAuth
// @Produce json
// @Param offset query int false "Offset for pagination (must be >= 0)" default(0)
// @Param limit query int false "Limit for pagination (minimum 1, capped at 100)" default(10)
// @Param title_like query string false "Case-insensitive substring filter on title"
// @Param author_like query string false "Case-insensitive substring filter on author"
// @Param owner_id query int false "Exact match filter on owner_id"
// @Param sort query string false "Sort field (id, title, author, created_at, updated_at, owner_id)" default(id) Enums(id, title, author, created_at, updated_at, owner_id)
// @Param order query string false "Sort direction" default(asc) Enums(asc, desc)
// @Success 200 {array} models.Book "Successfully retrieved list of books"
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /books [get]
func (h *bookHandler) FindBooks(c *gin.Context) {
	q, ok := parseBookListQuery(c)
	if !ok {
		return
	}
	books, err := h.svc.ListBooks(c.Request.Context(), q)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrListBooksDB):
			httperr.Write(c, http.StatusInternalServerError, "Failed to list books")
		case errors.Is(err, service.ErrListBooksMarshal):
			httperr.Write(c, http.StatusInternalServerError, "Failed to marshal data")
		case errors.Is(err, service.ErrListBooksRedis):
			httperr.Write(c, http.StatusInternalServerError, "Failed to set cache")
		case errors.Is(err, service.ErrListBooksUnmarshal):
			httperr.Write(c, http.StatusInternalServerError, "Failed to unmarshal cached data")
		default:
			httperr.Write(c, http.StatusInternalServerError, "Failed to list books")
		}
		return
	}
	httpresp.OK(c, books)
}

// CreateBook godoc
// @Summary Create a new book
// @Description Create a new book with the given input data
// @Tags books
// @Security ApiKeyAuth
// @Security JwtAuth
// @Accept  json
// @Produce  json
// @Param   input     body   models.CreateBook   true   "Create book object"
// @Success 201 {object} models.Book "Successfully created book"
// @Failure 400 {string} string "Bad Request"
// @Failure 401 {string} string "Unauthorized"
// @Failure 500 {string} string "Internal Server Error"
// @Router /books [post]
func (h *bookHandler) CreateBook(c *gin.Context) {
	ownerID, ok := contextUserID(c)
	if !ok {
		httperr.Write(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var input models.CreateBook
	if err := c.ShouldBindJSON(&input); err != nil {
		httperr.Write(c, http.StatusBadRequest, err.Error())
		return
	}
	book, err := h.svc.CreateBook(c.Request.Context(), ownerID, input.Title, input.Author)
	if err != nil {
		httperr.Write(c, http.StatusInternalServerError, "Failed to create book")
		return
	}
	httpresp.Created(c, book)
}

// FindBook godoc
// @Summary Find a book by ID
// @Description Get details of a book by its ID
// @Tags books
// @Security ApiKeyAuth
// @Produce json
// @Param id path string true "Book ID"
// @Success 200 {object} models.Book "Successfully retrieved book"
// @Failure 404 {string} string "Book not found"
// @Router /books/{id} [get]
func (h *bookHandler) FindBook(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	book, err := h.svc.GetBook(c.Request.Context(), id)
	if err != nil {
		if repository.IsBookNotFound(err) {
			httperr.Write(c, http.StatusNotFound, "book not found")
			return
		}
		httperr.Write(c, http.StatusInternalServerError, "Failed to load book")
		return
	}
	httpresp.OK(c, book)
}

// UpdateBook godoc
// @Summary Replace a book by ID (PUT)
// @Description Replaces both title and author. Use PATCH for partial updates.
// @Tags books
// @Security ApiKeyAuth
// @Security JwtAuth
// @Accept  json
// @Produce  json
// @Param id path string true "Book ID"
// @Param input body models.ReplaceBook true "Full book fields"
// @Success 200 {object} models.Book "Successfully updated book"
// @Failure 400 {string} string "Bad Request"
// @Failure 401 {string} string "Unauthorized"
// @Failure 403 {string} string "Forbidden"
// @Failure 404 {string} string "book not found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /books/{id} [put]
func (h *bookHandler) UpdateBook(c *gin.Context) {
	actorID, ok := contextUserID(c)
	if !ok {
		httperr.Write(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	var input models.ReplaceBook
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		httperr.Write(c, http.StatusBadRequest, err.Error())
		return
	}
	book, err := h.svc.ReplaceBook(c.Request.Context(), actorID, id, input.Title, input.Author)
	if err != nil {
		if repository.IsBookNotFound(err) {
			httperr.Write(c, http.StatusNotFound, "book not found")
			return
		}
		if errors.Is(err, service.ErrBookForbidden) {
			httperr.Write(c, http.StatusForbidden, "forbidden")
			return
		}
		httperr.Write(c, http.StatusInternalServerError, "Failed to update book")
		return
	}
	httpresp.OK(c, book)
}

// PatchBook godoc
// @Summary Partially update a book by ID (PATCH)
// @Description Updates only fields present in the JSON body (at least one of title or author).
// @Tags books
// @Security ApiKeyAuth
// @Security JwtAuth
// @Accept  json
// @Produce  json
// @Param id path string true "Book ID"
// @Param input body models.PatchBook true "Fields to change"
// @Success 200 {object} models.Book "Successfully updated book"
// @Failure 400 {string} string "Bad Request"
// @Failure 401 {string} string "Unauthorized"
// @Failure 403 {string} string "Forbidden"
// @Failure 404 {string} string "book not found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /books/{id} [patch]
func (h *bookHandler) PatchBook(c *gin.Context) {
	actorID, ok := contextUserID(c)
	if !ok {
		httperr.Write(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	var input models.PatchBook
	if err := c.ShouldBindJSON(&input); err != nil {
		httperr.Write(c, http.StatusBadRequest, err.Error())
		return
	}
	if input.Title == nil && input.Author == nil {
		httperr.Write(c, http.StatusBadRequest, "at least one of title or author is required")
		return
	}
	book, err := h.svc.PatchBook(c.Request.Context(), actorID, id, input.Title, input.Author)
	if err != nil {
		if repository.IsBookNotFound(err) {
			httperr.Write(c, http.StatusNotFound, "book not found")
			return
		}
		if errors.Is(err, service.ErrBookForbidden) {
			httperr.Write(c, http.StatusForbidden, "forbidden")
			return
		}
		httperr.Write(c, http.StatusInternalServerError, "Failed to update book")
		return
	}
	httpresp.OK(c, book)
}

// DeleteBook godoc
// @Summary Delete a book by ID
// @Description Delete the book with the given ID
// @Tags books
// @Security ApiKeyAuth
// @Security JwtAuth
// @Produce json
// @Param id path string true "Book ID"
// @Success 204 "Successfully deleted book"
// @Failure 401 {string} string "Unauthorized"
// @Failure 403 {string} string "Forbidden"
// @Failure 404 {string} string "book not found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /books/{id} [delete]
func (h *bookHandler) DeleteBook(c *gin.Context) {
	actorID, ok := contextUserID(c)
	if !ok {
		httperr.Write(c, http.StatusUnauthorized, "Unauthorized")
		return
	}
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := h.svc.DeleteBook(c.Request.Context(), actorID, id); err != nil {
		if repository.IsBookNotFound(err) {
			httperr.Write(c, http.StatusNotFound, "book not found")
			return
		}
		if errors.Is(err, service.ErrBookForbidden) {
			httperr.Write(c, http.StatusForbidden, "forbidden")
			return
		}
		httperr.Write(c, http.StatusInternalServerError, "Failed to delete book")
		return
	}
	c.Status(http.StatusNoContent)
}
