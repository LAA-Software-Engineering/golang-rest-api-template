package auth

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang-rest-api-template/pkg/cache"

	"github.com/go-redis/redis/v8"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// memStringCache is an in-memory Cache enough for denylist unit tests.
type memStringCache struct {
	mu   sync.Mutex
	data map[string]string
	err  error // if set, Get/Set return this error (except redis.Nil path via missing key)
}

func newMemStringCache() *memStringCache {
	return &memStringCache{data: make(map[string]string)}
}

func (m *memStringCache) Get(ctx context.Context, key string) *redis.StringCmd {
	cmd := redis.NewStringCmd(ctx, "get", key)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		cmd.SetErr(m.err)
		return cmd
	}
	v, ok := m.data[key]
	if !ok {
		cmd.SetErr(redis.Nil)
		return cmd
	}
	cmd.SetVal(v)
	return cmd
}

func (m *memStringCache) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	cmd := redis.NewStatusCmd(ctx, "set", key, value)
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.err != nil {
		cmd.SetErr(m.err)
		return cmd
	}
	if expiration < 0 {
		cmd.SetErr(errors.New("negative expiration"))
		return cmd
	}
	m.data[key] = stringify(value)
	cmd.SetVal("OK")
	return cmd
}

func (m *memStringCache) Incr(ctx context.Context, key string) *redis.IntCmd {
	return redis.NewIntCmd(ctx, "incr", key)
}

func (m *memStringCache) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	return redis.NewCmd(ctx, "eval")
}

func (m *memStringCache) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	return redis.NewIntCmd(ctx, "del")
}

func stringify(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return ""
	}
}

var _ cache.Cache = (*memStringCache)(nil)

func TestRedisDenylistJTI(t *testing.T) {
	c := newMemStringCache()
	d := NewRedisDenylist(c)
	ctx := context.Background()

	denied, err := d.IsDenied(ctx, "abc")
	require.NoError(t, err)
	assert.False(t, denied)

	require.NoError(t, d.Deny(ctx, "abc", time.Now().Add(time.Minute)))
	denied, err = d.IsDenied(ctx, "abc")
	require.NoError(t, err)
	assert.True(t, denied)

	// Expired-until: no write
	require.NoError(t, d.Deny(ctx, "old", time.Now().Add(-time.Second)))
	_, ok := c.data[denylistKeyPrefix+"old"]
	assert.False(t, ok)
}

func TestRedisDenylistUserRevokeBefore(t *testing.T) {
	c := newMemStringCache()
	d := NewRedisDenylist(c)
	ctx := context.Background()

	before := time.Unix(1_700_000_000, 0)
	require.NoError(t, d.DenyUserBefore(ctx, 42, before))

	revoked, err := d.IsUserRevoked(ctx, 42, before.Add(-time.Second))
	require.NoError(t, err)
	assert.True(t, revoked)

	revoked, err = d.IsUserRevoked(ctx, 42, before)
	require.NoError(t, err)
	assert.True(t, revoked) // same-second iat is covered (ts >= iat)

	revoked, err = d.IsUserRevoked(ctx, 42, before.Add(time.Second))
	require.NoError(t, err)
	assert.False(t, revoked)

	revoked, err = d.IsUserRevoked(ctx, 99, before.Add(-time.Second))
	require.NoError(t, err)
	assert.False(t, revoked)
}

func TestRedisDenylistFailOpenOnGetError(t *testing.T) {
	c := newMemStringCache()
	c.err = errors.New("redis down")
	d := NewRedisDenylist(c)
	ctx := context.Background()

	denied, err := d.IsDenied(ctx, "jti")
	require.NoError(t, err)
	assert.False(t, denied)

	revoked, err := d.IsUserRevoked(ctx, 1, time.Now())
	require.NoError(t, err)
	assert.False(t, revoked)
}

func TestNoopDenylistUserMethods(t *testing.T) {
	d := NoopDenylist{}
	assert.NoError(t, d.DenyUserBefore(context.Background(), 1, time.Now()))
	revoked, err := d.IsUserRevoked(context.Background(), 1, time.Now().Add(-time.Hour))
	assert.NoError(t, err)
	assert.False(t, revoked)
}
