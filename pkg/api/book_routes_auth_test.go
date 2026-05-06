package api

import (
	"encoding/json"
	"fmt"
	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/middleware"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
)

// testBookWriteMiddlewareStack mirrors NewRouter book mutation middleware so
// JWT requirements stay aligned with production routes.
func testBookWriteMiddlewareStack(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	ok := func(c *gin.Context) {
		switch c.Request.Method {
		case http.MethodDelete:
			c.Status(http.StatusNoContent)
		default:
			c.Status(http.StatusOK)
		}
	}
	r.PUT("/api/v1/books/:id", middleware.APIKeyAuth(), middleware.JWTAuth(), ok)
	r.PATCH("/api/v1/books/:id", middleware.APIKeyAuth(), middleware.JWTAuth(), ok)
	r.DELETE("/api/v1/books/:id", middleware.APIKeyAuth(), middleware.JWTAuth(), ok)
	return r
}

func TestBookWriteRoutesRequireAPIKeyAndJWT(t *testing.T) {
	const apiKey = testAPISecretKey

	token, err := auth.GenerateToken("book-writer", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	tests := []struct {
		name       string
		method     string
		apiKeyHdr  string
		authzHdr   string
		wantStatus int
		wantErrSub string
	}{
		{
			name:       "put missing api key",
			method:     http.MethodPut,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Unauthorized",
		},
		{
			name:       "put wrong api key",
			method:     http.MethodPut,
			apiKeyHdr:  "not-the-secret",
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Unauthorized",
		},
		{
			name:       "put api key only no jwt",
			method:     http.MethodPut,
			apiKeyHdr:  apiKey,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Missing Authorization Header",
		},
		{
			name:       "put invalid bearer prefix",
			method:     http.MethodPut,
			apiKeyHdr:  apiKey,
			authzHdr:   "Token " + token,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Invalid Authorization Header",
		},
		{
			name:       "put garbage jwt",
			method:     http.MethodPut,
			apiKeyHdr:  apiKey,
			authzHdr:   "Bearer not-a-jwt",
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Invalid token",
		},
		{
			name:       "put valid api key and jwt",
			method:     http.MethodPut,
			apiKeyHdr:  apiKey,
			authzHdr:   "Bearer " + token,
			wantStatus: http.StatusOK,
		},
		{
			name:       "patch missing api key",
			method:     http.MethodPatch,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Unauthorized",
		},
		{
			name:       "patch valid api key and jwt",
			method:     http.MethodPatch,
			apiKeyHdr:  apiKey,
			authzHdr:   "Bearer " + token,
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete api key only no jwt",
			method:     http.MethodDelete,
			apiKeyHdr:  apiKey,
			wantStatus: http.StatusUnauthorized,
			wantErrSub: "Missing Authorization Header",
		},
		{
			name:       "delete valid api key and jwt",
			method:     http.MethodDelete,
			apiKeyHdr:  apiKey,
			authzHdr:   "Bearer " + token,
			wantStatus: http.StatusNoContent,
		},
	}

	r := testBookWriteMiddlewareStack(t)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/v1/books/1", nil)
			if tt.apiKeyHdr != "" {
				req.Header.Set("X-API-Key", tt.apiKeyHdr)
			}
			if tt.authzHdr != "" {
				req.Header.Set("Authorization", tt.authzHdr)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != tt.wantStatus {
				t.Fatalf("status: got %d want %d body=%q", w.Code, tt.wantStatus, w.Body.String())
			}
			if tt.wantErrSub != "" {
				var body map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
					t.Fatalf("json body: %v raw=%q", err, w.Body.String())
				}
				errVal, _ := body["error"].(string)
				if !strings.Contains(errVal, tt.wantErrSub) {
					t.Fatalf("error: got %q want substring %q", errVal, tt.wantErrSub)
				}
			}
		})
	}
}

func TestBookWriteRoutesRejectTamperedJWT(t *testing.T) {
	const apiKey = testAPISecretKey

	token, err := auth.GenerateToken("u1", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	if len(token) < 20 {
		t.Fatalf("unexpected short token")
	}

	valid := "Bearer " + token
	badSig := "Bearer " + token[:len(token)-4]

	r := testBookWriteMiddlewareStack(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/books/1", nil)
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("Authorization", badSig)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("want 401 for tampered token, got %d body=%q", rec.Code, rec.Body.String())
	}

	req2 := httptest.NewRequest(http.MethodPut, "/api/v1/books/1", nil)
	req2.Header.Set("X-API-Key", apiKey)
	req2.Header.Set("Authorization", valid)
	rec2 := httptest.NewRecorder()
	r.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("want 200 for valid token, got %d", rec2.Code)
	}
}

func TestBookWriteRoutesConcurrentAuthorizedRequests(t *testing.T) {
	const apiKey = testAPISecretKey

	token, err := auth.GenerateToken("concurrent-user", 1)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	r := testBookWriteMiddlewareStack(t)
	const workers = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPut, "/api/v1/books/1", nil)
			req.Header.Set("X-API-Key", apiKey)
			req.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				errs <- errTestStatus{got: w.Code, body: w.Body.String()}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
}

type errTestStatus struct {
	got  int
	body string
}

func (e errTestStatus) Error() string {
	return fmt.Sprintf("unexpected HTTP status %d, body=%s", e.got, e.body)
}
