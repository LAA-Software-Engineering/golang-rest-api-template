package auth

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashRefreshTokenDeterministic(t *testing.T) {
	a := HashRefreshToken("plain-token")
	b := HashRefreshToken("plain-token")
	assert.Equal(t, a, b)
	assert.Len(t, a, 64)
	assert.NotEqual(t, a, HashRefreshToken("other"))
}

func TestNewOpaqueTokenUnique(t *testing.T) {
	a, err := NewOpaqueToken()
	require.NoError(t, err)
	b, err := NewOpaqueToken()
	require.NoError(t, err)
	assert.NotEqual(t, a, b)
	assert.NotEmpty(t, a)
}

func TestNoopDenylist(t *testing.T) {
	d := NoopDenylist{}
	assert.NoError(t, d.Deny(context.Background(), "jti", time.Now().Add(time.Minute)))
	denied, err := d.IsDenied(context.Background(), "jti")
	assert.NoError(t, err)
	assert.False(t, denied)
}

func TestTokenDenylistEnabledEnv(t *testing.T) {
	t.Setenv("TOKEN_DENYLIST_ENABLED", "false")
	assert.False(t, TokenDenylistEnabled())
	t.Setenv("TOKEN_DENYLIST_ENABLED", "true")
	assert.True(t, TokenDenylistEnabled())
	t.Setenv("TOKEN_DENYLIST_ENABLED", "")
	assert.True(t, TokenDenylistEnabled())
}

func TestNewTokenDenylistFromEnvNoopWhenDisabled(t *testing.T) {
	t.Setenv("TOKEN_DENYLIST_ENABLED", "false")
	dl := NewTokenDenylistFromEnv(nil)
	_, ok := dl.(NoopDenylist)
	assert.True(t, ok)
}

func TestAccessTokenTTLDefault(t *testing.T) {
	t.Setenv("ACCESS_TOKEN_TTL", "")
	assert.Equal(t, defaultAccessTokenTTL, AccessTokenTTL())
	t.Setenv("ACCESS_TOKEN_TTL", "10m")
	assert.Equal(t, 10*time.Minute, AccessTokenTTL())
}

func TestRefreshTokenTTLDefault(t *testing.T) {
	t.Setenv("REFRESH_TOKEN_TTL", "")
	assert.Equal(t, defaultRefreshTokenTTL, RefreshTokenTTL())
	t.Setenv("REFRESH_TOKEN_TTL", "24h")
	assert.Equal(t, 24*time.Hour, RefreshTokenTTL())
}

func TestGenerateTokenIncludesJTIAndIAT(t *testing.T) {
	prev := JWTSigningKey()
	t.Cleanup(func() { _ = SetJWTSigningKey(prev) })
	require.NoError(t, SetJWTSigningKey(bytes.Repeat([]byte("k"), MinJWTSecretKeyBytes)))

	tok, err := GenerateToken("u", 1, RoleUser)
	require.NoError(t, err)

	claims := &Claims{}
	parsed, err := jwt.ParseWithClaims(tok, claims, JWTKeyFunc(JWTSigningKey()))
	require.NoError(t, err)
	require.True(t, parsed.Valid)
	assert.NotEmpty(t, claims.ID)
	assert.NotNil(t, claims.IssuedAt)
	assert.NotNil(t, claims.ExpiresAt)
}
