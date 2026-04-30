package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/database"
	"golang-rest-api-template/pkg/models"
	"net/http"
	"net/http/httptest"
	"testing"

	"gorm.io/gorm"

	"github.com/go-redis/redis/v8"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestNewBookRepository(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	mockCtx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &mockCtx)

	assert.NotNil(t, repo, "NewBookRepository should return a non-nil instance of bookRepository")
	assert.Equal(t, mockDB, repo.DB, "DB should be set to the mock database instance")
	assert.Equal(t, mockCache, repo.RedisClient, "RedisClient should be set to the mock cache instance")
}

func TestHealthcheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &ctx)

	// Set up Gin
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	// Call the actual Healthcheck method
	repo.Healthcheck(c)

	// Check the response
	assert.Equal(t, http.StatusOK, recorder.Code)
	assert.Equal(t, "\"ok\"", recorder.Body.String())
}

func TestParseIDParamNilContext(t *testing.T) {
	id, ok := parseIDParam(nil)
	assert.Equal(t, uint(0), id)
	assert.False(t, ok)
}

func TestFindBooksInvalidOffset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", repo.FindBooks)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/books?offset=abc&limit=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid offset format")
}

func TestFindBooksInvalidLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", repo.FindBooks)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/books?offset=0&limit=xyz", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid limit format")
}

func TestCreateBookBindError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", func(c *gin.Context) {
		c.Set("appCtx", repo)
		repo.CreateBook(c)
	})

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/books", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestCreateBookMissingAppCtx(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", repo.CreateBook) // Not setting appCtx

	inputBook := models.CreateBook{Title: "New Book", Author: "New Author"}
	requestBody, _ := json.Marshal(inputBook)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/books", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
}

func TestCreateBookCacheKeysError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", func(c *gin.Context) {
		c.Set("appCtx", repo)
		repo.CreateBook(c)
	})

	inputBook := models.CreateBook{Title: "New Book", Author: "New Author"}
	requestBody, _ := json.Marshal(inputBook)

	mockDB.EXPECT().Create(gomock.Any()).Return(&gorm.DB{Error: nil})

	// Mock cache keys error
	mockCache.EXPECT().Keys(ctx, "books_offset_*").Return(redis.NewStringSliceResult(nil, errors.New("keys error")))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/books", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Should still succeed even if cache invalidation fails
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "New Book")
}

func TestCreateBookCacheDelError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", func(c *gin.Context) {
		c.Set("appCtx", repo)
		repo.CreateBook(c)
	})

	inputBook := models.CreateBook{Title: "New Book", Author: "New Author"}
	requestBody, _ := json.Marshal(inputBook)

	mockDB.EXPECT().Create(gomock.Any()).Return(&gorm.DB{Error: nil})

	// Mock cache keys success but del error
	mockCache.EXPECT().Keys(ctx, "books_offset_*").Return(redis.NewStringSliceResult([]string{"key1"}, nil))
	mockCache.EXPECT().Del(ctx, "key1").Return(redis.NewIntResult(0, errors.New("del error")))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/books", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Should still succeed
	assert.Equal(t, http.StatusCreated, w.Code)
	assert.Contains(t, w.Body.String(), "New Book")
}

func TestUpdateBookInvalidID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", repo.UpdateBook)

	updateInput := models.UpdateBook{Title: "New Title", Author: "New Author"}
	requestBody, _ := json.Marshal(updateInput)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/book/abc", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid id format")
}

func TestUpdateBookNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", repo.UpdateBook)

	updateInput := models.UpdateBook{Title: "New Title", Author: "New Author"}
	requestBody, _ := json.Marshal(updateInput)

	mockDB.EXPECT().FirstByID(gomock.Any(), uint(1)).DoAndReturn(func(dest interface{}, id uint) database.Database {
		return mockDB
	})
	mockDB.EXPECT().Error().Return(gorm.ErrRecordNotFound)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/book/1", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "book not found")
}

func TestUpdateBookBindError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", repo.UpdateBook)

	existingBook := models.Book{ID: 1, Title: "Old Title", Author: "Old Author"}

	mockDB.EXPECT().FirstByID(gomock.Any(), uint(1)).DoAndReturn(func(dest interface{}, id uint) database.Database {
		*dest.(*models.Book) = existingBook
		return mockDB
	})
	mockDB.EXPECT().Error().Return(nil)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/book/1", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestDeleteBookInvalidID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", repo.DeleteBook)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/book/xyz", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid id format")
}

func TestDeleteBookNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", repo.DeleteBook)

	mockDB.EXPECT().FirstByID(gomock.Any(), uint(1)).DoAndReturn(func(dest interface{}, id uint) database.Database {
		return mockDB
	})
	mockDB.EXPECT().Error().Return(gorm.ErrRecordNotFound)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/book/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "book not found")
}

func TestFindBooks(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	mockGormDB := database.NewMockDatabase(ctrl) // Correct type for GORM DB operations
	ctx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &ctx)

	// Set up Gin
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", repo.FindBooks)

	// Set up common mock expectations
	mockGormDB.EXPECT().Find(gomock.Any()).DoAndReturn(func(books *[]models.Book) *gorm.DB {
		*books = append(*books, models.Book{Title: "New Book", Author: "New Author"})
		return &gorm.DB{Error: nil} // Assume this is the struct provided by the actual Gorm package
	}).AnyTimes()

	books := []models.Book{{Title: "Book One", Author: "Author One"}}
	cachedData, _ := json.Marshal(books)
	mockCache.EXPECT().Get(ctx, "books_offset_0_limit_10").Return(redis.NewStringResult(string(cachedData), nil))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/books?offset=0&limit=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Book One")
}

func TestCreateBook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()

	repo := NewBookRepository(mockDB, mockCache, &ctx)

	// Set up Gin
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", func(c *gin.Context) {
		// Set the appCtx in the Gin context
		c.Set("appCtx", repo)
		repo.CreateBook(c)
	})

	// Example data for the test
	inputBook := models.CreateBook{Title: "New Book", Author: "New Author"}
	requestBody, err := json.Marshal(inputBook)
	if err != nil {
		t.Fatalf("Failed to marshal input book data: %v", err)
	}

	// Set up database mock to simulate successful book creation
	mockDB.EXPECT().Create(gomock.Any()).DoAndReturn(func(book *models.Book) *gorm.DB {
		// Normally, you might simulate setting an ID or other fields modified by the DB
		return &gorm.DB{Error: nil}
	})

	// Set up cache mock to simulate key retrieval and deletion
	keyPattern := "books_offset_*"
	mockCache.EXPECT().Keys(ctx, keyPattern).Return(redis.NewStringSliceResult([]string{"books_offset_0_limit_10"}, nil))
	mockCache.EXPECT().Del(ctx, "books_offset_0_limit_10").Return(redis.NewIntResult(1, nil))

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/books", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatalf("Failed to create the HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Serve the HTTP request
	r.ServeHTTP(w, req)

	// Assertions to check the response
	assert.Equal(t, http.StatusCreated, w.Code, "Expected HTTP status code 201")
	assert.Contains(t, w.Body.String(), "New Book", "Response body should contain the book title")
}

func TestFindBook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	// Set up Gin
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/book/:id", repo.FindBook)

	// Prepare test data
	expectedBook := models.Book{
		ID:     1,
		Title:  "Effective Go",
		Author: "Robert Griesemer",
	}

	// Mock expectations

	// Mock the FirstByID method
	mockDB.EXPECT().
		FirstByID(gomock.Any(), uint(1)).
		DoAndReturn(func(dest interface{}, id uint) database.Database {
			if b, ok := dest.(*models.Book); ok {
				*b = expectedBook
			}
			return mockDB
		}).Times(1)

	// Mock the Error method or field access
	mockDB.EXPECT().
		Error().
		Return(nil).
		Times(1)

	// Perform the request
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/book/1", nil)
	r.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response struct {
		Data models.Book `json:"data"`
	}

	err := json.NewDecoder(w.Body).Decode(&response)
	assert.NoError(t, err)
	assert.Equal(t, expectedBook.Author, response.Data.Author)
}

func TestFindBookNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/book/:id", repo.FindBook)

	mockDB.EXPECT().FirstByID(gomock.Any(), uint(1)).DoAndReturn(func(dest interface{}, id uint) database.Database {
		return mockDB
	})
	mockDB.EXPECT().Error().Return(gorm.ErrRecordNotFound)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/book/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "book not found")
}

func TestFindBookRejectsInvalidID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/book/:id", repo.FindBook)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/book/1%20OR%201=1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid id format")
}

func TestDeleteBook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	// Create mock for the database
	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	// Set up Gin for testing
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", repo.DeleteBook)

	// Prepare the book data
	existingBook := models.Book{
		ID:     1,
		Title:  "Test Book",
		Author: "Test Author",
	}

	// Mock the FirstByID method to load the existingBook and return mockDB
	mockDB.EXPECT().
		FirstByID(gomock.Any(), uint(1)).
		DoAndReturn(func(dest interface{}, id uint) database.Database {
			if b, ok := dest.(*models.Book); ok {
				*b = existingBook
			}
			return mockDB
		}).Times(1)

	// Mock Delete method
	mockDB.EXPECT().
		Delete(&existingBook).
		Return(&gorm.DB{Error: nil}).Times(1)

	// Mock Error method to return nil
	mockDB.EXPECT().Error().Return(nil).AnyTimes()

	// Perform the DELETE request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/book/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
}
