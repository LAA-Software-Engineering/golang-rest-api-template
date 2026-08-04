package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/httperr"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.uber.org/mock/gomock"
)

func TestRateLimitConfigFromEnv_defaults(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "")
	t.Setenv("RATE_LIMIT_REQUESTS", "")
	t.Setenv("RATE_LIMIT_WINDOW", "")
	t.Setenv("RATE_LIMIT_BACKEND", "")

	cfg := RateLimitConfigFromEnv()
	if !cfg.Enabled || cfg.Requests != DefaultRateLimitRequests || cfg.Window != DefaultRateLimitWindow || cfg.Backend != rateLimitBackendRedis {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
}

func TestRateLimitConfigFromEnv_custom(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	t.Setenv("RATE_LIMIT_REQUESTS", "10")
	t.Setenv("RATE_LIMIT_WINDOW", "30s")
	t.Setenv("RATE_LIMIT_BACKEND", "memory")

	cfg := RateLimitConfigFromEnv()
	if !cfg.Enabled || cfg.Requests != 10 || cfg.Window != 30*time.Second || cfg.Backend != rateLimitBackendMemory {
		t.Fatalf("unexpected cfg: %+v", cfg)
	}
}

func TestRateLimitConfigFromEnv_disabled(t *testing.T) {
	for _, v := range []string{"0", "false", "off", "none", "no"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv("RATE_LIMIT_ENABLED", v)
			if RateLimitConfigFromEnv().Enabled {
				t.Fatalf("expected disabled for %q", v)
			}
		})
	}
}

func TestRateLimitConfigFromEnv_invalidFallsBack(t *testing.T) {
	t.Setenv("RATE_LIMIT_REQUESTS", "-1")
	t.Setenv("RATE_LIMIT_WINDOW", "nope")
	t.Setenv("RATE_LIMIT_BACKEND", "postgres")
	t.Setenv("RATE_LIMIT_ENABLED", "maybe")

	cfg := RateLimitConfigFromEnv()
	if cfg.Requests != DefaultRateLimitRequests || cfg.Window != DefaultRateLimitWindow || cfg.Backend != rateLimitBackendRedis || !cfg.Enabled {
		t.Fatalf("expected defaults on invalid input: %+v", cfg)
	}
}

func TestMemoryRateLimitStore_allowsUntilLimit(t *testing.T) {
	store := NewMemoryRateLimitStore()
	ctx := context.Background()
	const limit = 3
	window := time.Minute

	for i := 1; i <= limit; i++ {
		allowed, remaining, err := store.Allow(ctx, "1.2.3.4", limit, window)
		if err != nil {
			t.Fatalf("allow #%d: %v", i, err)
		}
		if !allowed {
			t.Fatalf("allow #%d: want allowed", i)
		}
		if remaining != limit-i {
			t.Fatalf("allow #%d: remaining=%d want %d", i, remaining, limit-i)
		}
	}

	allowed, remaining, err := store.Allow(ctx, "1.2.3.4", limit, window)
	if err != nil {
		t.Fatal(err)
	}
	if allowed || remaining != 0 {
		t.Fatalf("want denied remaining=0, got allowed=%v remaining=%d", allowed, remaining)
	}
}

func TestMemoryRateLimitStore_perClientFairness(t *testing.T) {
	store := NewMemoryRateLimitStore()
	ctx := context.Background()
	const limit = 1

	allowed, _, err := store.Allow(ctx, "a", limit, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("client a first: allowed=%v err=%v", allowed, err)
	}
	allowed, _, err = store.Allow(ctx, "a", limit, time.Minute)
	if err != nil || allowed {
		t.Fatalf("client a second should deny: allowed=%v err=%v", allowed, err)
	}
	allowed, _, err = store.Allow(ctx, "b", limit, time.Minute)
	if err != nil || !allowed {
		t.Fatalf("client b should still be allowed: allowed=%v err=%v", allowed, err)
	}
}

func TestMemoryRateLimitStore_windowReset(t *testing.T) {
	store := NewMemoryRateLimitStore()
	ctx := context.Background()
	window := 20 * time.Millisecond

	if _, _, err := store.Allow(ctx, "ip", 1, window); err != nil {
		t.Fatal(err)
	}
	allowed, _, err := store.Allow(ctx, "ip", 1, window)
	if err != nil || allowed {
		t.Fatalf("want deny in same window: allowed=%v err=%v", allowed, err)
	}

	time.Sleep(window + 5*time.Millisecond)
	allowed, _, err = store.Allow(ctx, "ip", 1, window)
	if err != nil || !allowed {
		t.Fatalf("want allow after window: allowed=%v err=%v", allowed, err)
	}
}

func TestMemoryRateLimitStore_prunesExpiredKeys(t *testing.T) {
	store := NewMemoryRateLimitStore().(*memoryRateLimitStore)
	ctx := context.Background()
	window := 15 * time.Millisecond

	if _, _, err := store.Allow(ctx, "stale-a", 1, window); err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.Allow(ctx, "stale-b", 1, window); err != nil {
		t.Fatal(err)
	}
	store.mu.Lock()
	if got := len(store.windows); got != 2 {
		store.mu.Unlock()
		t.Fatalf("pre-prune size=%d want 2", got)
	}
	store.mu.Unlock()

	time.Sleep(window + 5*time.Millisecond)

	// Touch a different key; Allow should prune expired entries first.
	if _, _, err := store.Allow(ctx, "fresh", 5, time.Minute); err != nil {
		t.Fatal(err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if _, ok := store.windows["stale-a"]; ok {
		t.Fatal("stale-a should have been pruned")
	}
	if _, ok := store.windows["stale-b"]; ok {
		t.Fatal("stale-b should have been pruned")
	}
	if _, ok := store.windows["fresh"]; !ok {
		t.Fatal("fresh should remain")
	}
}

func TestMemoryRateLimitStore_canceledContext(t *testing.T) {
	store := NewMemoryRateLimitStore()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	allowed, _, err := store.Allow(ctx, "ip", 1, time.Minute)
	if allowed || !errors.Is(err, context.Canceled) {
		t.Fatalf("want canceled, got allowed=%v err=%v", allowed, err)
	}
}

func TestMemoryRateLimitStore_concurrent(t *testing.T) {
	store := NewMemoryRateLimitStore()
	const limit = 50
	const goroutines = 100
	var allowedCount atomic.Int64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			ok, _, err := store.Allow(context.Background(), "same", limit, time.Minute)
			if err != nil {
				t.Errorf("allow: %v", err)
				return
			}
			if ok {
				allowedCount.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowedCount.Load(); got != limit {
		t.Fatalf("allowed=%d want %d", got, limit)
	}
}

func TestRedisRateLimitStore_allowAndDeny(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := cache.NewMockCache(ctrl)
	store := NewRedisRateLimitStore(mock)
	ctx := context.Background()
	key := rateLimitKeyPrefix + "9.9.9.9"
	window := time.Minute
	ms := window.Milliseconds()

	cmd1 := redis.NewCmd(ctx)
	cmd1.SetVal(int64(1))
	mock.EXPECT().Eval(ctx, rateLimitIncrExpireScript, []string{key}, ms).Return(cmd1)

	allowed, remaining, err := store.Allow(ctx, "9.9.9.9", 2, window)
	if err != nil || !allowed || remaining != 1 {
		t.Fatalf("first: allowed=%v remaining=%d err=%v", allowed, remaining, err)
	}

	cmd2 := redis.NewCmd(ctx)
	cmd2.SetVal(int64(2))
	mock.EXPECT().Eval(ctx, rateLimitIncrExpireScript, []string{key}, ms).Return(cmd2)
	allowed, remaining, err = store.Allow(ctx, "9.9.9.9", 2, window)
	if err != nil || !allowed || remaining != 0 {
		t.Fatalf("second: allowed=%v remaining=%d err=%v", allowed, remaining, err)
	}

	cmd3 := redis.NewCmd(ctx)
	cmd3.SetVal(int64(3))
	mock.EXPECT().Eval(ctx, rateLimitIncrExpireScript, []string{key}, ms).Return(cmd3)
	allowed, remaining, err = store.Allow(ctx, "9.9.9.9", 2, window)
	if err != nil || allowed || remaining != 0 {
		t.Fatalf("third: allowed=%v remaining=%d err=%v", allowed, remaining, err)
	}
}

func TestRedisRateLimitStore_evalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mock := cache.NewMockCache(ctrl)
	store := NewRedisRateLimitStore(mock)
	ctx := context.Background()
	key := rateLimitKeyPrefix + "ip"
	window := time.Minute

	cmd := redis.NewCmd(ctx)
	cmd.SetErr(errors.New("redis down"))
	mock.EXPECT().Eval(ctx, rateLimitIncrExpireScript, []string{key}, window.Milliseconds()).Return(cmd)

	allowed, _, err := store.Allow(ctx, "ip", 5, window)
	if allowed || err == nil {
		t.Fatalf("want error, got allowed=%v err=%v", allowed, err)
	}
}

func TestRedisRateLimitStore_nilCache(t *testing.T) {
	store := NewRedisRateLimitStore(nil)
	allowed, _, err := store.Allow(context.Background(), "ip", 1, time.Minute)
	if allowed || err == nil {
		t.Fatalf("want error, got allowed=%v err=%v", allowed, err)
	}
}

func TestClientRateLimiter_perClientAndHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	store := NewMemoryRateLimitStore()
	cfg := RateLimitConfig{Enabled: true, Requests: 2, Window: time.Minute, Backend: rateLimitBackendMemory}

	r := gin.New()
	r.Use(ClientRateLimiter(store, cfg))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	hit := func(remote string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	rec := hit("10.0.0.1:1234")
	if rec.Code != http.StatusOK {
		t.Fatalf("1st: want 200 got %d", rec.Code)
	}
	if rec.Header().Get("X-RateLimit-Limit") != "2" || rec.Header().Get("X-RateLimit-Remaining") != "1" {
		t.Fatalf("headers: limit=%q remaining=%q", rec.Header().Get("X-RateLimit-Limit"), rec.Header().Get("X-RateLimit-Remaining"))
	}

	rec = hit("10.0.0.1:1234")
	if rec.Code != http.StatusOK || rec.Header().Get("X-RateLimit-Remaining") != "0" {
		t.Fatalf("2nd: code=%d remaining=%q", rec.Code, rec.Header().Get("X-RateLimit-Remaining"))
	}

	rec = hit("10.0.0.1:1234")
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("3rd: want 429 got %d body=%s", rec.Code, rec.Body.String())
	}
	var body httperr.ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "rate limit exceeded" {
		t.Fatalf("body: %+v", body)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}

	// Different client still allowed.
	rec = hit("10.0.0.2:9999")
	if rec.Code != http.StatusOK {
		t.Fatalf("other client: want 200 got %d", rec.Code)
	}
}

func TestClientRateLimiter_disabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ClientRateLimiter(NewMemoryRateLimitStore(), RateLimitConfig{Enabled: false, Requests: 1, Window: time.Minute}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.RemoteAddr = "1.1.1.1:1"
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("hit %d: want 200 got %d", i, rec.Code)
		}
	}
}

type errStore struct{}

func (errStore) Allow(context.Context, string, int, time.Duration) (bool, int, error) {
	return false, 0, errors.New("boom")
}

func TestClientRateLimiter_storeErrorFailClosed(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(ClientRateLimiter(errStore{}, RateLimitConfig{Enabled: true, Requests: 10, Window: time.Minute}))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503 got %d", rec.Code)
	}
}

func TestClientRateLimiter_unknownClientKey(t *testing.T) {
	if got := clientRateLimitKey(nil); got != "unknown" {
		t.Fatalf("nil context: %q", got)
	}
}

func TestRateLimiterFromEnv_memoryBackend(t *testing.T) {
	t.Setenv("RATE_LIMIT_ENABLED", "true")
	t.Setenv("RATE_LIMIT_REQUESTS", "1")
	t.Setenv("RATE_LIMIT_WINDOW", "1m")
	t.Setenv("RATE_LIMIT_BACKEND", "memory")

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RateLimiterFromEnv(nil))
	r.GET("/x", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "8.8.8.8:53"
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("1st want 200 got %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Fatalf("2nd want 429 got %d", rec.Code)
	}
}
