package database

import (
	"context"
	_ "embed"
	"fmt"
	"log"
	"math"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var sleep = time.Sleep

// schemaSQL is the application DDL applied at startup (replacing GORM AutoMigrate).
// It is idempotent (CREATE TABLE IF NOT EXISTS) so repeated boots are safe. The
// same file is the schema input for sqlc code generation (see sqlc.yaml).
//
//go:embed schema.sql
var schemaSQL string

const (
	defaultPostgresMaxOpenConns    = 25
	defaultPostgresMaxIdleConns    = 5
	defaultPostgresConnMaxLifetime = time.Hour
	defaultPostgresConnMaxIdleTime = 10 * time.Minute
)

// clampToInt32 narrows a positive int to int32 with an explicit upper-bound
// check, so an out-of-range POSTGRES_* value cannot wrap to a negative or
// nonsensical pool size.
func clampToInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < 0 {
		return 0
	}
	return int32(v)
}

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

// applyPoolConfig sets pgxpool sizing from POSTGRES_* env vars (with sane
// defaults). The mapping to the old database/sql knobs is deliberate and not 1:1,
// because pgxpool's model differs:
//
//   - POSTGRES_MAX_OPEN_CONNS -> MaxConns: hard ceiling on total connections
//     (same meaning as sql.DB.SetMaxOpenConns).
//   - POSTGRES_MAX_IDLE_CONNS -> MinIdleConns: keep at least this many *idle*
//     connections ready. database/sql's SetMaxIdleConns was a ceiling on idle
//     connections; pgxpool has no idle ceiling (idle connections are reaped by
//     MaxConnIdleTime), so MinIdleConns ("keep some idle ready") is the closest
//     analog. MinConns — a pool-wide floor pgxpool actively maintains via its
//     health-check loop — is intentionally left at 0 so a quiet service can drain
//     to zero connections, matching the old idle behavior.
//   - POSTGRES_CONN_MAX_LIFETIME -> MaxConnLifetime (same as SetConnMaxLifetime).
//   - POSTGRES_CONN_MAX_IDLE_TIME -> MaxConnIdleTime (same as SetConnMaxIdleTime).
//
// MinIdleConns is capped at MaxConns.
func applyPoolConfig(cfg *pgxpool.Config) {
	maxOpen := getenvPositiveInt("POSTGRES_MAX_OPEN_CONNS", defaultPostgresMaxOpenConns)
	minIdle := getenvPositiveInt("POSTGRES_MAX_IDLE_CONNS", defaultPostgresMaxIdleConns)
	if minIdle > maxOpen {
		minIdle = maxOpen
	}
	cfg.MaxConns = clampToInt32(maxOpen)
	cfg.MinIdleConns = clampToInt32(minIdle)
	cfg.MaxConnLifetime = getenvPositiveDuration("POSTGRES_CONN_MAX_LIFETIME", defaultPostgresConnMaxLifetime)
	cfg.MaxConnIdleTime = getenvPositiveDuration("POSTGRES_CONN_MAX_IDLE_TIME", defaultPostgresConnMaxIdleTime)
}

// NewDatabase opens a PostgreSQL connection pool using POSTGRES_* environment
// variables, retries on failure, applies the embedded schema, and configures the
// pool. It returns nil if the database cannot be opened, migrated, or configured.
func NewDatabase() *pgxpool.Pool {
	db_hostname := os.Getenv("POSTGRES_HOST")
	db_name := os.Getenv("POSTGRES_DB")
	db_user := os.Getenv("POSTGRES_USER")
	db_pass := os.Getenv("POSTGRES_PASSWORD")
	db_port := os.Getenv("POSTGRES_PORT")

	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", db_user, db_pass, db_hostname, db_port, db_name)

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Printf("database: invalid connection config: %v", err)
		return nil
	}
	applyPoolConfig(cfg)

	var pool *pgxpool.Pool
	for i := 1; i <= 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		pool, err = pgxpool.NewWithConfig(ctx, cfg)
		if err == nil {
			err = pool.Ping(ctx)
		}
		cancel()
		if err == nil {
			break
		}
		if pool != nil {
			pool.Close()
			pool = nil
		}
		log.Printf("Attempt %d: Failed to initialize database. Retrying...", i)
		sleep(3 * time.Second)
	}

	if pool == nil {
		log.Printf("Failed to initialize database after retries: %v", err)
		return nil
	}

	if err := ApplySchema(context.Background(), pool); err != nil {
		log.Printf("Failed to apply database schema: %v", err)
		pool.Close()
		return nil
	}

	return pool
}

// ApplySchema executes the embedded schema against pool. It is idempotent.
func ApplySchema(ctx context.Context, pool *pgxpool.Pool) error {
	_, err := pool.Exec(ctx, schemaSQL)
	return err
}
