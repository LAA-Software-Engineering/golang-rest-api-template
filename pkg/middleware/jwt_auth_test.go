package middleware

import (
	"bytes"
	"encoding/json"
	"golang-rest-api-template/pkg/auth"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	if err := auth.SetJWTSigningKey(bytes.Repeat([]byte("m"), auth.MinJWTSecretKeyBytes)); err != nil {
		panic(err)
	}
	if err := SetAPISecretKey(bytes.Repeat([]byte("x"), MinAPISecretKeyBytes)); err != nil {
		panic(err)
	}
}

func testRouterJWTAuthOnly(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/protected", JWTAuth(), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestJWTAuthSigningKeyNotConfiguredReturns503(t *testing.T) {
	prev := auth.JWTSigningKey()
	auth.ClearJWTSigningKeyForTesting()
	t.Cleanup(func() {
		if err := auth.SetJWTSigningKey(prev); err != nil {
			t.Fatalf("restore jwt key: %v", err)
		}
	})
	r := testRouterJWTAuthOnly(t)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer irrelevant")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("want 503, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestJWTAuthValidBearer(t *testing.T) {
	token, err := auth.GenerateToken("middleware-user")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	r := testRouterJWTAuthOnly(t)
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestJWTAuthSetsUsernameContext(t *testing.T) {
	const wantUser = "ctx-user"
	token, err := auth.GenerateToken(wantUser)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var got string
	r.GET("/p", JWTAuth(), func(c *gin.Context) {
		v, ok := c.Get("username")
		if !ok {
			t.Error("username not in context")
			c.Status(http.StatusInternalServerError)
			return
		}
		got, _ = v.(string)
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/p", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d body=%q", rec.Code, rec.Body.String())
	}
	if got != wantUser {
		t.Fatalf("username: got %q want %q", got, wantUser)
	}
}

func TestJWTAuthRejectsBadRequests(t *testing.T) {
	validHS256, err := auth.GenerateToken("ok-user")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	key := auth.JWTSigningKey()
	exp := time.Now().Add(time.Hour)
	claims := &auth.Claims{
		Username: "other",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(exp),
		},
	}
	hs512 := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	hs512Str, err := hs512.SignedString(key)
	if err != nil {
		t.Fatalf("HS512 token: %v", err)
	}
	noneTok := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	noneStr, err := noneTok.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("none token: %v", err)
	}

	expiredClaims := &auth.Claims{
		Username: "expired-user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	expiredTok := jwt.NewWithClaims(jwt.SigningMethodHS256, expiredClaims)
	expiredStr, err := expiredTok.SignedString(key)
	if err != nil {
		t.Fatalf("expired HS256 token: %v", err)
	}

	wrongKey := bytes.Repeat([]byte("n"), auth.MinJWTSecretKeyBytes)
	wrongSigClaims := &auth.Claims{
		Username: "wrong-sig-user",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	wrongSigTok := jwt.NewWithClaims(jwt.SigningMethodHS256, wrongSigClaims)
	wrongSigStr, err := wrongSigTok.SignedString(wrongKey)
	if err != nil {
		t.Fatalf("wrong-signature HS256 token: %v", err)
	}

	tests := []struct {
		name       string
		authz      string
		wantStatus int
		wantErrSub string
	}{
		{
			name:       "missing authorization",
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Missing Authorization Header",
		},
		{
			name:       "invalid bearer prefix",
			authz:      "Token " + validHS256,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Invalid Authorization Header",
		},
		{
			name:       "malformed jwt",
			authz:      "Bearer not-a-jwt",
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Invalid token",
		},
		{
			name:       "HS512 algorithm",
			authz:      "Bearer " + hs512Str,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Invalid token",
		},
		{
			name:       "none algorithm",
			authz:      "Bearer " + noneStr,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Invalid token",
		},
		{
			name:       "expired HS256",
			authz:      "Bearer " + expiredStr,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Invalid token",
		},
		{
			name:       "HS256 wrong signing key",
			authz:      "Bearer " + wrongSigStr,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Invalid token",
		},
		{
			name:       "valid HS256",
			authz:      "Bearer " + validHS256,
			wantStatus: http.StatusOK,
		},
	}

	r := testRouterJWTAuthOnly(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if tt.authz != "" {
				req.Header.Set("Authorization", tt.authz)
			}
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status: got %d want %d body=%q", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantErrSub != "" {
				var body map[string]any
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
					t.Fatalf("json: %v raw=%q", err, rec.Body.String())
				}
				errVal, _ := body["error"].(string)
				if !strings.Contains(errVal, tt.wantErrSub) {
					t.Fatalf("error: got %q want substring %q", errVal, tt.wantErrSub)
				}
			}
		})
	}
}

func TestJWTAuthConcurrentValidRequests(t *testing.T) {
	token, err := auth.GenerateToken("concurrent-jwt-user")
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	r := testRouterJWTAuthOnly(t)
	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				errs <- w.Body.String()
			}
		}()
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		if msg != "" {
			t.Fatalf("unexpected failure body=%q", msg)
		}
	}
}
