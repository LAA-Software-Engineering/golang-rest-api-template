package service

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

type fakeBookStore struct {
	listFn   func(q repository.BookListQuery) ([]models.Book, error)
	createFn func(book *models.Book) error
	firstFn  func(id uint) (*models.Book, error)
	updateFn func(id uint, title, author string) (*models.Book, error)
	patchFn  func(id uint, title, author *string) (*models.Book, error)
	deleteFn func(id uint) error
}

func (f *fakeBookStore) List(q repository.BookListQuery) ([]models.Book, error) {
	if f.listFn != nil {
		return f.listFn(q)
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

func defaultListQuery(offset, limit int) repository.BookListQuery {
	return repository.BookListQuery{
		Offset: offset,
		Limit:  limit,
		Sort:   "id",
		Order:  "asc",
	}
}

func TestBooksListDataCacheKey(t *testing.T) {
	q := repository.BookListQuery{
		Offset: 5, Limit: 10, TitleLike: "go", AuthorLike: "ann",
		Sort: "title", Order: "desc",
	}
	assert.Equal(t,
		"books_g2_offset_5_limit_10_title_go_author_ann_owner___sort_title_order_desc",
		BooksListDataCacheKey(2, q),
	)
	owner := uint(7)
	q.OwnerID = &owner
	assert.Equal(t,
		"books_g2_offset_5_limit_10_title_go_author_ann_owner_7_sort_title_order_desc",
		BooksListDataCacheKey(2, q),
	)
}

func TestListBooksNilRedisUsesStore(t *testing.T) {
	want := []models.Book{{ID: 1, Title: "a", Author: "b"}}
	store := &fakeBookStore{
		listFn: func(q repository.BookListQuery) ([]models.Book, error) {
			assert.Equal(t, 3, q.Offset)
			assert.Equal(t, 7, q.Limit)
			assert.Equal(t, "go", q.TitleLike)
			return want, nil
		},
	}
	svc := NewBookService(store, nil)
	q := defaultListQuery(3, 7)
	q.TitleLike = "go"
	got, err := svc.ListBooks(context.Background(), q)
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
	q := defaultListQuery(0, 10)
	dataKey := BooksListDataCacheKey(0, q)

	gomock.InOrder(
		mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockRedis.EXPECT().Get(gomock.Any(), dataKey).Return(redis.NewStringResult(string(payload), nil)),
	)

	store := &fakeBookStore{
		listFn: func(q repository.BookListQuery) ([]models.Book, error) {
			t.Fatal("store.List should not run on cache hit")
			return nil, nil
		},
	}
	svc := NewBookService(store, mockRedis)
	got, err := svc.ListBooks(context.Background(), q)
	assert.NoError(t, err)
	assert.Equal(t, want, got)
}

func TestListBooksCacheUnmarshalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	q := defaultListQuery(1, 5)
	dataKey := BooksListDataCacheKey(0, q)

	gomock.InOrder(
		mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("0", nil)),
		mockRedis.EXPECT().Get(gomock.Any(), dataKey).Return(redis.NewStringResult("not-json", nil)),
	)

	svc := NewBookService(&fakeBookStore{}, mockRedis)
	_, err := svc.ListBooks(context.Background(), q)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrListBooksUnmarshal)
}

func TestListBooksStoreError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	q := defaultListQuery(0, 10)
	dataKey := BooksListDataCacheKey(0, q)
	dbErr := errors.New("db down")

	gomock.InOrder(
		mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockRedis.EXPECT().Get(gomock.Any(), dataKey).Return(redis.NewStringResult("", redis.Nil)),
	)

	store := &fakeBookStore{
		listFn: func(q repository.BookListQuery) ([]models.Book, error) {
			return nil, dbErr
		},
	}
	svc := NewBookService(store, mockRedis)
	_, err := svc.ListBooks(context.Background(), q)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrListBooksDB)
}

func TestListBooksSingleflightCoalescesConcurrentMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)

	var listCalls atomic.Int32
	store := &fakeBookStore{
		listFn: func(q repository.BookListQuery) ([]models.Book, error) {
			listCalls.Add(1)
			time.Sleep(25 * time.Millisecond)
			return []models.Book{{ID: 1, Title: "t", Author: "a"}}, nil
		},
	}

	q := defaultListQuery(0, 10)
	dataKey := BooksListDataCacheKey(0, q)
	var mu sync.Mutex
	cached := ""

	mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)).AnyTimes()
	mockRedis.EXPECT().Get(gomock.Any(), dataKey).DoAndReturn(func(_ context.Context, key string) *redis.StringCmd {
		if key != dataKey {
			return redis.NewStringResult("", errors.New("unexpected key"))
		}
		mu.Lock()
		s := cached
		mu.Unlock()
		if s == "" {
			return redis.NewStringResult("", redis.Nil)
		}
		return redis.NewStringResult(s, nil)
	}).MinTimes(40)
	mockRedis.EXPECT().Set(gomock.Any(), dataKey, gomock.Any(), time.Minute).DoAndReturn(func(_ context.Context, _ string, v interface{}, _ time.Duration) *redis.StatusCmd {
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
		mu.Lock()
		cached = string(b)
		mu.Unlock()
		return redis.NewStatusResult("OK", nil)
	}).Times(1)

	svc := NewBookService(store, mockRedis)
	const n = 40
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, _ = svc.ListBooks(context.Background(), q)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(1), listCalls.Load(), "store.List must run once for coalesced cache misses (#124)")
}

func TestListBooksRedisSetError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)
	q := defaultListQuery(0, 10)
	dataKey := BooksListDataCacheKey(0, q)
	want := []models.Book{{Title: "t", Author: "a"}}

	gomock.InOrder(
		mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)),
		mockRedis.EXPECT().Get(gomock.Any(), dataKey).Return(redis.NewStringResult("", redis.Nil)),
	)
	mockRedis.EXPECT().Set(gomock.Any(), dataKey, gomock.Any(), time.Minute).Return(redis.NewStatusResult("", errors.New("set failed")))

	store := &fakeBookStore{
		listFn: func(q repository.BookListQuery) ([]models.Book, error) {
			return want, nil
		},
	}
	svc := NewBookService(store, mockRedis)
	_, err := svc.ListBooks(context.Background(), q)
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrListBooksRedis)
}

func TestListBooksDistinctFiltersUseDistinctCacheKeys(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockRedis := cache.NewMockCache(ctrl)

	q1 := defaultListQuery(0, 10)
	q1.TitleLike = "go"
	q2 := defaultListQuery(0, 10)
	q2.TitleLike = "rust"
	key1 := BooksListDataCacheKey(0, q1)
	key2 := BooksListDataCacheKey(0, q2)
	assert.NotEqual(t, key1, key2)

	payload, err := json.Marshal([]models.Book{{Title: "go book"}})
	assert.NoError(t, err)

	mockRedis.EXPECT().Get(gomock.Any(), BooksListCacheGenKey).Return(redis.NewStringResult("", redis.Nil)).Times(2)
	mockRedis.EXPECT().Get(gomock.Any(), key1).Return(redis.NewStringResult(string(payload), nil))
	mockRedis.EXPECT().Get(gomock.Any(), key2).Return(redis.NewStringResult("", redis.Nil))
	mockRedis.EXPECT().Set(gomock.Any(), key2, gomock.Any(), time.Minute).Return(redis.NewStatusResult("OK", nil))

	var listedQ repository.BookListQuery
	store := &fakeBookStore{
		listFn: func(q repository.BookListQuery) ([]models.Book, error) {
			listedQ = q
			return []models.Book{{Title: "rust book"}}, nil
		},
	}
	svc := NewBookService(store, mockRedis)

	got1, err := svc.ListBooks(context.Background(), q1)
	assert.NoError(t, err)
	assert.Equal(t, "go book", got1[0].Title)

	got2, err := svc.ListBooks(context.Background(), q2)
	assert.NoError(t, err)
	assert.Equal(t, "rust book", got2[0].Title)
	assert.Equal(t, "rust", listedQ.TitleLike)
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
