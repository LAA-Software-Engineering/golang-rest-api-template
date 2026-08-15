package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

const (
	defaultAccessTokenTTL  = 5 * time.Minute
	defaultRefreshTokenTTL = 7 * 24 * time.Hour
	refreshTokenBytes      = 32
	familyIDBytes          = 16
)

// AccessTokenTTL returns the access JWT lifetime from ACCESS_TOKEN_TTL or the default (5m).
func AccessTokenTTL() time.Duration {
	return durationFromEnv("ACCESS_TOKEN_TTL", defaultAccessTokenTTL)
}

// RefreshTokenTTL returns the refresh token lifetime from REFRESH_TOKEN_TTL or the default (7d).
func RefreshTokenTTL() time.Duration {
	return durationFromEnv("REFRESH_TOKEN_TTL", defaultRefreshTokenTTL)
}

func durationFromEnv(key string, def time.Duration) time.Duration {
	s := strings.TrimSpace(os.Getenv(key))
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		log.Printf("auth: invalid %s=%q, using default %v", key, s, def)
		return def
	}
	return d
}

// NewOpaqueToken returns a URL-safe random token string (base64 raw URL encoding).
func NewOpaqueToken() (string, error) {
	b := make([]byte, refreshTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate opaque token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// NewFamilyID returns a random opaque family identifier for refresh token rotation chains.
func NewFamilyID() (string, error) {
	b := make([]byte, familyIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("auth: generate family id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// HashRefreshToken returns the SHA-256 hex digest of the plaintext refresh token.
func HashRefreshToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}
