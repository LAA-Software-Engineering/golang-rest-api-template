package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// MinJWTSecretKeyBytes is the minimum accepted length for the HMAC JWT signing
// secret (raw bytes, not base64-decoded length).
const MinJWTSecretKeyBytes = 32

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
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), 14)
	return string(bytes), err
}

// GenerateToken issues an HS256 JWT with username and user_id claims. userID must
// be non-zero (database primary key of the authenticated user).
func GenerateToken(username string, userID uint) (string, error) {
	if userID == 0 {
		return "", fmt.Errorf("auth.GenerateToken: user id must be non-zero")
	}
	signingMu.RLock()
	key := jwtSigningKey
	signingMu.RUnlock()
	if len(key) < MinJWTSecretKeyBytes {
		return "", fmt.Errorf("auth.GenerateToken: %w", ErrJWTSigningKeyNotConfigured)
	}

	exp := time.Now().Add(5 * time.Minute)
	claims := &Claims{
		Username: username,
		UserID:   userID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
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
