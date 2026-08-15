package auth

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"golang-rest-api-template/pkg/cache"

	"github.com/go-redis/redis/v8"
)

const (
	denylistKeyPrefix    = "v1:token_denylist:jti:"
	userRevokeBeforePref = "v1:token_denylist:user:"
	userRevokeBeforeSuff = ":revoke_before"
)

// TokenDenylist records revoked access-token JTIs and optional per-user
// revoke_before cutoffs until their natural access-token horizon.
type TokenDenylist interface {
	// Deny marks jti as revoked until until (typically the JWT exp).
	Deny(ctx context.Context, jti string, until time.Time) error
	// IsDenied reports whether jti is currently revoked.
	IsDenied(ctx context.Context, jti string) (bool, error)
	// DenyUserBefore marks all access tokens for userID with iat before before
	// as revoked. The Redis entry TTL is AccessTokenTTL().
	DenyUserBefore(ctx context.Context, userID uint, before time.Time) error
	// IsUserRevoked reports whether an access token issued at iat is covered by
	// a user-level revoke_before cutoff (stored value > iat).
	IsUserRevoked(ctx context.Context, userID uint, iat time.Time) (bool, error)
}

// NoopDenylist never revokes or reports denials (used when denylist is disabled).
type NoopDenylist struct{}

// Deny implements TokenDenylist.
func (NoopDenylist) Deny(context.Context, string, time.Time) error { return nil }

// IsDenied implements TokenDenylist.
func (NoopDenylist) IsDenied(context.Context, string) (bool, error) { return false, nil }

// DenyUserBefore implements TokenDenylist.
func (NoopDenylist) DenyUserBefore(context.Context, uint, time.Time) error { return nil }

// IsUserRevoked implements TokenDenylist.
func (NoopDenylist) IsUserRevoked(context.Context, uint, time.Time) (bool, error) {
	return false, nil
}

// RedisDenylist stores revoked JTIs and per-user revoke_before cutoffs in Redis.
type RedisDenylist struct {
	cache cache.Cache
}

// NewRedisDenylist wraps c for denylist operations. c must be non-nil.
func NewRedisDenylist(c cache.Cache) *RedisDenylist {
	return &RedisDenylist{cache: c}
}

func userRevokeBeforeKey(userID uint) string {
	return userRevokeBeforePref + strconv.FormatUint(uint64(userID), 10) + userRevokeBeforeSuff
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

// DenyUserBefore implements TokenDenylist.
func (d *RedisDenylist) DenyUserBefore(ctx context.Context, userID uint, before time.Time) error {
	if userID == 0 {
		return fmt.Errorf("auth: empty user id")
	}
	if before.IsZero() {
		return fmt.Errorf("auth: empty revoke_before")
	}
	ttl := AccessTokenTTL()
	if ttl <= 0 {
		return nil
	}
	key := userRevokeBeforeKey(userID)
	val := strconv.FormatInt(before.Unix(), 10)
	if err := d.cache.Set(ctx, key, val, ttl).Err(); err != nil {
		return fmt.Errorf("auth: user revoke_before set: %w", err)
	}
	return nil
}

// IsUserRevoked implements TokenDenylist. Redis errors fail open.
func (d *RedisDenylist) IsUserRevoked(ctx context.Context, userID uint, iat time.Time) (bool, error) {
	if userID == 0 || iat.IsZero() {
		return false, nil
	}
	s, err := d.cache.Get(ctx, userRevokeBeforeKey(userID)).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		log.Printf("auth: user revoke_before get failed (fail-open): %v", err)
		return false, nil
	}
	ts, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		log.Printf("auth: invalid revoke_before value %q (fail-open): %v", s, err)
		return false, nil
	}
	return ts > iat.Unix(), nil
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
