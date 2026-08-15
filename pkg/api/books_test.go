package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/middleware"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"
	"golang-rest-api-template/pkg/service"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/go-redis/redis/v8"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
	"gorm.io/driver/sqlite"
)

// withBookActor injects the authenticated user id as JWTAuth would (for handler tests).
func withBookActor(userID uint) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(middleware.ContextUserID, userID)
		c.Set("username", "test-user")
		c.Next()
	}
}

func defaultBookListQuery(offset, limit int) repository.BookListQuery {
	return repository.BookListQuery{Offset: offset, Limit: limit, Sort: "id", Order: "asc"}
}

func TestNewBookHandler(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)

	h := NewBookHandler(mockStore, mockCache)

	assert.NotNil(t, h, "NewBookHandler should return a non-nil *bookHandler")
}

func TestHealthcheck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)

	h := NewBookHandler(mockStore, mockCache)

	// Set up Gin
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)

	// Call the actual Healthcheck method
	h.Healthcheck(c)

	assert.Equal(t, http.StatusOK, recorder.Code)
	var health struct {
		Data string `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(recorder.Body.Bytes(), &health))
	assert.Equal(t, "ok", health.Data)
}

func TestParseIDParamNilContext(t *testing.T) {
	id, ok := parseIDParam(nil)
	assert.Equal(t, uint(0), id)
	assert.False(t, ok)
}

func TestFindBooksInvalidOffset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)

	h := NewBookHandler(mockStore, mockCache)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/books?offset=abc&limit=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid offset format")
}

func TestFindBooksInvalidLimit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)

	h := NewBookHandler(mockStore, mockCache)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/books?offset=0&limit=xyz", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid limit format")
}

func TestFindBooksNegativeOffset(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	h := NewBookHandler(mockStore, mockCache)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/books?offset=-1&limit=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "offset must be")
}

func TestFindBooksLimitBelowOne(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	h := NewBookHandler(mockStore, mockCache)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/books?offset=0&limit=0", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "limit must be at least 1")
}

func TestFindBooksLimitCappedAtMax(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "cap_limit.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 120; i++ {
		if err := db.Create(&models.Book{OwnerID: 1, Title: "b" + strconv.Itoa(i), Author: "a"}).Error; err != nil {
			t.Fatal(err)
		}
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockCache := cache.NewMockCache(ctrl)
	h := NewBookHandler(repository.NewGormBookStore(db), mockCache)

	gomock.InOrder(
		mockCache.EXPECT().Get(gomock.Any(), service.BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockCache.EXPECT().Get(gomock.Any(), service.BooksListDataCacheKey(0, defaultBookListQuery(0, findBooksMaxLimit))).Return(redis.NewStringResult("", redis.Nil)),
	)
	mockCache.EXPECT().Set(gomock.Any(), service.BooksListDataCacheKey(0, defaultBookListQuery(0, findBooksMaxLimit)), gomock.Any(), time.Minute).Return(redis.NewStatusResult("OK", nil)).Times(1)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/books?offset=0&limit=500", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data []models.Book `json:"data"`
	}
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Len(t, resp.Data, findBooksMaxLimit)
}

func TestCreateBookDatabaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)
	h := NewBookHandler(mockStore, mockCache)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", withBookActor(1), h.CreateBook)

	inputBook := models.CreateBook{Title: "New Book", Author: "New Author"}
	requestBody, err := json.Marshal(inputBook)
	if err != nil {
		t.Fatal(err)
	}

	dbErr := errors.New("db create failed")
	mockStore.EXPECT().Create(gomock.Any()).Return(dbErr)

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

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)

	h := NewBookHandler(mockStore, mockCache)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", withBookActor(1), h.CreateBook)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/books", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestCreateBookRequiresAuthContext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h := NewBookHandler(repository.NewMockBookPersistence(ctrl), cache.NewMockCache(ctrl))
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", h.CreateBook)
	body, err := json.Marshal(models.CreateBook{Title: "x", Author: "y"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/books", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestCreateBookCacheIncrError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)

	h := NewBookHandler(mockStore, mockCache)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", withBookActor(1), h.CreateBook)

	inputBook := models.CreateBook{Title: "New Book", Author: "New Author"}
	requestBody, _ := json.Marshal(inputBook)

	mockStore.EXPECT().Create(gomock.Any()).Return(nil)
	mockCache.EXPECT().Incr(gomock.Any(), service.BooksListCacheGenKey).Return(redis.NewIntResult(0, errors.New("incr error")))

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

	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", withBookActor(1), h.UpdateBook)

	updateInput := models.ReplaceBook{Title: "New Title", Author: "New Author"}
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

	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", withBookActor(1), h.UpdateBook)

	updateInput := models.ReplaceBook{Title: "New Title", Author: "New Author"}
	requestBody, _ := json.Marshal(updateInput)

	mockStore.EXPECT().FirstByID(uint(1)).Return(nil, gorm.ErrRecordNotFound)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/book/1", bytes.NewBuffer(requestBody))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "book not found")
}

func TestUpdateBookForbiddenWrongOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Book{OwnerID: 1, Title: "mine", Author: "a"}).Error; err != nil {
		t.Fatal(err)
	}
	h := NewBookHandler(repository.NewGormBookStore(db), nil)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", withBookActor(2), h.UpdateBook)
	body, err := json.Marshal(models.ReplaceBook{Title: "n", Author: "n"})
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/book/1", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "forbidden")
}

func TestUpdateBookBindError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", withBookActor(1), h.UpdateBook)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("PUT", "/book/1", bytes.NewBufferString("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "error")
}

func TestPatchBookRequiresAtLeastOneField(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h := NewBookHandler(repository.NewMockBookPersistence(ctrl), nil)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PATCH("/book/:id", withBookActor(1), h.PatchBook)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/book/1", bytes.NewBufferString("{}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "at least one")
}

func TestPutBookRequiresTitleAndAuthor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h := NewBookHandler(repository.NewMockBookPersistence(ctrl), nil)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", withBookActor(1), h.UpdateBook)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/book/1", bytes.NewBufferString(`{"title":"only-title"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestPatchBookTitleOnly(t *testing.T) {
	// Isolated DB: shared in-memory SQLite is reused across tests and races under parallel pkg/api runs.
	dbPath := filepath.Join(t.TempDir(), "patch_book_title.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Book{OwnerID: 1, Title: "old", Author: "same"}).Error; err != nil {
		t.Fatal(err)
	}
	h := NewBookHandler(repository.NewGormBookStore(db), nil)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PATCH("/book/:id", withBookActor(1), h.PatchBook)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPatch, "/book/1", bytes.NewBufferString(`{"title":"new-title"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	var resp struct {
		Data models.Book `json:"data"`
	}
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.Equal(t, "new-title", resp.Data.Title)
	assert.Equal(t, "same", resp.Data.Author)
}

func TestFindBooksDatabaseError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	dbPath := filepath.Join(t.TempDir(), "findbooks_db_err.sqlite")
	sqlDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
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

	mockCache := cache.NewMockCache(ctrl)
	h := NewBookHandler(repository.NewGormBookStore(sqlDB), mockCache)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	gomock.InOrder(
		mockCache.EXPECT().Get(gomock.Any(), service.BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockCache.EXPECT().Get(gomock.Any(), service.BooksListDataCacheKey(0, defaultBookListQuery(0, 10))).Return(redis.NewStringResult("", redis.Nil)),
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

	existingBook := models.Book{OwnerID: 1, Title: "Old Title", Author: "Old Author"}
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

	h := NewBookHandler(repository.NewGormBookStore(db), nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", withBookActor(1), h.UpdateBook)

	updateInput := models.ReplaceBook{Title: "New Title", Author: "New Author"}
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
	b := models.Book{OwnerID: 1, Title: "t", Author: "a"}
	if err := db.Create(&b).Error; err != nil {
		t.Fatal(err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockCache := cache.NewMockCache(ctrl)
	mockCache.EXPECT().Incr(gomock.Any(), service.BooksListCacheGenKey).Return(redis.NewIntResult(1, nil)).Times(1)

	h := NewBookHandler(repository.NewGormBookStore(db), mockCache)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.PUT("/book/:id", withBookActor(1), h.UpdateBook)

	body, err := json.Marshal(models.ReplaceBook{Title: "n", Author: "n"})
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
	b := models.Book{OwnerID: 1, Title: "del", Author: "me"}
	if err := db.Create(&b).Error; err != nil {
		t.Fatal(err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockCache := cache.NewMockCache(ctrl)
	mockCache.EXPECT().Incr(gomock.Any(), service.BooksListCacheGenKey).Return(redis.NewIntResult(1, nil)).Times(1)

	h := NewBookHandler(repository.NewGormBookStore(db), mockCache)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", withBookActor(1), h.DeleteBook)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodDelete, "/book/1", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestDeleteBookInvalidID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", withBookActor(1), h.DeleteBook)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/book/xyz", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid id format")
}

func TestDeleteBookNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", withBookActor(1), h.DeleteBook)

	mockStore.EXPECT().FirstByID(uint(1)).Return(nil, gorm.ErrRecordNotFound)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/book/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "book not found")
}

func TestFindBooks(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "findbooks_list.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Book{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Book{OwnerID: 1, Title: "Book One", Author: "Author One"}).Error; err != nil {
		t.Fatal(err)
	}

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockCache := cache.NewMockCache(ctrl)
	h := NewBookHandler(repository.NewGormBookStore(db), mockCache)

	gomock.InOrder(
		mockCache.EXPECT().Get(gomock.Any(), service.BooksListCacheGenKey).Return(redis.NewStringResult("0", nil)),
		mockCache.EXPECT().Get(gomock.Any(), service.BooksListDataCacheKey(0, defaultBookListQuery(0, 10))).Return(redis.NewStringResult("", redis.Nil)),
	)
	mockCache.EXPECT().Set(gomock.Any(), service.BooksListDataCacheKey(0, defaultBookListQuery(0, 10)), gomock.Any(), time.Minute).Return(redis.NewStatusResult("OK", nil)).Times(1)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/books?offset=0&limit=10", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "Book One")
}

func TestCreateBook(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	mockCache := cache.NewMockCache(ctrl)

	h := NewBookHandler(mockStore, mockCache)

	// Set up Gin
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.POST("/books", withBookActor(1), h.CreateBook)

	// Example data for the test
	inputBook := models.CreateBook{Title: "New Book", Author: "New Author"}
	requestBody, err := json.Marshal(inputBook)
	if err != nil {
		t.Fatalf("Failed to marshal input book data: %v", err)
	}

	mockStore.EXPECT().Create(gomock.Any()).Return(nil)

	mockCache.EXPECT().Incr(gomock.Any(), service.BooksListCacheGenKey).Return(redis.NewIntResult(1, nil))

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

	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	// Set up Gin
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/book/:id", h.FindBook)

	// Prepare test data
	expectedBook := models.Book{
		ID:     1,
		Title:  "Effective Go",
		Author: "Robert Griesemer",
	}

	mockStore.EXPECT().FirstByID(uint(1)).Return(&expectedBook, nil).Times(1)

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

	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/book/:id", h.FindBook)

	mockStore.EXPECT().FirstByID(uint(1)).Return(nil, gorm.ErrRecordNotFound)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/book/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "book not found")
}

func TestFindBookRejectsInvalidID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/book/:id", h.FindBook)

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
	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	// Set up Gin for testing
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", withBookActor(1), h.DeleteBook)

	mockStore.EXPECT().FirstByID(uint(1)).Return(&models.Book{ID: 1, OwnerID: 1, Title: "t", Author: "a"}, nil).Times(1)
	mockStore.EXPECT().DeleteByID(uint(1)).Return(nil).Times(1)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodDelete, "/book/1", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Empty(t, w.Body.Bytes())
}

func TestDeleteBookDatabaseErrorOnDelete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockStore := repository.NewMockBookPersistence(ctrl)
	h := NewBookHandler(mockStore, nil)

	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.DELETE("/book/:id", withBookActor(1), h.DeleteBook)

	delErr := errors.New("delete failed")
	mockStore.EXPECT().FirstByID(uint(1)).Return(&models.Book{ID: 1, OwnerID: 1}, nil).Times(1)
	mockStore.EXPECT().DeleteByID(uint(1)).Return(delErr).Times(1)

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
	if err := db.Create(&models.Book{OwnerID: 1, Title: "Coalesced", Author: "Author"}).Error; err != nil {
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
	h := NewBookHandler(repository.NewGormBookStore(db), mockCache)

	const n = 50
	dataKey := service.BooksListDataCacheKey(0, defaultBookListQuery(0, 10))
	var cacheMu sync.Mutex
	var cachedPayload string
	mockCache.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, key string) *redis.StringCmd {
		switch key {
		case service.BooksListCacheGenKey:
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
	mockCache.EXPECT().Set(gomock.Any(), dataKey, gomock.Any(), time.Minute).DoAndReturn(func(_ context.Context, _ string, v interface{}, _ time.Duration) *redis.StatusCmd {
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
	r.GET("/books", h.FindBooks)

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
	if err := db.Create(&models.Book{OwnerID: 1, Title: "One", Author: "A"}).Error; err != nil {
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
	h := NewBookHandler(repository.NewGormBookStore(db), mockCache)

	dataKey := service.BooksListDataCacheKey(0, defaultBookListQuery(0, 10))
	var cacheMu sync.Mutex
	var cachedPayload string
	mockCache.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, key string) *redis.StringCmd {
		switch key {
		case service.BooksListCacheGenKey:
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
	mockCache.EXPECT().Set(gomock.Any(), dataKey, gomock.Any(), time.Minute).DoAndReturn(func(_ context.Context, _ string, v interface{}, _ time.Duration) *redis.StatusCmd {
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
	r.GET("/books", h.FindBooks)

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

func TestFindBooksInvalidSort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h := NewBookHandler(repository.NewMockBookPersistence(ctrl), cache.NewMockCache(ctrl))
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/books?sort=password", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid sort field")
}

func TestFindBooksInvalidOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h := NewBookHandler(repository.NewMockBookPersistence(ctrl), cache.NewMockCache(ctrl))
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/books?order=sideways", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid order")
}

func TestFindBooksInvalidOwnerID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	h := NewBookHandler(repository.NewMockBookPersistence(ctrl), cache.NewMockCache(ctrl))
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/books?owner_id=abc", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid owner_id format")
}

func TestFindBooksFiltersAndSort(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockCache := cache.NewMockCache(ctrl)

	dbPath := filepath.Join(t.TempDir(), "findbooks_filter.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Book{}))
	store := repository.NewGormBookStore(db)
	assert.NoError(t, store.Create(&models.Book{OwnerID: 1, Title: "Go in Action", Author: "Kennedy"}))
	assert.NoError(t, store.Create(&models.Book{OwnerID: 1, Title: "Rust Book", Author: "Matsakis"}))
	assert.NoError(t, store.Create(&models.Book{OwnerID: 2, Title: "Go Patterns", Author: "Kennedy"}))

	q := repository.BookListQuery{
		Offset: 0, Limit: 10, TitleLike: "go", AuthorLike: "kennedy",
		Sort: "title", Order: "asc",
	}
	owner := uint(1)
	q.OwnerID = &owner
	dataKey := service.BooksListDataCacheKey(0, q)

	gomock.InOrder(
		mockCache.EXPECT().Get(gomock.Any(), service.BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockCache.EXPECT().Get(gomock.Any(), dataKey).Return(redis.NewStringResult("", redis.Nil)),
	)
	mockCache.EXPECT().Set(gomock.Any(), dataKey, gomock.Any(), time.Minute).Return(redis.NewStatusResult("OK", nil)).Times(1)

	h := NewBookHandler(store, mockCache)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/books?title_like=go&author_like=kennedy&owner_id=1&sort=title&order=asc", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var envelope struct {
		Data []models.Book `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
	if assert.Len(t, envelope.Data, 1) {
		assert.Equal(t, "Go in Action", envelope.Data[0].Title)
	}
}

func TestFindBooksFilterCacheIsolation(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockCache := cache.NewMockCache(ctrl)

	dbPath := filepath.Join(t.TempDir(), "findbooks_cache_iso.sqlite")
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	assert.NoError(t, err)
	assert.NoError(t, db.AutoMigrate(&models.Book{}))
	store := repository.NewGormBookStore(db)
	assert.NoError(t, store.Create(&models.Book{OwnerID: 1, Title: "Alpha", Author: "a"}))
	assert.NoError(t, store.Create(&models.Book{OwnerID: 1, Title: "Beta", Author: "b"}))

	qGo := defaultBookListQuery(0, 10)
	qGo.TitleLike = "alpha"
	qRust := defaultBookListQuery(0, 10)
	qRust.TitleLike = "beta"
	keyGo := service.BooksListDataCacheKey(0, qGo)
	keyRust := service.BooksListDataCacheKey(0, qRust)
	assert.NotEqual(t, keyGo, keyRust)

	payload, err := json.Marshal([]models.Book{{Title: "Alpha", Author: "a", OwnerID: 1}})
	assert.NoError(t, err)

	mockCache.EXPECT().Get(gomock.Any(), service.BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)).Times(2)
	mockCache.EXPECT().Get(gomock.Any(), keyGo).Return(redis.NewStringResult(string(payload), nil))
	mockCache.EXPECT().Get(gomock.Any(), keyRust).Return(redis.NewStringResult("", redis.Nil))
	mockCache.EXPECT().Set(gomock.Any(), keyRust, gomock.Any(), time.Minute).Return(redis.NewStatusResult("OK", nil))

	h := NewBookHandler(store, mockCache)
	gin.SetMode(gin.TestMode)
	r := gin.Default()
	r.GET("/books", h.FindBooks)

	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, httptest.NewRequest(http.MethodGet, "/books?title_like=alpha", nil))
	assert.Equal(t, http.StatusOK, w1.Code)

	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodGet, "/books?title_like=beta", nil))
	assert.Equal(t, http.StatusOK, w2.Code)

	var env2 struct {
		Data []models.Book `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w2.Body.Bytes(), &env2))
	if assert.Len(t, env2.Data, 1) {
		assert.Equal(t, "Beta", env2.Data[0].Title)
	}
}
