package database

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang-rest-api-template/pkg/models"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupSQLiteDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	assert.NoError(t, err)

	err = db.AutoMigrate(&models.Book{}, &models.User{})
	assert.NoError(t, err)

	return db
}

func TestGormDatabaseWhereAndFirst(t *testing.T) {
	db := setupSQLiteDB(t)
	gdb := &GormDatabase{DB: db}

	book := models.Book{Title: "The Cover Test", Author: "Author A"}
	assert.NoError(t, gdb.Create(&book).Error)

	var found models.Book
	result := gdb.Where("title = ?", "The Cover Test").First(&found)
	assert.NoError(t, result.Error())
	assert.Equal(t, book.Title, found.Title)
	assert.Equal(t, book.Author, found.Author)
}

func TestGormDatabaseFirstByID(t *testing.T) {
	db := setupSQLiteDB(t)
	gdb := &GormDatabase{DB: db}

	book := models.Book{Title: "Lookup By ID", Author: "Author B"}
	assert.NoError(t, gdb.Create(&book).Error)

	var found models.Book
	result := gdb.FirstByID(&found, book.ID)
	assert.NoError(t, result.Error())
	assert.Equal(t, book.Title, found.Title)
}

func TestGormDatabaseError(t *testing.T) {
	db := setupSQLiteDB(t)
	gdb := &GormDatabase{DB: db}

	var notFound models.Book
	err := gdb.Where("id = ?", 999).First(&notFound).Error()
	assert.Error(t, err)
	assert.True(t, errors.Is(err, gorm.ErrRecordNotFound))
}

func TestConfigureConnPoolSQLite(t *testing.T) {
	db := setupSQLiteDB(t)
	assert.NoError(t, configureConnPool(db))
}

func TestGetenvPositiveInt(t *testing.T) {
	t.Setenv("DATABASE_TEST_GETENV_INT", "42")
	assert.Equal(t, 42, getenvPositiveInt("DATABASE_TEST_GETENV_INT", 1))
	t.Setenv("DATABASE_TEST_GETENV_INT", "0")
	assert.Equal(t, 99, getenvPositiveInt("DATABASE_TEST_GETENV_INT", 99))
}

func TestGetenvPositiveDuration(t *testing.T) {
	t.Setenv("DATABASE_TEST_GETENV_DUR", "3m")
	assert.Equal(t, 3*time.Minute, getenvPositiveDuration("DATABASE_TEST_GETENV_DUR", time.Second))
	t.Setenv("DATABASE_TEST_GETENV_DUR", "not-a-duration")
	assert.Equal(t, 5*time.Second, getenvPositiveDuration("DATABASE_TEST_GETENV_DUR", 5*time.Second))
}

func TestNewDatabaseInvalidPostgresEnv(t *testing.T) {
	originalSleep := sleep
	sleep = func(time.Duration) {}
	defer func() { sleep = originalSleep }()

	oldHost, hadHost := os.LookupEnv("POSTGRES_HOST")
	oldDB, hadDB := os.LookupEnv("POSTGRES_DB")
	oldUser, hadUser := os.LookupEnv("POSTGRES_USER")
	oldPass, hadPass := os.LookupEnv("POSTGRES_PASSWORD")
	oldPort, hadPort := os.LookupEnv("POSTGRES_PORT")

	defer func() {
		if hadHost {
			os.Setenv("POSTGRES_HOST", oldHost)
		} else {
			os.Unsetenv("POSTGRES_HOST")
		}
		if hadDB {
			os.Setenv("POSTGRES_DB", oldDB)
		} else {
			os.Unsetenv("POSTGRES_DB")
		}
		if hadUser {
			os.Setenv("POSTGRES_USER", oldUser)
		} else {
			os.Unsetenv("POSTGRES_USER")
		}
		if hadPass {
			os.Setenv("POSTGRES_PASSWORD", oldPass)
		} else {
			os.Unsetenv("POSTGRES_PASSWORD")
		}
		if hadPort {
			os.Setenv("POSTGRES_PORT", oldPort)
		} else {
			os.Unsetenv("POSTGRES_PORT")
		}
	}()

	os.Setenv("POSTGRES_HOST", "127.0.0.1")
	os.Setenv("POSTGRES_DB", "invalid_db")
	os.Setenv("POSTGRES_USER", "invalid_user")
	os.Setenv("POSTGRES_PASSWORD", "invalid_pass")
	os.Setenv("POSTGRES_PORT", "1")

	db := NewDatabase()
	assert.Nil(t, db)
}
