package cache

//go:generate go run -mod=mod go.uber.org/mock/mockgen@v0.6.0 -destination=cache_mock.go -package=cache golang-rest-api-template/pkg/cache Cache

import (
	"context"
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

// Cache abstracts Redis operations used by the app. Book list caching invalidates
// via a monotonic generation counter (see pkg/service BooksListCacheGenKey); it
// must not use Redis KEYS (O(n) on the keyspace). Callers needing pattern deletes
// should use Del with explicit keys or extend this interface with SCAN-based APIs.
type Cache interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Incr(ctx context.Context, key string) *redis.IntCmd
	// Eval runs a Lua script against Redis (used for atomic multi-command updates).
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd
	Del(context.Context, ...string) *redis.IntCmd
}

// App Redis client must satisfy Cache without requiring Redis KEYS on the keyspace.
var _ Cache = (*redis.Client)(nil)

// NewRedisClient builds a go-redis client from environment variables.
// See redisOptionsFromEnv for supported keys and defaults.
func NewRedisClient() (*redis.Client, error) {
	opts, err := redisOptionsFromEnv()
	if err != nil {
		return nil, err
	}
	return redis.NewClient(opts), nil
}

func redisOptionsFromEnv() (*redis.Options, error) {
	opts := &redis.Options{
		Password: strings.TrimSpace(os.Getenv("REDIS_PASSWORD")),
	}

	if u := strings.TrimSpace(os.Getenv("REDIS_USERNAME")); u != "" {
		opts.Username = u
	}

	addr := strings.TrimSpace(os.Getenv("REDIS_ADDR"))
	if addr != "" {
		opts.Addr = addr
	} else {
		host := strings.TrimSpace(os.Getenv("REDIS_HOST"))
		if host == "" {
			host = "127.0.0.1"
		}
		port := strings.TrimSpace(os.Getenv("REDIS_PORT"))
		if port == "" {
			port = "6379"
		}
		opts.Addr = host + ":" + port
	}

	if s := strings.TrimSpace(os.Getenv("REDIS_DB")); s != "" {
		db, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("REDIS_DB: %w", err)
		}
		if db < 0 {
			return nil, fmt.Errorf("REDIS_DB must be >= 0, got %d", db)
		}
		opts.DB = db
	}

	dial, err := durationFromEnv("REDIS_DIAL_TIMEOUT", 5*time.Second)
	if err != nil {
		return nil, err
	}
	read, err := durationFromEnv("REDIS_READ_TIMEOUT", 3*time.Second)
	if err != nil {
		return nil, err
	}
	write, err := durationFromEnv("REDIS_WRITE_TIMEOUT", 3*time.Second)
	if err != nil {
		return nil, err
	}
	opts.DialTimeout = dial
	opts.ReadTimeout = read
	opts.WriteTimeout = write

	if envTruthy("REDIS_TLS") {
		cfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if envTruthy("REDIS_TLS_INSECURE") {
			// Development only (e.g. self-signed certs). Do not set in production.
			cfg.InsecureSkipVerify = true // #nosec G402 -- REDIS_TLS_INSECURE explicit opt-in for non-prod TLS
		}
		opts.TLSConfig = cfg
	}

	return opts, nil
}

// envTruthy reports whether the named environment variable is set to a
// conventional affirmative value (1, true, yes, on), case-insensitive.
func envTruthy(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func durationFromEnv(key string, defaultVal time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return defaultVal, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s must be a Go duration (e.g. 5s): %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive, got %s", key, v)
	}
	return d, nil
}
