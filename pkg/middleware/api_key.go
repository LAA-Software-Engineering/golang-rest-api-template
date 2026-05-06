package middleware

import (
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"golang-rest-api-template/pkg/httperr"

	"github.com/gin-gonic/gin"
)

// MinAPISecretKeyBytes is the minimum accepted length for the API shared secret
// (raw bytes of API_SECRET_KEY).
const MinAPISecretKeyBytes = 32

var (
	// ErrAPISecretKeyTooShort is returned by SetAPISecretKey when the secret is
	// shorter than MinAPISecretKeyBytes.
	ErrAPISecretKeyTooShort = errors.New("API secret key below minimum length")

	apiSecretMu sync.RWMutex
	apiSecret   []byte
)

// SetAPISecretKey configures the expected raw value for the X-API-Key header.
// It must be called during application startup with a non-empty secret of at
// least MinAPISecretKeyBytes.
func SetAPISecretKey(secret []byte) error {
	if len(secret) < MinAPISecretKeyBytes {
		return fmt.Errorf("%w: got %d bytes, need at least %d", ErrAPISecretKeyTooShort, len(secret), MinAPISecretKeyBytes)
	}
	k := make([]byte, len(secret))
	copy(k, secret)
	apiSecretMu.Lock()
	apiSecret = k
	apiSecretMu.Unlock()
	return nil
}

func apiSecretCopy() []byte {
	apiSecretMu.RLock()
	defer apiSecretMu.RUnlock()
	if len(apiSecret) == 0 {
		return nil
	}
	out := make([]byte, len(apiSecret))
	copy(out, apiSecret)
	return out
}

// ClearAPISecretKeyForTesting removes the configured API secret. Intended only
// for tests; callers must restore a valid key (for example via t.Cleanup).
func ClearAPISecretKeyForTesting() {
	apiSecretMu.Lock()
	apiSecret = nil
	apiSecretMu.Unlock()
}

// constantTimeAPIKeyEqual reports whether provided equals secret using
// crypto/subtle. Length is folded into the compared buffers so a single
// ConstantTimeCompare runs for every request.
func constantTimeAPIKeyEqual(provided string, secret []byte) bool {
	if len(secret) == 0 {
		return false
	}
	n := len(secret)
	p := []byte(provided)
	candidate := make([]byte, n)
	if len(p) >= n {
		copy(candidate, p[:n])
	} else {
		copy(candidate, p)
	}
	var la, lb [8]byte
	binary.BigEndian.PutUint64(la[:], uint64(len(p)))
	binary.BigEndian.PutUint64(lb[:], uint64(n))
	left := append(candidate, la[:]...)
	right := append(make([]byte, 0, n+8), secret...)
	right = append(right, lb[:]...)
	return subtle.ConstantTimeCompare(left, right) == 1
}

// APIKeyAuth returns Gin middleware that validates the X-API-Key header
// against the configured secret using a constant-time comparison.
func APIKeyAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		secret := apiSecretCopy()
		if len(secret) < MinAPISecretKeyBytes {
			httperr.Abort(c, http.StatusServiceUnavailable, "API secret key not configured")
			return
		}
		apiKey := c.GetHeader("X-API-Key")
		if constantTimeAPIKeyEqual(apiKey, secret) {
			c.Next()
			return
		}
		httperr.Abort(c, http.StatusUnauthorized, "Unauthorized")
	}
}
