// Package pgtest provides a shared PostgreSQL testcontainer for repository and
// handler tests. It replaces the previous SQLite-in-memory GORM test fixtures:
// sqlc generates Postgres-specific code, so tests must run against a real Postgres.
//
// One container is started lazily per test binary (package) and reused across
// tests in that package. Pool(t) returns a pool against a freshly truncated set
// of application tables so tests remain isolated.
package pgtest

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"golang-rest-api-template/pkg/database"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	once       sync.Once
	sharedPool *pgxpool.Pool
	startErr   error
)

// start boots a single Postgres container for the current test binary and applies
// the application schema. Subsequent calls reuse it.
func start() {
	ctx := context.Background()
	c, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("test_db"),
		postgres.WithUsername("test_user"),
		postgres.WithPassword("test_pass"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		startErr = fmt.Errorf("start postgres container: %w", err)
		return
	}

	dsn, err := c.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		startErr = fmt.Errorf("connection string: %w", err)
		return
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		startErr = fmt.Errorf("open pool: %w", err)
		return
	}
	if err := database.ApplySchema(ctx, pool); err != nil {
		startErr = fmt.Errorf("apply schema: %w", err)
		return
	}
	sharedPool = pool
}

// Pool returns a connection pool to the shared test database with all application
// tables truncated (and identity sequences reset) so each test starts clean.
// The test is skipped if Docker is not available.
func Pool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	once.Do(start)
	if startErr != nil {
		t.Skipf("pgtest: skipping, could not start Postgres container: %v", startErr)
	}
	truncate(t, sharedPool)
	return sharedPool
}

// truncate resets all application tables to an empty state with id sequences at 1.
func truncate(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()
	_, err := pool.Exec(ctx, `TRUNCATE books, users, refresh_tokens RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("pgtest: truncate: %v", err)
	}
}
