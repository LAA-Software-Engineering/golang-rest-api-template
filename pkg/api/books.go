package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/database"
	"golang-rest-api-template/pkg/models"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

// booksListCacheGenKey is incremented on book writes so list pagination entries
// (scoped by generation) go stale without Redis KEYS.
const booksListCacheGenKey = "v1:books:list_cache_gen"

const (
	findBooksMinLimit = 1
	findBooksMaxLimit = 100
)

func booksListDataCacheKey(gen int64, offset, limit int) string {
	return fmt.Sprintf("books_g%d_offset_%d_limit_%d", gen, offset, limit)
}

func (r *bookRepository) booksListCacheGeneration() int64 {
	if r == nil || r.RedisClient == nil || r.Ctx == nil {
		return 0
	}
	n, err := r.RedisClient.Get(*r.Ctx, booksListCacheGenKey).Int64()
	if err != nil {
		return 0
	}
	if n < 0 {
		return 0
	}
	return n
}

func (r *bookRepository) bumpBooksListCacheGeneration() {
	if r == nil || r.RedisClient == nil || r.Ctx == nil {
		return
	}
	_, _ = r.RedisClient.Incr(*r.Ctx, booksListCacheGenKey).Result()
}

type BookRepository interface {
	Healthcheck(c *gin.Context)
	FindBooks(c *gin.Context)
	CreateBook(c *gin.Context)
	FindBook(c *gin.Context)
	UpdateBook(c *gin.Context)
	DeleteBook(c *gin.Context)
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

// Sentinel errors for FindBooks singleflight so callers can map failures to HTTP responses.
var (
	errListBooksSFDB      = errors.New("listBooks singleflight: database")
	errListBooksSFMarshal = errors.New("listBooks singleflight: marshal")
	errListBooksSFRedis   = errors.New("listBooks singleflight: redis set")
)

// bookRepository holds shared resources like database and Redis client
type bookRepository struct {
	DB          database.Database
	RedisClient cache.Cache
	Ctx         *context.Context
	listBooksSF singleflight.Group
}

// NewAppContext creates a new AppContext
func NewBookRepository(db database.Database, redisClient cache.Cache, ctx *context.Context) *bookRepository {
	return &bookRepository{
		DB:          db,
		RedisClient: redisClient,
		Ctx:         ctx,
	}
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
	var books []models.Book

	// Query params trimmed so cache keys match parsed integers (no cache fragmentation from " 0 " vs "0").
	offsetQuery := strings.TrimSpace(c.DefaultQuery("offset", "0"))
	limitQuery := strings.TrimSpace(c.DefaultQuery("limit", "10"))

	offset, err := strconv.Atoi(offsetQuery)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid offset format"})
		return
	}

	limit, err := strconv.Atoi(limitQuery)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid limit format"})
		return
	}

	if offset < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be >= 0"})
		return
	}
	if limit < findBooksMinLimit {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be at least 1"})
		return
	}
	if limit > findBooksMaxLimit {
		limit = findBooksMaxLimit
	}

	gen := r.booksListCacheGeneration()
	cacheKey := booksListDataCacheKey(gen, offset, limit)

	// Try fetching the data from Redis first
	cachedBooks, err := r.RedisClient.Get(*r.Ctx, cacheKey).Result()
	if err == nil {
		err := json.Unmarshal([]byte(cachedBooks), &books)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to unmarshal cached data"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"data": books})
		return
	}

	// If cache missed, coalesce concurrent loads on the same cache key (cache stampede protection).
	out, err, _ := r.listBooksSF.Do(cacheKey, func() (interface{}, error) {
		var loaded []models.Book
		if err := r.DB.Offset(offset).Limit(limit).Find(&loaded).Error; err != nil {
			return nil, fmt.Errorf("%w: %v", errListBooksSFDB, err)
		}
		serializedBooks, err := json.Marshal(loaded)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", errListBooksSFMarshal, err)
		}
		if err := r.RedisClient.Set(*r.Ctx, cacheKey, serializedBooks, time.Minute).Err(); err != nil {
			return nil, fmt.Errorf("%w: %v", errListBooksSFRedis, err)
		}
		return loaded, nil
	})
	if err != nil {
		switch {
		case errors.Is(err, errListBooksSFDB):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		case errors.Is(err, errListBooksSFMarshal):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to marshal data"})
		case errors.Is(err, errListBooksSFRedis):
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set cache"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list books"})
		}
		return
	}

	books = out.([]models.Book)
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

	book := models.Book{Title: input.Title, Author: input.Author}

	if err := r.DB.Create(&book).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create book"})
		return
	}

	r.bumpBooksListCacheGeneration()

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
	var book models.Book

	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	if err := r.DB.FirstByID(&book, id).Error(); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
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
	var book models.Book
	var input models.UpdateBook

	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	if err := r.DB.FirstByID(&book, id).Error(); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := r.DB.Model(&book).Updates(models.Book{Title: input.Title, Author: input.Author}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update book"})
		return
	}

	r.bumpBooksListCacheGeneration()

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
	var book models.Book

	id, ok := parseIDParam(c)
	if !ok {
		return
	}

	if err := r.DB.FirstByID(&book, id).Error(); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "book not found"})
		return
	}

	if err := r.DB.Delete(&book).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete book"})
		return
	}

	r.bumpBooksListCacheGeneration()

	c.Status(http.StatusNoContent)
}
