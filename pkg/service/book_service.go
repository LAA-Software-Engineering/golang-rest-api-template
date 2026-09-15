package service

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/events"
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
// Free-form filter strings are hex-encoded so delimiter substrings cannot collide
// distinct queries onto the same key (or singleflight key).
func BooksListDataCacheKey(gen int64, q repository.BookListQuery) string {
	owner := "_"
	if q.OwnerID != nil {
		owner = fmt.Sprintf("%d", *q.OwnerID)
	}
	return fmt.Sprintf(
		"books_g%d_offset_%d_limit_%d_title_%s_author_%s_owner_%s_sort_%s_order_%s",
		gen, q.Offset, q.Limit,
		hex.EncodeToString([]byte(q.TitleLike)),
		hex.EncodeToString([]byte(q.AuthorLike)),
		owner, q.Sort, q.Order,
	)
}

// BookService coordinates book reads/writes, list caching, cache generation
// bumps, and best-effort domain-event publishing.
type BookService struct {
	store     repository.BookPersistence
	redis     cache.Cache
	publisher events.Publisher
	listSF    singleflight.Group
}

// NewBookService constructs a BookService. redis and publisher are optional
// collaborators: a nil redis disables list caching, and a nil publisher disables
// event publishing (equivalent to events.NopPublisher).
func NewBookService(store repository.BookPersistence, redis cache.Cache, publisher events.Publisher) *BookService {
	return &BookService{store: store, redis: redis, publisher: publisher}
}

// publishBookEvent emits a book domain event best-effort: a publish failure is
// recorded as a metric but never propagated, so it cannot turn an
// already-committed mutation into an error response.
func (s *BookService) publishBookEvent(ctx context.Context, eventType string, b *models.Book) {
	if s == nil || s.publisher == nil || b == nil {
		return
	}
	err := s.publisher.Publish(ctx, events.Event{
		Type:       eventType,
		Aggregate:  events.AggregateBooks,
		Key:        strconv.FormatUint(uint64(b.ID), 10),
		OccurredAt: time.Now().UTC(),
		Payload: events.BookPayloadV1{
			ID:      b.ID,
			OwnerID: b.OwnerID,
			Title:   b.Title,
			Author:  b.Author,
		},
	})
	result := "success"
	if err != nil {
		result = "error"
	}
	events.PublishTotal.WithLabelValues(eventType, result).Inc()
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

// bumpListCacheGeneration invalidates cached list pages by incrementing a global
// generation counter (BooksListCacheGenKey). Cached keys embed the generation, so
// readers miss without scanning or KEYS on Redis (#123).
func (s *BookService) bumpListCacheGeneration(ctx context.Context) {
	if s == nil || s.redis == nil {
		return
	}
	_, _ = s.redis.Incr(ctx, BooksListCacheGenKey).Result()
}

// ListBooks returns books for the given query using Redis list cache and singleflight on miss.
func (s *BookService) ListBooks(ctx context.Context, q repository.BookListQuery) ([]models.Book, error) {
	if s.redis == nil {
		return s.store.List(ctx, q)
	}

	gen := s.cacheGeneration(ctx)
	cacheKey := BooksListDataCacheKey(gen, q)

	cachedBooks, err := s.redis.Get(ctx, cacheKey).Result()
	if err == nil {
		var books []models.Book
		if err := json.Unmarshal([]byte(cachedBooks), &books); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrListBooksUnmarshal, err)
		}
		return books, nil
	}

	out, err, _ := s.listSF.Do(cacheKey, func() (interface{}, error) {
		loaded, err := s.store.List(ctx, q)
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
	if err := s.store.Create(ctx, book); err != nil {
		return nil, err
	}
	s.bumpListCacheGeneration(ctx)
	s.publishBookEvent(ctx, events.TypeBookCreated, book)
	return book, nil
}

// GetBook returns a book by id or a repository.ErrNotFound-compatible error from the store.
func (s *BookService) GetBook(ctx context.Context, id uint) (*models.Book, error) {
	return s.store.FirstByID(ctx, id)
}

// ReplaceBook replaces title and author when actorID owns the book (PUT semantics).
func (s *BookService) ReplaceBook(ctx context.Context, actorID, id uint, title, author string) (*models.Book, error) {
	b, err := s.store.FirstByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.OwnerID != actorID {
		return nil, ErrBookForbidden
	}
	book, err := s.store.UpdateFields(ctx, id, title, author)
	if err != nil {
		return nil, err
	}
	s.bumpListCacheGeneration(ctx)
	s.publishBookEvent(ctx, events.TypeBookUpdated, book)
	return book, nil
}

// PatchBook applies a partial update for any non-nil title/author pointers (PATCH semantics).
func (s *BookService) PatchBook(ctx context.Context, actorID, id uint, title, author *string) (*models.Book, error) {
	b, err := s.store.FirstByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if b.OwnerID != actorID {
		return nil, ErrBookForbidden
	}
	book, err := s.store.PatchFields(ctx, id, title, author)
	if err != nil {
		return nil, err
	}
	s.bumpListCacheGeneration(ctx)
	s.publishBookEvent(ctx, events.TypeBookUpdated, book)
	return book, nil
}

// DeleteBook removes a book when actorID owns it, then bumps list cache generation.
func (s *BookService) DeleteBook(ctx context.Context, actorID, id uint) error {
	b, err := s.store.FirstByID(ctx, id)
	if err != nil {
		return err
	}
	if b.OwnerID != actorID {
		return ErrBookForbidden
	}
	if err := s.store.DeleteByID(ctx, id); err != nil {
		return err
	}
	s.bumpListCacheGeneration(ctx)
	// b is the pre-delete snapshot fetched above, so the deleted event carries
	// the book's owner/title/author rather than just an id.
	s.publishBookEvent(ctx, events.TypeBookDeleted, b)
	return nil
}
