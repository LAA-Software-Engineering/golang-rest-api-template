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
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/go-redis/redis/v8"

	"github.com/gin-gonic/gin"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
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

func TestCreateBookDatabaseError(t *testing.T) {
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
	requestBody, err := json.Marshal(inputBook)
	if err != nil {
		t.Fatal(err)
	}

	dbErr := errors.New("db create failed")
	mockDB.EXPECT().Create(gomock.Any()).Return(&gorm.DB{Error: dbErr})

	w := httptest.NewRecorder()
	req, err := http.NewRequest("POST", "/books", bytes.NewBuffer(requestBody))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to create book")
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

func TestCreateBookCacheIncrError(t *testing.T) {
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
	mockCache.EXPECT().Incr(ctx, booksListCacheGenKey).Return(redis.NewIntResult(0, errors.New("incr error")))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/books", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	// Should still succeed even if cache generation bump fails
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

func TestFindBooksDatabaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sqlDB, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}

	raw, err := sqlDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	gdb := &database.GormDatabase{DB: sqlDB}
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(gdb, mockCache, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", repo.FindBooks)

	gomock.InOrder(
		mockCache.EXPECT().Get(ctx, booksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockCache.EXPECT().Get(ctx, booksListDataCacheKey(0, 0, 10)).Return(redis.NewStringResult("", redis.Nil)),
	)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/books?offset=0&limit=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to list books")
}

func TestUpdateBookDatabaseErrorOnUpdates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}

	existingBook := models.Book{Title: "Old Title", Author: "Old Author"}
	if err := db.Create(&existingBook).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Exec(`
		CREATE TRIGGER tr_books_abort_update
		BEFORE UPDATE ON books
		BEGIN
			SELECT RAISE(ABORT, 'forced update failure');
		END
	`).Error; err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	repo := NewBookRepository(&database.GormDatabase{DB: db}, nil, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", repo.UpdateBook)

	updateInput := models.UpdateBook{Title: "New Title", Author: "New Author"}
	requestBody, err := json.Marshal(updateInput)
	if err != nil {
		t.Fatal(err)
	}

	w := httptest.NewRecorder()
	req := httptest.NewRequest("PUT", "/book/1", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to update book")
}

func TestUpdateBookBumpsListCacheGen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "update_bump.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}
	b := models.Book{Title: "t", Author: "a"}
	if err := db.Create(&b).Error; err != nil {
		t.Fatal(err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()
	mockCache.EXPECT().Incr(ctx, booksListCacheGenKey).Return(redis.NewIntResult(1, nil)).Times(1)

	repo := NewBookRepository(&database.GormDatabase{DB: db}, mockCache, &ctx)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", repo.UpdateBook)

	body, err := json.Marshal(models.UpdateBook{Title: "n", Author: "n"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/book/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
}

func TestDeleteBookBumpsListCacheGen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "delete_bump.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}
	b := models.Book{Title: "del", Author: "me"}
	if err := db.Create(&b).Error; err != nil {
		t.Fatal(err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()
	mockCache.EXPECT().Incr(ctx, booksListCacheGenKey).Return(redis.NewIntResult(1, nil)).Times(1)

	repo := NewBookRepository(&database.GormDatabase{DB: db}, mockCache, &ctx)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", repo.DeleteBook)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/book/1", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
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
	gomock.InOrder(
		mockCache.EXPECT().Get(ctx, booksListCacheGenKey).Return(redis.NewStringResult("0", nil)),
		mockCache.EXPECT().Get(ctx, booksListDataCacheKey(0, 0, 10)).Return(redis.NewStringResult(string(cachedData), nil)),
	)

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

	mockCache.EXPECT().Incr(ctx, booksListCacheGenKey).Return(redis.NewIntResult(1, nil))

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

	mockDB.EXPECT().Error().Return(nil).Times(1)

	// Perform the DELETE request
	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/book/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
}

func TestDeleteBookDatabaseErrorOnDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDB := database.NewMockDatabase(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(mockDB, nil, &ctx)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", repo.DeleteBook)

	existingBook := models.Book{
		ID:     1,
		Title:  "Test Book",
		Author: "Test Author",
	}

	mockDB.EXPECT().
		FirstByID(gomock.Any(), uint(1)).
		DoAndReturn(func(dest interface{}, id uint) database.Database {
			if b, ok := dest.(*models.Book); ok {
				*b = existingBook
			}
			return mockDB
		}).Times(1)
	mockDB.EXPECT().Error().Return(nil).Times(1)

	delErr := errors.New("delete failed")
	mockDB.EXPECT().
		Delete(&existingBook).
		Return(&gorm.DB{Error: delErr}).Times(1)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/book/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to delete book")
}

func TestFindBooksSingleflightCoalescesDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "singleflight.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Book{Title: "Coalesced", Author: "Author"}).Error; err != nil {
		t.Fatal(err)
	}

	const cbName = "pkg/api:test_find_books_sf_counter"
	var selectN atomic.Int32
	if err := db.Callback().Query().After("gorm:query").Register(cbName, func(*gorm.DB) {
		selectN.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(cbName) })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(&database.GormDatabase{DB: db}, mockCache, &ctx)

	const n = 50
	dataKey := booksListDataCacheKey(0, 0, 10)
	var cacheMu sync.Mutex
	var cachedPayload string
	mockCache.EXPECT().Get(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, key string) *redis.StringCmd {
		switch key {
		case booksListCacheGenKey:
			return redis.NewStringResult("", redis.Nil)
		case dataKey:
			cacheMu.Lock()
			s := cachedPayload
			cacheMu.Unlock()
			if s == "" {
				return redis.NewStringResult("", redis.Nil)
			}
			return redis.NewStringResult(s, nil)
		default:
			return redis.NewStringResult("", errors.New("unexpected cache Get key"))
		}
	}).MinTimes(2 * n)
	mockCache.EXPECT().Set(ctx, dataKey, gomock.Any(), time.Minute).DoAndReturn(func(_ context.Context, _ string, v interface{}, _ time.Duration) *redis.StatusCmd {
		var b []byte
		switch x := v.(type) {
		case []byte:
			b = x
		case string:
			b = []byte(x)
		default:
			var err error
			b, err = json.Marshal(x)
			if err != nil {
				return redis.NewStatusResult("", err)
			}
		}
		cacheMu.Lock()
		cachedPayload = string(b)
		cacheMu.Unlock()
		return redis.NewStatusResult("OK", nil)
	}).Times(1)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", repo.FindBooks)

	var wg sync.WaitGroup
	wg.Add(n)
	statusCh := make(chan int, n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			w := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/books?offset=0&limit=10", nil)
			r.ServeHTTP(w, req)
			statusCh <- w.Code
		}()
	}
	wg.Wait()
	close(statusCh)
	for code := range statusCh {
		assert.Equal(t, http.StatusOK, code)
	}

	if got := selectN.Load(); got != 1 {
		t.Fatalf("expected exactly 1 coalesced DB query, got %d", got)
	}
}

func TestFindBooksLeadingZerosShareListCache(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "leadzero.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Book{Title: "One", Author: "A"}).Error; err != nil {
		t.Fatal(err)
	}

	const cbName = "pkg/api:test_leadzero_list_queries"
	var queryN atomic.Int32
	if err := db.Callback().Query().After("gorm:query").Register(cbName, func(*gorm.DB) {
		queryN.Add(1)
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(cbName) })

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockCache := cache.NewMockCache(ctrl)
	ctx := context.Background()
	repo := NewBookRepository(&database.GormDatabase{DB: db}, mockCache, &ctx)

	dataKey := booksListDataCacheKey(0, 0, 10)
	var cacheMu sync.Mutex
	var cachedPayload string
	mockCache.EXPECT().Get(ctx, gomock.Any()).DoAndReturn(func(_ context.Context, key string) *redis.StringCmd {
		switch key {
		case booksListCacheGenKey:
			return redis.NewStringResult("", redis.Nil)
		case dataKey:
			cacheMu.Lock()
			s := cachedPayload
			cacheMu.Unlock()
			if s == "" {
				return redis.NewStringResult("", redis.Nil)
			}
			return redis.NewStringResult(s, nil)
		default:
			return redis.NewStringResult("", errors.New("unexpected cache Get key"))
		}
	}).MinTimes(4)
	mockCache.EXPECT().Set(ctx, dataKey, gomock.Any(), time.Minute).DoAndReturn(func(_ context.Context, _ string, v interface{}, _ time.Duration) *redis.StatusCmd {
		var b []byte
		switch x := v.(type) {
		case []byte:
			b = x
		case string:
			b = []byte(x)
		default:
			var merr error
			b, merr = json.Marshal(x)
			if merr != nil {
				return redis.NewStatusResult("", merr)
			}
		}
		cacheMu.Lock()
		cachedPayload = string(b)
		cacheMu.Unlock()
		return redis.NewStatusResult("OK", nil)
	}).Times(1)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", repo.FindBooks)

	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/books?offset=00&limit=010", nil))
	assert.Equal(t, http.StatusOK, w1.Code)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/books?offset=0&limit=10", nil))
	assert.Equal(t, http.StatusOK, w2.Code)

	if got := queryN.Load(); got != 1 {
		t.Fatalf("expected one DB list query (second HTTP call hits same cache key), got %d", got)
	}
}
