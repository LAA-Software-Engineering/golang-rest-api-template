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
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
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

	assert.NoError(t, s.Create(&models.Book{Title: "A", Author: "1"}))
	assert.NoError(t, s.Create(&models.Book{Title: "B", Author: "2"}))

	list, err := s.List(0, 10)
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
		assert.NoError(t, s.Create(&models.Book{Title: string(rune('A' + i)), Author: "x"}))
	}

	page, err := s.List(1, 2)
	if !assert.NoError(t, err) {
		return
	}
	assert.Len(t, page, 2)
}

func TestGormBookStoreUpdateFields(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	b := &models.Book{Title: "old", Author: "old"}
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

func TestGormBookStoreDeleteByID(t *testing.T) {
	db := testBookDB(t)
	s := NewGormBookStore(db)
	b := &models.Book{Title: "gone", Author: "soon"}
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
	assert.NoError(t, s.Create(&models.Book{Title: "c", Author: "c"}))

	var wg sync.WaitGroup
	const n = 32
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			_, err := s.List(0, 10)
			assert.NoError(t, err)
		}()
	}
	wg.Wait()
}
