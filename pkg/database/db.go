package database

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"golang-rest-api-template/pkg/models"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var sleep = time.Sleep

const (
	defaultPostgresMaxOpenConns    = 25
	defaultPostgresMaxIdleConns    = 5
	defaultPostgresConnMaxLifetime = time.Hour
	defaultPostgresConnMaxIdleTime = 10 * time.Minute
)

func getenvPositiveInt(key string, def int) int {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		log.Printf("database: invalid %s=%q, using default %d", key, s, def)
		return def
	}
	return n
}

func getenvPositiveDuration(key string, def time.Duration) time.Duration {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		log.Printf("database: invalid %s=%q, using default %v", key, s, def)
		return def
	}
	return d
}

// configureConnPool applies *sql.DB pool settings from POSTGRES_* env vars (with
// sane defaults). max idle connections is capped at max open.
func configureConnPool(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	maxOpen := getenvPositiveInt("POSTGRES_MAX_OPEN_CONNS", defaultPostgresMaxOpenConns)
	maxIdle := getenvPositiveInt("POSTGRES_MAX_IDLE_CONNS", defaultPostgresMaxIdleConns)
	if maxIdle > maxOpen {
		maxIdle = maxOpen
	}
	lifetime := getenvPositiveDuration("POSTGRES_CONN_MAX_LIFETIME", defaultPostgresConnMaxLifetime)
	idleTime := getenvPositiveDuration("POSTGRES_CONN_MAX_IDLE_TIME", defaultPostgresConnMaxIdleTime)

	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(lifetime)
	sqlDB.SetConnMaxIdleTime(idleTime)
	return nil
}

// NewDatabase opens PostgreSQL using POSTGRES_* environment variables, retries on
// failure, runs AutoMigrate for application models, and configures the underlying
// sql.DB pool. It returns nil if the database cannot be opened or configured.
func NewDatabase() *gorm.DB {
	var database *gorm.DB
	var err error

	db_hostname := os.Getenv("POSTGRES_HOST")
	db_name := os.Getenv("POSTGRES_DB")
	db_user := os.Getenv("POSTGRES_USER")
	db_pass := os.Getenv("POSTGRES_PASSWORD")
	db_port := os.Getenv("POSTGRES_PORT")

	dbURl := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", db_user, db_pass, db_hostname, db_port, db_name)

	for i := 1; i <= 3; i++ {
		database, err = gorm.Open(postgres.Open(dbURl), &gorm.Config{})
		if err == nil {
			break
		}
		log.Printf("Attempt %d: Failed to initialize database. Retrying...", i)
		sleep(3 * time.Second)
	}

	if database == nil {
		log.Printf("Failed to initialize database after retries: %v", err)
		return nil
	}

	if err := database.AutoMigrate(&models.Book{}); err != nil {
		log.Printf("Failed to migrate Book model: %v", err)
		return nil
	}

	if err := database.AutoMigrate(&models.User{}); err != nil {
		log.Printf("Failed to migrate User model: %v", err)
		return nil
	}

	if err := configureConnPool(database); err != nil {
		log.Printf("Failed to configure SQL connection pool: %v", err)
		return nil
	}

	return database
}
