package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/models"
	"golang-rest-api-template/pkg/repository"

	"golang.org/x/sync/singleflight"
)

// BooksListCacheGenKey is the Redis key for the books list cache generation counter.
const BooksListCacheGenKey = "v1:books:list_cache_gen"

// Sentinel errors for ListBooks singleflight and callers (e.g. HTTP handlers).
var (
	ErrListBooksDB        = errors.New("service: list books database")
	ErrListBooksMarshal   = errors.New("service: list books marshal")
	ErrListBooksRedis     = errors.New("service: list books redis set")
	ErrListBooksUnmarshal = errors.New("service: list books cache unmarshal")
	ErrBookForbidden      = errors.New("service: book forbidden")
)

// BooksListDataCacheKey is the Redis key for a cached books list page (tests and docs).
func BooksListDataCacheKey(gen int64, offset, limit int) string {
	return fmt.Sprintf("books_g%d_offset_%d_limit_%d", gen, offset, limit)
}

// BookService coordinates book reads/writes, list caching, and cache generation bumps.
type BookService struct {
	store  repository.BookPersistence
	redis  cache.Cache
	listSF singleflight.Group
}

// NewBookService constructs a BookService.
func NewBookService(store repository.BookPersistence, redis cache.Cache) *BookService {
	return &BookService{store: store, redis: redis}
}

func (s *BookService) cacheGeneration(ctx context.Context) int64 {
	if s == nil || s.redis == nil {
		return 0
	}
	n, err := s.redis.Get(ctx, BooksListCacheGenKey).Int64()
	if err != nil {
		return 0
	}
	if n < 0 {
		return 0
	}
	return n
}

func (s *BookService) bumpListCacheGeneration(ctx context.Context) {
	if s == nil || s.redis == nil {
		return
	}
	_, _ = s.redis.Incr(ctx, BooksListCacheGenKey).Result()
}

// ListBooks returns books for offset/limit using Redis list cache and singleflight on miss.
func (s *BookService) ListBooks(ctx context.Context, offset, limit int) ([]models.Book, error) {
	if s.redis == nil {
		return s.store.List(offset, limit)
	}

	gen := s.cacheGeneration(ctx)
	cacheKey := BooksListDataCacheKey(gen, offset, limit)

	cachedBooks, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var books []models.Book
		if err := json.Unmarshal([]byte(cachedBooks), &books); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrListBooksUnmarshal, err)
		}
		return books, nil
	}

	out, err, _ := s.listSF.Do(cacheKey, func() (interface{}, error) {
		loaded, err := s.store.List(offset, limit)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrListBooksDB, err)
		}
		serializedBooks, err := json.Marshal(loaded)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrListBooksMarshal, err)
		}
		if err := s.redis.Set(ctx, cacheKey, serializedBooks, time.Minute).Err(); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrListBooksRedis, err)
		}
		return loaded, nil
	})
	if err != nil {
		return nil, err
	}
	return out.([]models.Book), nil
}

// CreateBook persists a new book owned by ownerID and bumps the list cache generation.
func (s *BookService) CreateBook(ctx context.Context, ownerID uint, title, author string) (*models.Book, error) {
	book := &models.Book{OwnerID: ownerID, Title: title, Author: author}
	if err := s.store.Create(book); err != nil {
		return nil, err
	}
	s.bumpListCacheGeneration(ctx)
	return book, nil
}

// GetBook returns a book by id or gorm.ErrRecordNotFound-compatible error from the store.
func (s *BookService) GetBook(_ context.Context, id uint) (*models.Book, error) {
	return s.store.FirstByID(id)
}

// ReplaceBook replaces title and author when actorID owns the book (PUT semantics).
func (s *BookService) ReplaceBook(ctx context.Context, actorID, id uint, title, author string) (*models.Book, error) {
	b, err := s.store.FirstByID(id)
	if err != nil {
		return nil, err
	}
	if b.OwnerID != actorID {
		return nil, ErrBookForbidden
	}
	book, err := s.store.UpdateFields(id, title, author)
	if err != nil {
		return nil, err
	}
	s.bumpListCacheGeneration(ctx)
	return book, nil
}

// PatchBook applies a partial update for any non-nil title/author pointers (PATCH semantics).
func (s *BookService) PatchBook(ctx context.Context, actorID, id uint, title, author *string) (*models.Book, error) {
	b, err := s.store.FirstByID(id)
	if err != nil {
		return nil, err
	}
	if b.OwnerID != actorID {
		return nil, ErrBookForbidden
	}
	book, err := s.store.PatchFields(id, title, author)
	if err != nil {
		return nil, err
	}
	s.bumpListCacheGeneration(ctx)
	return book, nil
}

// DeleteBook removes a book when actorID owns it, then bumps list cache generation.
func (s *BookService) DeleteBook(ctx context.Context, actorID, id uint) error {
	b, err := s.store.FirstByID(id)
	if err != nil {
		return err
	}
	if b.OwnerID != actorID {
		return ErrBookForbidden
	}
	if err := s.store.DeleteByID(id); err != nil {
		return err
	}
	s.bumpListCacheGeneration(ctx)
	return nil
}
