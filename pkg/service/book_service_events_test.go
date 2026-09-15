package service

import (
	"context"
	"errors"
	"testing"

	"golang-rest-api-template/pkg/events"
	"golang-rest-api-template/pkg/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakePublisher records published events and can be made to fail.
type fakePublisher struct {
	events []events.Event
	err    error
}

func (f *fakePublisher) Publish(_ context.Context, e events.Event) error {
	f.events = append(f.events, e)
	return f.err
}

func (f *fakePublisher) Close() error { return f.err }

func TestCreateBookPublishesCreatedEvent(t *testing.T) {
	store := &fakeBookStore{
		createFn: func(book *models.Book) error {
			book.ID = 42
			return nil
		},
	}
	pub := &fakePublisher{}
	svc := NewBookService(store, nil, pub)

	_, err := svc.CreateBook(context.Background(), 7, "Dune", "Herbert")
	require.NoError(t, err)

	require.Len(t, pub.events, 1)
	e := pub.events[0]
	assert.Equal(t, events.TypeBookCreated, e.Type)
	assert.Equal(t, events.AggregateBooks, e.Aggregate)
	assert.Equal(t, "42", e.Key)
	assert.False(t, e.OccurredAt.IsZero())
	assert.Equal(t, events.BookPayloadV1{ID: 42, OwnerID: 7, Title: "Dune", Author: "Herbert"}, e.Payload)
}

func TestReplaceBookPublishesUpdatedEvent(t *testing.T) {
	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			return &models.Book{ID: 1, OwnerID: 5, Title: "old", Author: "old"}, nil
		},
		updateFn: func(id uint, title, author string) (*models.Book, error) {
			return &models.Book{ID: 1, OwnerID: 5, Title: title, Author: author}, nil
		},
	}
	pub := &fakePublisher{}
	svc := NewBookService(store, nil, pub)

	_, err := svc.ReplaceBook(context.Background(), 5, 1, "new", "author")
	require.NoError(t, err)

	require.Len(t, pub.events, 1)
	assert.Equal(t, events.TypeBookUpdated, pub.events[0].Type)
	assert.Equal(t, events.BookPayloadV1{ID: 1, OwnerID: 5, Title: "new", Author: "author"}, pub.events[0].Payload)
}

func TestPatchBookPublishesUpdatedEvent(t *testing.T) {
	title := "patched"
	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			return &models.Book{ID: 2, OwnerID: 5, Title: "old", Author: "a"}, nil
		},
		patchFn: func(id uint, t, a *string) (*models.Book, error) {
			return &models.Book{ID: 2, OwnerID: 5, Title: *t, Author: "a"}, nil
		},
	}
	pub := &fakePublisher{}
	svc := NewBookService(store, nil, pub)

	_, err := svc.PatchBook(context.Background(), 5, 2, &title, nil)
	require.NoError(t, err)

	require.Len(t, pub.events, 1)
	assert.Equal(t, events.TypeBookUpdated, pub.events[0].Type)
}

func TestDeleteBookPublishesDeletedEventWithPreDeleteSnapshot(t *testing.T) {
	store := &fakeBookStore{
		firstFn: func(id uint) (*models.Book, error) {
			return &models.Book{ID: 9, OwnerID: 5, Title: "gone", Author: "ghost"}, nil
		},
		deleteFn: func(id uint) error { return nil },
	}
	pub := &fakePublisher{}
	svc := NewBookService(store, nil, pub)

	err := svc.DeleteBook(context.Background(), 5, 9)
	require.NoError(t, err)

	require.Len(t, pub.events, 1)
	e := pub.events[0]
	assert.Equal(t, events.TypeBookDeleted, e.Type)
	// The deleted event carries the pre-delete snapshot, not just the id.
	assert.Equal(t, events.BookPayloadV1{ID: 9, OwnerID: 5, Title: "gone", Author: "ghost"}, e.Payload)
}

func TestPublishFailureDoesNotFailWrite(t *testing.T) {
	store := &fakeBookStore{
		createFn: func(book *models.Book) error {
			book.ID = 1
			return nil
		},
	}
	pub := &fakePublisher{err: errors.New("broker unavailable")}
	svc := NewBookService(store, nil, pub)

	book, err := svc.CreateBook(context.Background(), 1, "t", "a")
	// Best-effort: a publish failure is swallowed; the mutation still succeeds.
	require.NoError(t, err)
	require.NotNil(t, book)
	assert.Equal(t, uint(1), book.ID)
	require.Len(t, pub.events, 1)
}

func TestNilPublisherDoesNotPanic(t *testing.T) {
	store := &fakeBookStore{
		createFn: func(book *models.Book) error { book.ID = 1; return nil },
	}
	// nil publisher is the default "no publishing" mode.
	svc := NewBookService(store, nil, nil)

	_, err := svc.CreateBook(context.Background(), 1, "t", "a")
	assert.NoError(t, err)
}
