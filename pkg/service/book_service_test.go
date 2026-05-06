package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/models"

	"github.com/go-redis/redis/v8"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

type fakeBookStore struct {
	listFn   func(offset, limit int) ([]models.Book, error)
	createFn func(book *models.Book) error
	firstFn  func(id uint) (*models.Book, error)
	updateFn func(id uint, title, author string) (*models.Book, error)
	patchFn  func(id uint, title, author *string) (*models.Book, error)
	deleteFn func(id uint) error
}

func (f *fakeBookStore) List(offset, limit int) ([]models.Book, error) {
	if f.listFn != nil {
		return f.listFn(offset, limit)
	}
	return nil, nil
}

func (f *fakeBookStore) Create(book *models.Book) error {
	if f.createFn != nil {
		return f.createFn(book)
	}
	return nil
}

func (f *fakeBookStore) FirstByID(id uint) (*models.Book, error) {
	if f.firstFn != nil {
		return f.firstFn(id)
	}
	return nil, nil
}

func (f *fakeBookStore) UpdateFields(id uint, title, author string) (*models.Book, error) {
	if f.updateFn != nil {
		return f.updateFn(id, title, author)
	}
	return nil, nil
}

func (f *fakeBookStore) PatchFields(id uint, title, author *string) (*models.Book, error) {
	if f.patchFn != nil {
		return f.patchFn(id, title, author)
	}
	return nil, nil
}

func (f *fakeBookStore) DeleteByID(id uint) error {
	if f.deleteFn != nil {
		return f.deleteFn(id)
	}
	return nil
}

func TestBooksListDataCacheKey(t *testing.T) {
	assert.Equal(t, "books_g2_offset_5_limit_10", BooksListDataCacheKey(2, 5, 10))
}

func TestListBooksNilRedisUsesStore(t *testing.T) {
	want := []models.Book{{ID: 1, Title: "a", Author: "b"}}
	store := &fakeBookStore{
		listFn: func(offset, limit int) ([]models.Book, error) {
			assert.Equal(t, 3, offset)
			assert.Equal(t, 7, limit)
			return want, nil
		},
	}
	svc := NewBookService(store, nil)
	got, err := svc.ListBooks(context.Background(), 3, 7)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListBooksCacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)

	want := []models.Book{{ID: 9, Title: "cached", Author: "x"}}
	payload, err := json.Marshal(want)
	assert.NoError(t, err)
	dataKey := BooksListDataCacheKey(0, 0, 10)

	gomock.InOrder(
		mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockRedis.EXPECT().Get(gomock.Any(), dataKey).Return(redis.NewStringResult(string(payload), nil)),
	)

	store := &fakeBookStore{
		listFn: func(offset, limit int) ([]models.Book, error) {
			t.Fatal("store.List should not run on cache hit")
			return nil, nil
		},
	}
	svc := NewBookService(store, mockRedis)
	got, err := svc.ListBooks(context.Background(), 0, 10)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListBooksCacheUnmarshalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	dataKey := BooksListDataCacheKey(0, 1, 5)

	gomock.InOrder(
		mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("0", nil)),
		mockRedis.EXPECT().Get(gomock.Any(), dataKey).Return(redis.NewStringResult("not-json", nil)),
	)

	svc := NewBookService(&fakeBookStore{}, mockRedis)
	_, err := svc.ListBooks(context.Background(), 1, 5)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrListBooksUnmarshal)
}

func TestListBooksStoreError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	dataKey := BooksListDataCacheKey(0, 0, 10)
	dbErr := errors.New("db down")

	gomock.InOrder(
		mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockRedis.EXPECT().Get(gomock.Any(), dataKey).Return(redis.NewStringResult("", redis.Nil)),
	)

	store := &fakeBookStore{
		listFn: func(offset, limit int) ([]models.Book, error) {
			return nil, dbErr
		},
	}
	svc := NewBookService(store, mockRedis)
	_, err := svc.ListBooks(context.Background(), 0, 10)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrListBooksDB)
}

func TestListBooksRedisSetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	dataKey := BooksListDataCacheKey(0, 0, 10)
	want := []models.Book{{Title: "t", Author: "a"}}

	gomock.InOrder(
		mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockRedis.EXPECT().Get(gomock.Any(), dataKey).Return(redis.NewStringResult("", redis.Nil)),
	)
	mockRedis.EXPECT().Set(gomock.Any(), dataKey, gomock.Any(), time.Minute).Return(redis.NewStatusResult("", errors.New("set failed")))

	store := &fakeBookStore{
		listFn: func(offset, limit int) ([]models.Book, error) {
			return want, nil
		},
	}
	svc := NewBookService(store, mockRedis)
	_, err := svc.ListBooks(context.Background(), 0, 10)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrListBooksRedis)
}

func TestCreateBookBumpsGenerationWhenRedis(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	mockRedis.EXPECT().Incr(gomock.Any(), BooksListCacheGenKey).Return(redis.NewIntResult(1, nil)).Times(1)

	store := &fakeBookStore{
		createFn: func(book *models.Book) error {
			book.ID = 42
			return nil
		},
	}
	svc := NewBookService(store, mockRedis)
	book, err := svc.CreateBook(context.Background(), 1, "t", "a")
	assert.NoError(t, err)
	assert.NotNil(t, book)
	assert.Equal(t, uint(42), book.ID)
	assert.Equal(t, uint(1), book.OwnerID)
}

func TestCreateBookNoIncrWhenNilRedis(t *testing.T) {
	store := &fakeBookStore{
		createFn: func(book *models.Book) error {
			return nil
		},
	}
	svc := NewBookService(store, nil)
	_, err := svc.CreateBook(context.Background(), 1, "t", "a")
	assert.NoError(t, err)
}

func TestGetBookDelegates(t *testing.T) {
	want := &models.Book{ID: 3, Title: "x", Author: "y"}
	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			assert.Equal(t, uint(3), id)
			return want, nil
		},
	}
	svc := NewBookService(store, nil)
	got, err := svc.GetBook(context.Background(), 3)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestReplaceBookBumpsGeneration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	mockRedis.EXPECT().Incr(gomock.Any(), BooksListCacheGenKey).Return(redis.NewIntResult(2, nil)).Times(1)

	updated := &models.Book{ID: 1, OwnerID: 5, Title: "n", Author: "m"}
	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			return &models.Book{ID: 1, OwnerID: 5, Title: "o", Author: "o"}, nil
		},
		updateFn: func(id uint, title, author string) (*models.Book, error) {
			assert.Equal(t, uint(1), id)
			assert.Equal(t, "n", title)
			assert.Equal(t, "m", author)
			return updated, nil
		},
	}
	svc := NewBookService(store, mockRedis)
	got, err := svc.ReplaceBook(context.Background(), 5, 1, "n", "m")
	assert.NoError(t, err)
	assert.Equal(t, updated, got)
}

func TestPatchBookBumpsGenerationTitleOnly(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	mockRedis.EXPECT().Incr(gomock.Any(), BooksListCacheGenKey).Return(redis.NewIntResult(3, nil)).Times(1)

	newTitle := "patched"
	out := &models.Book{ID: 2, OwnerID: 7, Title: newTitle, Author: "kept"}
	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			return &models.Book{ID: 2, OwnerID: 7, Title: "old", Author: "kept"}, nil
		},
		patchFn: func(id uint, title, author *string) (*models.Book, error) {
			assert.Equal(t, uint(2), id)
			assert.NotNil(t, title)
			assert.Nil(t, author)
			assert.Equal(t, newTitle, *title)
			return out, nil
		},
	}
	svc := NewBookService(store, mockRedis)
	got, err := svc.PatchBook(context.Background(), 7, 2, &newTitle, nil)
	assert.NoError(t, err)
	assert.Equal(t, out, got)
}

func TestDeleteBookBumpsGeneration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	mockRedis.EXPECT().Incr(gomock.Any(), BooksListCacheGenKey).Return(redis.NewIntResult(1, nil)).Times(1)

	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			return &models.Book{ID: 9, OwnerID: 3}, nil
		},
		deleteFn: func(id uint) error {
			assert.Equal(t, uint(9), id)
			return nil
		},
	}
	svc := NewBookService(store, mockRedis)
	assert.NoError(t, svc.DeleteBook(context.Background(), 3, 9))
}

func TestReplaceBookWrongOwner(t *testing.T) {
	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			return &models.Book{ID: 1, OwnerID: 1}, nil
		},
	}
	svc := NewBookService(store, nil)
	_, err := svc.ReplaceBook(context.Background(), 2, 1, "x", "y")
	assert.ErrorIs(t, err, ErrBookForbidden)
}

func TestPatchBookWrongOwner(t *testing.T) {
	x := "t"
	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			return &models.Book{ID: 1, OwnerID: 1}, nil
		},
	}
	svc := NewBookService(store, nil)
	_, err := svc.PatchBook(context.Background(), 2, 1, &x, nil)
	assert.ErrorIs(t, err, ErrBookForbidden)
}

func TestDeleteBookWrongOwner(t *testing.T) {
	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			return &models.Book{ID: 1, OwnerID: 1}, nil
		},
	}
	svc := NewBookService(store, nil)
	assert.ErrorIs(t, svc.DeleteBook(context.Background(), 9, 1), ErrBookForbidden)
}
