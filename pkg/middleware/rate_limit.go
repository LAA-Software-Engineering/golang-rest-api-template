package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/httperr"

	"github.com/gin-gonic/gin"
)

const (
	// DefaultRateLimitRequests is the default max requests per client per window.
	DefaultRateLimitRequests = 60
	// DefaultRateLimitWindow is the default fixed window duration.
	DefaultRateLimitWindow = time.Minute

	rateLimitKeyPrefix     = "v1:ratelimit:"
	rateLimitBackendRedis  = "redis"
	rateLimitBackendMemory = "memory"

	// Atomic INCR + PEXPIRE on first hit so a crash cannot leave a key without TTL.
	rateLimitIncrExpireScript = `
local n = redis.call("INCR", KEYS[1])
if n == 1 then
  redis.call("PEXPIRE", KEYS[1], ARGV[1])
end
return n
`
)

// RateLimitConfig configures per-client fixed-window rate limiting.
type RateLimitConfig struct {
	// Enabled turns rate limiting on or off. When false, the middleware is a no-op.
	Enabled bool
	// Requests is the maximum number of requests allowed per client per Window.
	Requests int
	// Window is the fixed window duration for counting requests.
	Window time.Duration
	// Backend selects the counter store: "redis" (default) or "memory".
	Backend string
}

// RateLimitStore tracks per-client request counts for a fixed window.
type RateLimitStore interface {
	// Allow records one request for clientKey. It returns whether the request is
	// allowed, how many requests remain in the window (0 when denied), and any
	// store error (callers should treat store errors as fail-closed).
	Allow(ctx context.Context, clientKey string, limit int, window time.Duration) (allowed bool, remaining int, err error)
}

// RateLimitConfigFromEnv loads RateLimitConfig from environment variables.
//
//	RATE_LIMIT_ENABLED  — true/false (default true; false/0/off/none disables)
//	RATE_LIMIT_REQUESTS — positive int (default 60)
//	RATE_LIMIT_WINDOW   — Go duration (default 1m)
//	RATE_LIMIT_BACKEND  — redis|memory (default redis)
//
// Invalid values log a warning and fall back to defaults.
func RateLimitConfigFromEnv() RateLimitConfig {
	cfg := RateLimitConfig{
		Enabled:  true,
		Requests: DefaultRateLimitRequests,
		Window:   DefaultRateLimitWindow,
		Backend:  rateLimitBackendRedis,
	}

	if s := strings.TrimSpace(os.Getenv("RATE_LIMIT_ENABLED")); s != "" {
		switch strings.ToLower(s) {
		case "0", "false", "no", "off", "none":
			cfg.Enabled = false
		case "1", "true", "yes", "on":
			cfg.Enabled = true
		default:
			log.Printf("middleware: invalid RATE_LIMIT_ENABLED=%q, using default true", s)
		}
	}

	if s := strings.TrimSpace(os.Getenv("RATE_LIMIT_REQUESTS")); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n <= 0 {
			log.Printf("middleware: invalid RATE_LIMIT_REQUESTS=%q, using default %d", s, DefaultRateLimitRequests)
		} else {
			cfg.Requests = n
		}
	}

	if s := strings.TrimSpace(os.Getenv("RATE_LIMIT_WINDOW")); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil || d <= 0 {
			log.Printf("middleware: invalid RATE_LIMIT_WINDOW=%q, using default %v", s, DefaultRateLimitWindow)
		} else {
			cfg.Window = d
		}
	}

	if s := strings.TrimSpace(os.Getenv("RATE_LIMIT_BACKEND")); s != "" {
		switch strings.ToLower(s) {
		case rateLimitBackendRedis, rateLimitBackendMemory:
			cfg.Backend = strings.ToLower(s)
		default:
			log.Printf("middleware: invalid RATE_LIMIT_BACKEND=%q, using default %s", s, rateLimitBackendRedis)
		}
	}

	return cfg
}

// RateLimiterFromEnv returns Gin middleware for distributed per-client rate
// limiting using RateLimitConfigFromEnv. When the backend is redis, redisClient
// must implement cache.Cache (including Eval). A nil client falls back to the
// in-memory store with a warning.
func RateLimiterFromEnv(redisClient cache.Cache) gin.HandlerFunc {
	cfg := RateLimitConfigFromEnv()
	return ClientRateLimiter(rateLimitStoreFor(cfg, redisClient), cfg)
}

func rateLimitStoreFor(cfg RateLimitConfig, redisClient cache.Cache) RateLimitStore {
	if !cfg.Enabled {
		return nil
	}
	if cfg.Backend == rateLimitBackendMemory {
		return NewMemoryRateLimitStore()
	}
	if redisClient == nil {
		log.Printf("middleware: RATE_LIMIT_BACKEND=redis but cache is nil; using memory store")
		return NewMemoryRateLimitStore()
	}
	return NewRedisRateLimitStore(redisClient)
}

// ClientRateLimiter returns middleware that limits each client (by ClientIP)
// using store and cfg. Probes registered before this middleware are unaffected.
// On store errors the middleware fails closed with HTTP 503.
func ClientRateLimiter(store RateLimitStore, cfg RateLimitConfig) gin.HandlerFunc {
	if !cfg.Enabled || store == nil {
		return func(c *gin.Context) { c.Next() }
	}
	if cfg.Requests <= 0 {
		cfg.Requests = DefaultRateLimitRequests
	}
	if cfg.Window <= 0 {
		cfg.Window = DefaultRateLimitWindow
	}

	return func(c *gin.Context) {
		key := clientRateLimitKey(c)
		allowed, remaining, err := store.Allow(c.Request.Context(), key, cfg.Requests, cfg.Window)
		if err != nil {
			httperr.Abort(c, http.StatusServiceUnavailable, "rate limit unavailable")
			return
		}

		c.Header("X-RateLimit-Limit", strconv.Itoa(cfg.Requests))
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))

		if !allowed {
			retryAfter := int(cfg.Window.Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			c.Header("Retry-After", strconv.Itoa(retryAfter))
			httperr.Abort(c, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}
		c.Next()
	}
}

func clientRateLimitKey(c *gin.Context) string {
	if c == nil {
		return "unknown"
	}
	ip := strings.TrimSpace(c.ClientIP())
	if ip == "" {
		return "unknown"
	}
	return ip
}

// NewRedisRateLimitStore returns a Redis-backed fixed-window RateLimitStore.
// Keys use the prefix v1:ratelimit:{clientKey}.
func NewRedisRateLimitStore(c cache.Cache) RateLimitStore {
	return &redisRateLimitStore{cache: c}
}

type redisRateLimitStore struct {
	cache cache.Cache
}

// Allow implements RateLimitStore using a Lua script that atomically INCRs and
// sets PEXPIRE on the first hit in a window (avoids orphan keys without TTL).
func (s *redisRateLimitStore) Allow(ctx context.Context, clientKey string, limit int, window time.Duration) (bool, int, error) {
	if s == nil || s.cache == nil {
		return false, 0, fmt.Errorf("redis rate limit store: nil cache")
	}
	key := rateLimitKeyPrefix + clientKey
	ms := window.Milliseconds()
	if ms < 1 {
		ms = 1
	}
	n, err := s.cache.Eval(ctx, rateLimitIncrExpireScript, []string{key}, ms).Int64()
	if err != nil {
		return false, 0, fmt.Errorf("rate limit incr/expire %s: %w", key, err)
	}
	if n > int64(limit) {
		return false, 0, nil
	}
	return true, limit - int(n), nil
}

// NewMemoryRateLimitStore returns a process-local fixed-window RateLimitStore.
// It is suitable for tests and single-instance deployments; it is not shared
// across replicas. Expired windows are pruned on Allow to bound memory under
// ClientIP churn.
func NewMemoryRateLimitStore() RateLimitStore {
	return &memoryRateLimitStore{
		windows: make(map[string]memoryWindow),
	}
}

type memoryWindow struct {
	count int
	reset time.Time
}

type memoryRateLimitStore struct {
	mu      sync.Mutex
	windows map[string]memoryWindow
}

// Allow implements RateLimitStore in process memory.
func (s *memoryRateLimitStore) Allow(ctx context.Context, clientKey string, limit int, window time.Duration) (bool, int, error) {
	if err := ctx.Err(); err != nil {
		return false, 0, err
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.pruneExpiredLocked(now)

	w, ok := s.windows[clientKey]
	if !ok || !now.Before(w.reset) {
		w = memoryWindow{count: 0, reset: now.Add(window)}
	}
	w.count++
	s.windows[clientKey] = w

	if w.count > limit {
		return false, 0, nil
	}
	return true, limit - w.count, nil
}

// pruneExpiredLocked removes windows whose reset time has passed. Caller must hold s.mu.
func (s *memoryRateLimitStore) pruneExpiredLocked(now time.Time) {
	for key, w := range s.windows {
		if !now.Before(w.reset) {
			delete(s.windows, key)
		}
	}
}
