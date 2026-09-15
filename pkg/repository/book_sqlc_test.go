package repository

import (
	"context"
	"sync"
	"testing"

	"golang-rest-api-template/internal/pgtest"
	"golang-rest-api-template/pkg/models"

	"github.com/stretchr/testify/assert"
)

func newBookStore(t *testing.T) *SQLCBookStore {
	t.Helper()
	return NewSQLCBookStore(pgtest.Pool(t))
}

func TestSQLCBookStoreListCreateFirstByID(t *testing.T) {
	s := newBookStore(t)

	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "A", Author: "1"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "B", Author: "2"}))

	list, err := s.List(context.Background(), BookListQuery{Offset: 0, Limit: 10})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 2) {
		return
	}

	got, err := s.FirstByID(context.Background(), list[0].ID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, list[0].Title, got.Title)
}

func TestSQLCBookStoreFirstByIDNotFound(t *testing.T) {
	s := newBookStore(t)

	_, err := s.FirstByID(context.Background(), 999)
	assert.Error(t, err)
	assert.True(t, IsBookNotFound(err))
}

func TestSQLCBookStoreListOffsetLimit(t *testing.T) {
	s := newBookStore(t)
	for i := 0; i < 5; i++ {
		assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: string(rune('A' + i)), Author: "x"}))
	}

	page, err := s.List(context.Background(), BookListQuery{Offset: 1, Limit: 2})
	if !assert.NoError(t, err) {
		return
	}
	assert.Len(t, page, 2)
}

func TestSQLCBookStoreListTitleLike(t *testing.T) {
	s := newBookStore(t)
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "The Go Programming Language", Author: "Donovan"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Clean Code", Author: "Martin"}))

	list, err := s.List(context.Background(), BookListQuery{Offset: 0, Limit: 10, TitleLike: "go"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, "The Go Programming Language", list[0].Title)
}

func TestSQLCBookStoreListAuthorLike(t *testing.T) {
	s := newBookStore(t)
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "A", Author: "Alice Smith"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "B", Author: "Bob Jones"}))

	list, err := s.List(context.Background(), BookListQuery{Offset: 0, Limit: 10, AuthorLike: "smith"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, "Alice Smith", list[0].Author)
}

func TestSQLCBookStoreListOwnerID(t *testing.T) {
	s := newBookStore(t)
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "A", Author: "x"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 2, Title: "B", Author: "y"}))

	owner := uint(2)
	list, err := s.List(context.Background(), BookListQuery{Offset: 0, Limit: 10, OwnerID: &owner})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, uint(2), list[0].OwnerID)
}

func TestSQLCBookStoreListSortDesc(t *testing.T) {
	s := newBookStore(t)
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Alpha", Author: "z"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Beta", Author: "a"}))

	list, err := s.List(context.Background(), BookListQuery{Offset: 0, Limit: 10, Sort: "title", Order: "desc"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 2) {
		return
	}
	assert.Equal(t, "Beta", list[0].Title)
	assert.Equal(t, "Alpha", list[1].Title)
}

func TestSQLCBookStoreListCombinedFiltersAndPagination(t *testing.T) {
	s := newBookStore(t)
	owner := uint(1)
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Go Basics", Author: "Ann"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Go Advanced", Author: "Ann"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Go Expert", Author: "Ann"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 2, Title: "Go Other", Author: "Ann"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Rust", Author: "Ann"}))

	list, err := s.List(context.Background(), BookListQuery{
		Offset:    1,
		Limit:     1,
		TitleLike: "go",
		OwnerID:   &owner,
		Sort:      "title",
		Order:     "asc",
	})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, "Go Basics", list[0].Title)
}

func TestSQLCBookStoreListLikeMetacharactersLiteral(t *testing.T) {
	s := newBookStore(t)
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "100% Pure", Author: "x"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "100 Pure", Author: "x"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "under_score", Author: "y"}))

	list, err := s.List(context.Background(), BookListQuery{Offset: 0, Limit: 10, TitleLike: "100%"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, "100% Pure", list[0].Title)

	list, err = s.List(context.Background(), BookListQuery{Offset: 0, Limit: 10, TitleLike: "under_"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, "under_score", list[0].Title)
}

func TestSQLCBookStoreListStablePaginationOnTiedTitle(t *testing.T) {
	s := newBookStore(t)
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Same", Author: "a"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Same", Author: "b"}))
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "Same", Author: "c"}))

	page1, err := s.List(context.Background(), BookListQuery{Offset: 0, Limit: 2, Sort: "title", Order: "asc"})
	if !assert.NoError(t, err) {
		return
	}
	page2, err := s.List(context.Background(), BookListQuery{Offset: 2, Limit: 2, Sort: "title", Order: "asc"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, page1, 2) || !assert.Len(t, page2, 1) {
		return
	}
	seen := map[uint]struct{}{}
	for _, b := range append(page1, page2...) {
		if _, ok := seen[b.ID]; ok {
			t.Fatalf("duplicate id %d across pages", b.ID)
		}
		seen[b.ID] = struct{}{}
	}
	assert.True(t, page1[0].ID < page1[1].ID, "tie-breaker should order by id ASC")
	assert.True(t, page1[1].ID < page2[0].ID)
}

func TestSQLCBookStoreUpdateFields(t *testing.T) {
	s := newBookStore(t)
	b := &models.Book{OwnerID: 1, Title: "old", Author: "old"}
	assert.NoError(t, s.Create(context.Background(), b))

	out, err := s.UpdateFields(context.Background(), b.ID, "newt", "newa")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "newt", out.Title)
	assert.Equal(t, "newa", out.Author)

	reloaded, err := s.FirstByID(context.Background(), b.ID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "newt", reloaded.Title)
}

func TestSQLCBookStoreUpdateFieldsNotFound(t *testing.T) {
	s := newBookStore(t)

	_, err := s.UpdateFields(context.Background(), 42, "x", "y")
	assert.Error(t, err)
	assert.True(t, IsBookNotFound(err))
}

func TestSQLCBookStorePatchFieldsTitleOnly(t *testing.T) {
	s := newBookStore(t)
	b := &models.Book{OwnerID: 1, Title: "orig", Author: "keep"}
	assert.NoError(t, s.Create(context.Background(), b))
	newTitle := "patched"
	out, err := s.PatchFields(context.Background(), b.ID, &newTitle, nil)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "patched", out.Title)
	assert.Equal(t, "keep", out.Author)
	reloaded, err := s.FirstByID(context.Background(), b.ID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "patched", reloaded.Title)
	assert.Equal(t, "keep", reloaded.Author)
}

func TestSQLCBookStorePatchFieldsAuthorOnly(t *testing.T) {
	s := newBookStore(t)
	b := &models.Book{OwnerID: 1, Title: "keep", Author: "orig"}
	assert.NoError(t, s.Create(context.Background(), b))
	newAuthor := "new-author"
	out, err := s.PatchFields(context.Background(), b.ID, nil, &newAuthor)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "keep", out.Title)
	assert.Equal(t, "new-author", out.Author)
}

func TestSQLCBookStorePatchFieldsNoChange(t *testing.T) {
	s := newBookStore(t)
	b := &models.Book{OwnerID: 1, Title: "keep", Author: "same"}
	assert.NoError(t, s.Create(context.Background(), b))
	out, err := s.PatchFields(context.Background(), b.ID, nil, nil)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "keep", out.Title)
	assert.Equal(t, "same", out.Author)
}

func TestSQLCBookStoreDeleteByID(t *testing.T) {
	s := newBookStore(t)
	b := &models.Book{OwnerID: 1, Title: "gone", Author: "soon"}
	assert.NoError(t, s.Create(context.Background(), b))

	assert.NoError(t, s.DeleteByID(context.Background(), b.ID))
	_, err := s.FirstByID(context.Background(), b.ID)
	assert.True(t, IsBookNotFound(err))
}

func TestSQLCBookStoreDeleteByIDNotFound(t *testing.T) {
	s := newBookStore(t)

	err := s.DeleteByID(context.Background(), 99)
	assert.Error(t, err)
	assert.True(t, IsBookNotFound(err))
}

func TestSQLCBookStoreListConcurrent(t *testing.T) {
	s := newBookStore(t)
	assert.NoError(t, s.Create(context.Background(), &models.Book{OwnerID: 1, Title: "c", Author: "c"}))

	var wg sync.WaitGroup
	const n = 32
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := s.List(context.Background(), BookListQuery{Offset: 0, Limit: 10})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}
