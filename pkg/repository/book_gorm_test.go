package repository

import (
	"sync"
	"testing"

	"golang-rest-api-template/pkg/models"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func testBookDB(t *testing.T) *gorm.DB {
	t.Helper()
	// Unique DSN per test so shared-cache in-memory DBs do not leak rows across tests.
	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if !assert.NoError(t, err) {
		t.FailNow()
	}
	if !assert.NoError(t, db.AutoMigrate(&models.Book{})) {
		t.FailNow()
	}
	return db
}

func TestGormBookStoreListCreateFirstByID(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)

	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "A", Author: "1"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "B", Author: "2"}))

	list, err := s.List(BookListQuery{Offset: 0, Limit: 10})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 2) {
		return
	}

	got, err := s.FirstByID(list[0].ID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, list[0].Title, got.Title)
}

func TestGormBookStoreFirstByIDNotFound(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)

	_, err := s.FirstByID(999)
	assert.Error(t, err)
	assert.True(t, IsBookNotFound(err))
}

func TestGormBookStoreListOffsetLimit(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	for i := 0; i < 5; i++ {
		assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: string(rune('A' + i)), Author: "x"}))
	}

	page, err := s.List(BookListQuery{Offset: 1, Limit: 2})
	if !assert.NoError(t, err) {
		return
	}
	assert.Len(t, page, 2)
}

func TestGormBookStoreListTitleLike(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "The Go Programming Language", Author: "Donovan"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "Clean Code", Author: "Martin"}))

	list, err := s.List(BookListQuery{Offset: 0, Limit: 10, TitleLike: "go"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, "The Go Programming Language", list[0].Title)
}

func TestGormBookStoreListAuthorLike(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "A", Author: "Alice Smith"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "B", Author: "Bob Jones"}))

	list, err := s.List(BookListQuery{Offset: 0, Limit: 10, AuthorLike: "smith"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, "Alice Smith", list[0].Author)
}

func TestGormBookStoreListOwnerID(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "A", Author: "x"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 2, Title: "B", Author: "y"}))

	owner := uint(2)
	list, err := s.List(BookListQuery{Offset: 0, Limit: 10, OwnerID: &owner})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, uint(2), list[0].OwnerID)
}

func TestGormBookStoreListSortDesc(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "Alpha", Author: "z"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "Beta", Author: "a"}))

	list, err := s.List(BookListQuery{Offset: 0, Limit: 10, Sort: "title", Order: "desc"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 2) {
		return
	}
	assert.Equal(t, "Beta", list[0].Title)
	assert.Equal(t, "Alpha", list[1].Title)
}

func TestGormBookStoreListCombinedFiltersAndPagination(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	owner := uint(1)
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "Go Basics", Author: "Ann"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "Go Advanced", Author: "Ann"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "Go Expert", Author: "Ann"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 2, Title: "Go Other", Author: "Ann"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "Rust", Author: "Ann"}))

	list, err := s.List(BookListQuery{
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

func TestGormBookStoreListLikeMetacharactersLiteral(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "100% Pure", Author: "x"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "100 Pure", Author: "x"}))
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "under_score", Author: "y"}))

	list, err := s.List(BookListQuery{Offset: 0, Limit: 10, TitleLike: "100%"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, "100% Pure", list[0].Title)

	list, err = s.List(BookListQuery{Offset: 0, Limit: 10, TitleLike: "under_"})
	if !assert.NoError(t, err) {
		return
	}
	if !assert.Len(t, list, 1) {
		return
	}
	assert.Equal(t, "under_score", list[0].Title)
}

func TestGormBookStoreUpdateFields(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	b := &models.Book{OwnerID: 1, Title: "old", Author: "old"}
	assert.NoError(t, s.Create(b))

	out, err := s.UpdateFields(b.ID, "newt", "newa")
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "newt", out.Title)
	assert.Equal(t, "newa", out.Author)

	reloaded, err := s.FirstByID(b.ID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "newt", reloaded.Title)
}

func TestGormBookStoreUpdateFieldsNotFound(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)

	_, err := s.UpdateFields(42, "x", "y")
	assert.Error(t, err)
	assert.True(t, IsBookNotFound(err))
}

func TestGormBookStorePatchFieldsTitleOnly(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	b := &models.Book{OwnerID: 1, Title: "orig", Author: "keep"}
	assert.NoError(t, s.Create(b))
	newTitle := "patched"
	out, err := s.PatchFields(b.ID, &newTitle, nil)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "patched", out.Title)
	assert.Equal(t, "keep", out.Author)
	reloaded, err := s.FirstByID(b.ID)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "patched", reloaded.Title)
	assert.Equal(t, "keep", reloaded.Author)
}

func TestGormBookStorePatchFieldsAuthorOnly(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	b := &models.Book{OwnerID: 1, Title: "keep", Author: "orig"}
	assert.NoError(t, s.Create(b))
	newAuthor := "new-author"
	out, err := s.PatchFields(b.ID, nil, &newAuthor)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, "keep", out.Title)
	assert.Equal(t, "new-author", out.Author)
}

func TestGormBookStoreDeleteByID(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	b := &models.Book{OwnerID: 1, Title: "gone", Author: "soon"}
	assert.NoError(t, s.Create(b))

	assert.NoError(t, s.DeleteByID(b.ID))
	_, err := s.FirstByID(b.ID)
	assert.True(t, IsBookNotFound(err))
}

func TestGormBookStoreDeleteByIDNotFound(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)

	err := s.DeleteByID(99)
	assert.Error(t, err)
	assert.True(t, IsBookNotFound(err))
}

func TestGormBookStoreListConcurrent(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	assert.NoError(t, s.Create(&models.Book{OwnerID: 1, Title: "c", Author: "c"}))

	var wg sync.WaitGroup
	const n = 32
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := s.List(BookListQuery{Offset: 0, Limit: 10})
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}
