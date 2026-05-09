package auth

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func init() {
	// Same-length key as production minimum; runs even when TestMain is not linked
	// (e.g. `go test auth_test.go` alone).
	if err := SetJWTSigningKey(bytes.Repeat([]byte("k"), MinJWTSecretKeyBytes)); err != nil {
		panic(err)
	}
}

func TestHashPassword(t *testing.T) {
	password := "1234"
	hashedPassword, err := HashPassword(password)
	assert.Nil(t, err)
	assert.NotEmpty(t, hashedPassword)
}

func TestHashPasswordUsesDefaultBcryptCost(t *testing.T) {
	t.Setenv("BCRYPT_COST", "")
	hash, err := HashPassword("secret")
	if !assert.NoError(t, err) {
		return
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, defaultBcryptCost, cost)
}

func TestHashPasswordUsesBCRYPT_COST(t *testing.T) {
	t.Setenv("BCRYPT_COST", "10")
	hash, err := HashPassword("secret")
	if !assert.NoError(t, err) {
		return
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, 10, cost)
}

func TestHashPasswordClampsLowBCRYPT_COSTToMin(t *testing.T) {
	t.Setenv("BCRYPT_COST", "8")
	hash, err := HashPassword("secret")
	if !assert.NoError(t, err) {
		return
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, minBcryptCost, cost)
}

func TestHashPasswordInvalidBcryptCostEnvFallsBackToDefault(t *testing.T) {
	t.Setenv("BCRYPT_COST", "not-a-number")
	hash, err := HashPassword("secret")
	if !assert.NoError(t, err) {
		return
	}
	cost, err := bcrypt.Cost([]byte(hash))
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, defaultBcryptCost, cost)
}

func TestGenerateToken(t *testing.T) {
	user := "chud"
	token, err := GenerateToken(user, 1)
	assert.Nil(t, err)
	assert.NotEmpty(t, token)
}

func TestGenerateTokenRoundTripUsernameClaim(t *testing.T) {
	const wantUser = "alice"
	const wantID uint = 42
	tokenStr, err := GenerateToken(wantUser, wantID)
	if !assert.NoError(t, err) {
		return
	}

	parsed := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, parsed, JWTKeyFunc(JWTSigningKey()))
	if !assert.NoError(t, err) {
		return
	}
	if !assert.True(t, token.Valid) {
		return
	}
	assert.Equal(t, wantUser, parsed.Username)
	assert.Equal(t, wantID, parsed.UserID)
}

func TestGenerateRandomKey(t *testing.T) {
	randomKey := GenerateRandomKey()
	assert.NotEmpty(t, randomKey)
	assert.Len(t, randomKey, 44)
}

func TestSetJWTSigningKeyRejectsShortSecret(t *testing.T) {
	before := JWTSigningKey()
	err := SetJWTSigningKey(bytes.Repeat([]byte("s"), MinJWTSecretKeyBytes-1))
	assert.Error(t, err)
	assert.ErrorIs(t, err, ErrJWTSigningKeyTooShort)
	assert.Equal(t, before, JWTSigningKey())
}

func TestSetJWTSigningKeyAcceptsMinimumLength(t *testing.T) {
	secret := bytes.Repeat([]byte("z"), MinJWTSecretKeyBytes)
	err := SetJWTSigningKey(secret)
	if !assert.NoError(t, err) {
		return
	}
	assert.Equal(t, secret, JWTSigningKey())
	_, err = GenerateToken("user-after-rotate", 1)
	assert.NoError(t, err)
	// Restore default for other tests in the package.
	assert.NoError(t, SetJWTSigningKey(bytes.Repeat([]byte("k"), MinJWTSecretKeyBytes)))
}

func TestGenerateTokenErrorWhenSigningKeyUnset(t *testing.T) {
	prev := JWTSigningKey()
	ClearJWTSigningKeyForTesting()
	t.Cleanup(func() {
		assert.NoError(t, SetJWTSigningKey(prev))
	})
	_, err := GenerateToken("any", 1)
	assert.Error(t, err)
	assert.True(t, errors.Is(err, ErrJWTSigningKeyNotConfigured))
}

func TestGenerateTokenRejectsZeroUserID(t *testing.T) {
	_, err := GenerateToken("u", 0)
	assert.Error(t, err)
}

func TestJWTKeyFuncRejectsNonHS256Algorithms(t *testing.T) {
	key := []byte("jwt-keyfunc-test-secret-32bytes!!")
	exp := time.Now().Add(time.Hour)
	baseClaims := Claims{
		Username: "attacker",
		UserID:   1,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}

	t.Run("HS512", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodHS512, &baseClaims)
		s, err := tok.SignedString(key)
		if !assert.NoError(t, err) {
			return
		}
		parsed := &Claims{}
		_, err = jwt.ParseWithClaims(s, parsed, JWTKeyFunc(key))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected signing method")
	})

	t.Run("none", func(t *testing.T) {
		tok := jwt.NewWithClaims(jwt.SigningMethodNone, &baseClaims)
		s, err := tok.SignedString(jwt.UnsafeAllowNoneSignatureType)
		if !assert.NoError(t, err) {
			return
		}
		parsed := &Claims{}
		_, err = jwt.ParseWithClaims(s, parsed, JWTKeyFunc(key))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected signing method")
	})
}

func TestJWTKeyFuncAcceptsHS256(t *testing.T) {
	key := []byte("jwt-keyfunc-hs256-accept-secret!")
	claims := &Claims{
		Username: "legit",
		UserID:   9,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	s, err := tok.SignedString(key)
	if !assert.NoError(t, err) {
		return
	}
	parsed := &Claims{}
	token, err := jwt.ParseWithClaims(s, parsed, JWTKeyFunc(key))
	if !assert.NoError(t, err) {
		return
	}
	assert.True(t, token.Valid)
	assert.Equal(t, "legit", parsed.Username)
}
