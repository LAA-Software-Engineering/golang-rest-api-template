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
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/sync/singleflight"
)

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
// @Description Get a list of all books with optional pagination. Concurrent cache misses for the same offset/limit are coalesced (singleflight) so only one database read and Redis write runs per cache key.
// @Tags books
// @Security ApiKeyAuth
// @Produce json
// @Param offset query int false "Offset for pagination" default(0)
// @Param limit query int false "Limit for pagination" default(10)
// @Success 200 {array} models.Book "Successfully retrieved list of books"
// @Failure 500 {string} string "Internal Server Error"
// @Router /books [get]
func (r *bookRepository) FindBooks(c *gin.Context) {
	var books []models.Book

	// Get query params
	offsetQuery := c.DefaultQuery("offset", "0")
	limitQuery := c.DefaultQuery("limit", "10")

	// Convert query params to integers
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

	// Normalized cache key so equivalent pagination (e.g. "0" vs "00") shares one entry and singleflight.
	cacheKey := fmt.Sprintf("books_offset_%d_limit_%d", offset, limit)

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
	appCtxInterface, exists := c.Get("appCtx")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	appCtx, ok := appCtxInterface.(*bookRepository)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}
	var input models.CreateBook

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	book := models.Book{Title: input.Title, Author: input.Author}

	if err := appCtx.DB.Create(&book).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create book"})
		return
	}

	// Invalidate cache
	keysPattern := "books_offset_*"
	keys, err := appCtx.RedisClient.Keys(*appCtx.Ctx, keysPattern).Result()
	if err == nil {
		for _, key := range keys {
			appCtx.RedisClient.Del(*appCtx.Ctx, key)
		}
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

	c.Status(http.StatusNoContent)
}
