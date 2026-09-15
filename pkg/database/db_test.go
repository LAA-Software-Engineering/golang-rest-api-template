package database

import (
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClampToInt32(t *testing.T) {
	assert.Equal(t, int32(0), clampToInt32(0))
	assert.Equal(t, int32(25), clampToInt32(25))
	assert.Equal(t, int32(math.MaxInt32), clampToInt32(math.MaxInt32))
	assert.Equal(t, int32(math.MaxInt32), clampToInt32(math.MaxInt32+1))
	assert.Equal(t, int32(0), clampToInt32(-5))
}

func TestApplyPoolConfigDefaults(t *testing.T) {
	cfg, err := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db")
	require.NoError(t, err)

	applyPoolConfig(cfg)

	assert.Equal(t, int32(defaultPostgresMaxOpenConns), cfg.MaxConns)
	assert.Equal(t, int32(defaultPostgresMaxIdleConns), cfg.MinIdleConns)
	assert.Equal(t, defaultPostgresConnMaxLifetime, cfg.MaxConnLifetime)
	assert.Equal(t, defaultPostgresConnMaxIdleTime, cfg.MaxConnIdleTime)
}

func TestApplyPoolConfigFromEnv(t *testing.T) {
	t.Setenv("POSTGRES_MAX_OPEN_CONNS", "10")
	t.Setenv("POSTGRES_MAX_IDLE_CONNS", "50") // capped to max open
	t.Setenv("POSTGRES_CONN_MAX_LIFETIME", "2h")
	t.Setenv("POSTGRES_CONN_MAX_IDLE_TIME", "1m")

	cfg, err := pgxpool.ParseConfig("postgres://u:p@localhost:5432/db")
	require.NoError(t, err)
	applyPoolConfig(cfg)

	assert.Equal(t, int32(10), cfg.MaxConns)
	assert.Equal(t, int32(10), cfg.MinIdleConns, "idle should be capped at max open")
	assert.Equal(t, 2*time.Hour, cfg.MaxConnLifetime)
	assert.Equal(t, time.Minute, cfg.MaxConnIdleTime)
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

	t.Setenv("POSTGRES_HOST", "127.0.0.1")
	t.Setenv("POSTGRES_DB", "invalid_db")
	t.Setenv("POSTGRES_USER", "invalid_user")
	t.Setenv("POSTGRES_PASSWORD", "invalid_pass")
	t.Setenv("POSTGRES_PORT", "1")

	db := NewDatabase()
	assert.Nil(t, db)
}
