package api

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/database"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"
	"golang-rest-api-template/pkg/service"

	"github.com/gin-gonic/gin"
)

const (
	findBooksMinLimit = 1
	findBooksMaxLimit = 100
)

// BookRepository is the HTTP surface for book routes (Gin handlers).
type BookRepository interface {
	Healthcheck(c *gin.Context)
	FindBooks(c *gin.Context)
	CreateBook(c *gin.Context)
	FindBook(c *gin.Context)
	UpdateBook(c *gin.Context)
	DeleteBook(c *gin.Context)
}

type bookRepository struct {
	svc *service.BookService
}

// NewBookRepository wires persistence and cache into book HTTP handlers.
func NewBookRepository(db database.Database, redisClient cache.Cache) *bookRepository {
	store := repository.NewGormBookStore(db)
	return &bookRepository{svc: service.NewBookService(store, redisClient)}
}

func parseIDParam(c *gin.Context) (uint, bool) {
	if c == nil {
		return 0, false
	}
	value := c.Param("id")
	id, err := strconv.ParseUint(value, 10, strconv.IntSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid id format"})
		return 0, false
	}
	return uint(id), true
}

func parseOffsetLimit(c *gin.Context) (offset, limit int, ok bool) {
	offsetQuery := strings.TrimSpace(c.DefaultQuery("offset", "0"))
	limitQuery := strings.TrimSpace(c.DefaultQuery("limit", "10"))

	o, err := strconv.Atoi(offsetQuery)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid offset format"})
		return 0, 0, false
	}
	l, err := strconv.Atoi(limitQuery)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit format"})
		return 0, 0, false
	}
	if o < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be >= 0"})
		return 0, 0, false
	}
	if l < findBooksMinLimit {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be at least 1"})
		return 0, 0, false
	}
	if l > findBooksMaxLimit {
		l = findBooksMaxLimit
	}
	return o, l, true
}

// @BasePath /api/v1

// Healthcheck godoc
// @Summary ping example
// @Schemes
// @Description do ping
// @Tags example
// @Accept json
// @Produce json
// @Success 200 {string} ok
// @Router / [get]
func (r *bookRepository) Healthcheck(c *gin.Context) {
	c.JSON(http.StatusOK, "ok")
}

// FindBooks godoc
// @Summary Get all books with pagination
// @Description Get a list of all books with optional pagination. List entries are keyed by a monotonic cache generation (no Redis KEYS) and canonical integer offset/limit after parsing (leading zeros and surrounding whitespace in query params do not fragment the cache). Concurrent cache misses for the same offset/limit and generation are coalesced (singleflight) so only one database read and Redis write runs per cache key.
// @Tags books
// @Security ApiKeyAuth
// @Produce json
// @Param offset query int false "Offset for pagination (must be >= 0)" default(0)
// @Param limit query int false "Limit for pagination (minimum 1, capped at 100)" default(10)
// @Success 200 {array} models.Book "Successfully retrieved list of books"
// @Failure 400 {string} string "Bad Request"
// @Failure 500 {string} string "Internal Server Error"
// @Router /books [get]
func (r *bookRepository) FindBooks(c *gin.Context) {
	offset, limit, ok := parseOffsetLimit(c)
	if !ok {
		return
	}
	books, err := r.svc.ListBooks(c.Request.Context(), offset, limit)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrListBooksDB):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		case errors.Is(err, service.ErrListBooksMarshal):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal data"})
		case errors.Is(err, service.ErrListBooksRedis):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set cache"})
		case errors.Is(err, service.ErrListBooksUnmarshal):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unmarshal cached data"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": books})
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
func (r *bookRepository) CreateBook(c *gin.Context) {
	var input models.CreateBook
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	book, err := r.svc.CreateBook(c.Request.Context(), input.Title, input.Author)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create book"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": book})
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
func (r *bookRepository) FindBook(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	book, err := r.svc.GetBook(c.Request.Context(), id)
	if err != nil {
		if repository.IsBookNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load book"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": book})
}

// UpdateBook godoc
// @Summary Update a book by ID
// @Description Update the book details for the given ID
// @Tags books
// @Security ApiKeyAuth
// @Security JwtAuth
// @Accept  json
// @Produce  json
// @Param id path string true "Book ID"
// @Param input body models.UpdateBook true "Update book object"
// @Success 200 {object} models.Book "Successfully updated book"
// @Failure 400 {string} string "Bad Request"
// @Failure 401 {string} string "Unauthorized"
// @Failure 404 {string} string "book not found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /books/{id} [put]
func (r *bookRepository) UpdateBook(c *gin.Context) {
	var input models.UpdateBook
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	book, err := r.svc.UpdateBook(c.Request.Context(), id, input.Title, input.Author)
	if err != nil {
		if repository.IsBookNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update book"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": book})
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
// @Failure 404 {string} string "book not found"
// @Failure 500 {string} string "Internal Server Error"
// @Router /books/{id} [delete]
func (r *bookRepository) DeleteBook(c *gin.Context) {
	id, ok := parseIDParam(c)
	if !ok {
		return
	}
	if err := r.svc.DeleteBook(c.Request.Context(), id); err != nil {
		if repository.IsBookNotFound(err) {
			c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete book"})
		return
	}
	c.Status(http.StatusNoContent)
}
