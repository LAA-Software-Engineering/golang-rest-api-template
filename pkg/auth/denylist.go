package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"golang-rest-api-template/pkg/cache"

	"github.com/go-redis/redis/v8"
)

const denylistKeyPrefix = "v1:token_denylist:jti:"

// TokenDenylist records revoked access-token JTIs until their natural expiry.
type TokenDenylist interface {
	// Deny marks jti as revoked until until (typically the JWT exp).
	Deny(ctx context.Context, jti string, until time.Time) error
	// IsDenied reports whether jti is currently revoked.
	IsDenied(ctx context.Context, jti string) (bool, error)
}

// NoopDenylist never revokes or reports denials (used when denylist is disabled).
type NoopDenylist struct{}

// Deny implements TokenDenylist.
func (NoopDenylist) Deny(context.Context, string, time.Time) error { return nil }

// IsDenied implements TokenDenylist.
func (NoopDenylist) IsDenied(context.Context, string) (bool, error) { return false, nil }

// RedisDenylist stores revoked JTIs in Redis with TTL equal to remaining token life.
type RedisDenylist struct {
	cache cache.Cache
}

// NewRedisDenylist wraps c for jti denylist operations. c must be non-nil.
func NewRedisDenylist(c cache.Cache) *RedisDenylist {
	return &RedisDenylist{cache: c}
}

// Deny implements TokenDenylist.
func (d *RedisDenylist) Deny(ctx context.Context, jti string, until time.Time) error {
	if jti == "" {
		return fmt.Errorf("auth: empty jti")
	}
	ttl := time.Until(until)
	if ttl <= 0 {
		return nil
	}
	key := denylistKeyPrefix + jti
	if err := d.cache.Set(ctx, key, "1", ttl).Err(); err != nil {
		return fmt.Errorf("auth: denylist set: %w", err)
	}
	return nil
}

// IsDenied implements TokenDenylist. Redis errors fail open (token not denied)
// so availability is preserved when Redis is unavailable.
func (d *RedisDenylist) IsDenied(ctx context.Context, jti string) (bool, error) {
	if jti == "" {
		return false, nil
	}
	_, err := d.cache.Get(ctx, denylistKeyPrefix+jti).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		log.Printf("auth: denylist get failed (fail-open): %v", err)
		return false, nil
	}
	return true, nil
}

// TokenDenylistEnabled reports whether TOKEN_DENYLIST_ENABLED is affirmative.
// Default is true when the variable is unset (denylist attempted when Redis is wired).
func TokenDenylistEnabled() bool {
	s := strings.TrimSpace(os.Getenv("TOKEN_DENYLIST_ENABLED"))
	if s == "" {
		return true
	}
	switch strings.ToLower(s) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		log.Printf("auth: invalid TOKEN_DENYLIST_ENABLED=%q, treating as enabled", s)
		return true
	}
}

// NewTokenDenylistFromEnv returns a Redis-backed denylist when enabled and cache
// is non-nil; otherwise a noop denylist. Auth flows never require Redis.
func NewTokenDenylistFromEnv(c cache.Cache) TokenDenylist {
	if !TokenDenylistEnabled() || c == nil {
		return NoopDenylist{}
	}
	return NewRedisDenylist(c)
}
