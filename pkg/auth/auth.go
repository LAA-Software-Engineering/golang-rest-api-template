package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// MinJWTSecretKeyBytes is the minimum accepted length for the HMAC JWT signing
// secret (raw bytes, not base64-decoded length).
const MinJWTSecretKeyBytes = 32

// defaultBcryptCost is the bcrypt work factor for new password hashes (#128).
// The audit recommended evaluating the 10–12 range; 12 balances UX and strength.
const defaultBcryptCost = 12

// minBcryptCost is the lowest cost we accept from BCRYPT_COST (below this is weak
// for production-style deployments).
const minBcryptCost = 10

var (
	// ErrJWTSigningKeyNotConfigured is returned by GenerateToken when
	// SetJWTSigningKey has not been called successfully.
	ErrJWTSigningKeyNotConfigured = errors.New("jwt signing key not configured")

	// ErrJWTSigningKeyTooShort is returned by SetJWTSigningKey when the secret
	// is shorter than MinJWTSecretKeyBytes.
	ErrJWTSigningKeyTooShort = errors.New("jwt signing key below minimum length")

	signingMu     sync.RWMutex
	jwtSigningKey []byte
)

// Claims carries custom JWT fields plus registered (standard) JWT claims.
type Claims struct {
	Username string `json:"username"`
	// UserID is the authenticated users.id used for authorization (must be non-zero).
	UserID uint `json:"user_id"`
	// Role is the application role (e.g. RoleUser, RoleAdmin) used for RBAC.
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// SetJWTSigningKey configures the symmetric key used to sign and verify JWTs.
// It must be called once during application startup with a secret of at least
// MinJWTSecretKeyBytes; otherwise GenerateToken and JWT middleware cannot work.
func SetJWTSigningKey(secret []byte) error {
	if len(secret) < MinJWTSecretKeyBytes {
		return fmt.Errorf("%w: got %d bytes, need at least %d", ErrJWTSigningKeyTooShort, len(secret), MinJWTSecretKeyBytes)
	}
	k := make([]byte, len(secret))
	copy(k, secret)
	signingMu.Lock()
	jwtSigningKey = k
	signingMu.Unlock()
	return nil
}

// JWTSigningKey returns a copy of the configured JWT HMAC secret, or nil if
// SetJWTSigningKey has not completed successfully.
func JWTSigningKey() []byte {
	signingMu.RLock()
	defer signingMu.RUnlock()
	if len(jwtSigningKey) == 0 {
		return nil
	}
	out := make([]byte, len(jwtSigningKey))
	copy(out, jwtSigningKey)
	return out
}

// ClearJWTSigningKeyForTesting drops the configured signing key. It is only
// intended for tests that cover misconfiguration; callers must restore a valid
// key (for example via t.Cleanup and SetJWTSigningKey).
func ClearJWTSigningKeyForTesting() {
	signingMu.Lock()
	jwtSigningKey = nil
	signingMu.Unlock()
}

// JWTKeyFunc returns a jwt.Keyfunc for ParseWithClaims that only accepts
// tokens signed with HMAC-SHA256 using key. Any other signing method
// (including "none" and asymmetric algorithms) is rejected to prevent
// algorithm confusion attacks.
func JWTKeyFunc(key []byte) jwt.Keyfunc {
	return func(token *jwt.Token) (interface{}, error) {
		if token.Method != jwt.SigningMethodHS256 {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return key, nil
	}
}

func HashPassword(password string) (string, error) {
	cost := bcryptCostForPasswordHashing()
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), cost)
	return string(bytes), err
}

// bcryptCostForPasswordHashing returns the bcrypt cost for HashPassword.
// BCRYPT_COST overrides the default (12); it must parse as an integer in
// [minBcryptCost, bcrypt.MaxCost]. Invalid or out-of-range values log a warning
// and fall back to defaultBcryptCost.
func bcryptCostForPasswordHashing() int {
	s := strings.TrimSpace(os.Getenv("BCRYPT_COST"))
	if s == "" {
		return defaultBcryptCost
	}
	c, err := strconv.Atoi(s)
	if err != nil {
		log.Printf("auth: invalid BCRYPT_COST=%q, using default cost %d", s, defaultBcryptCost)
		return defaultBcryptCost
	}
	if c < bcrypt.MinCost || c > bcrypt.MaxCost {
		log.Printf("auth: invalid BCRYPT_COST=%d (must be %d–%d), using default cost %d", c, bcrypt.MinCost, bcrypt.MaxCost, defaultBcryptCost)
		return defaultBcryptCost
	}
	if c < minBcryptCost {
		log.Printf("auth: BCRYPT_COST=%d is below recommended minimum %d, using %d", c, minBcryptCost, minBcryptCost)
		return minBcryptCost
	}
	return c
}

// GenerateToken issues an HS256 JWT with username, user_id, and role claims.
// userID must be non-zero (database primary key of the authenticated user).
// role must be a known application role (see ValidRole).
// The token includes jti and iat so logout can denylist the access token until exp.
func GenerateToken(username string, userID uint, role string) (string, error) {
	if userID == 0 {
		return "", fmt.Errorf("auth.GenerateToken: user id must be non-zero")
	}
	if !ValidRole(role) {
		return "", fmt.Errorf("auth.GenerateToken: invalid role %q", role)
	}
	signingMu.RLock()
	key := jwtSigningKey
	signingMu.RUnlock()
	if len(key) < MinJWTSecretKeyBytes {
		return "", fmt.Errorf("auth.GenerateToken: %w", ErrJWTSigningKeyNotConfigured)
	}

	jti, err := NewOpaqueToken()
	if err != nil {
		return "", fmt.Errorf("auth.GenerateToken: jti: %w", err)
	}

	now := time.Now()
	exp := now.Add(AccessTokenTTL())
	claims := &Claims{
		Username: username,
		UserID:   userID,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        jti,
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(key)

	if err != nil {
		return "", err
	}

	return tokenString, nil
}

func GenerateRandomKey() string {
	key := make([]byte, 32) // generate a 256 bit key
	_, err := rand.Read(key)
	if err != nil {
		panic("Failed to generate random key: " + err.Error())
	}

	return base64.StdEncoding.EncodeToString(key)
}
