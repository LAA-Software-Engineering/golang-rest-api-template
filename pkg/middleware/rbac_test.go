package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"golang-rest-api-template/pkg/auth"

	"github.com/gin-gonic/gin"
)

func testRouterRequireRole(t *testing.T, allowed ...string) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin", JWTAuth(), RequireRole(allowed...), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	return r
}

func TestRequireRoleAllowsMatchingRole(t *testing.T) {
	token, err := auth.GenerateToken("admin", 1, auth.RoleAdmin)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	r := testRouterRequireRole(t, auth.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRequireRoleForbidsDisallowedRole(t *testing.T) {
	token, err := auth.GenerateToken("user", 2, auth.RoleUser)
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}
	r := testRouterRequireRole(t, auth.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body=%q", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("json: %v raw=%q", err, rec.Body.String())
	}
	errVal, _ := body["error"].(string)
	if !strings.Contains(errVal, "forbidden") {
		t.Fatalf("error: got %q want substring forbidden", errVal)
	}
}

func TestRequireRoleMissingContextForbids(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/admin", RequireRole(auth.RoleAdmin), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("want 403, got %d body=%q", rec.Code, rec.Body.String())
	}
}

func TestRequireRoleTableDriven(t *testing.T) {
	adminTok, err := auth.GenerateToken("a", 1, auth.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	userTok, err := auth.GenerateToken("u", 2, auth.RoleUser)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		allowed    []string
		token      string
		wantStatus int
	}{
		{name: "admin allowed", allowed: []string{auth.RoleAdmin}, token: adminTok, wantStatus: http.StatusOK},
		{name: "user on admin route", allowed: []string{auth.RoleAdmin}, token: userTok, wantStatus: http.StatusForbidden},
		{name: "either role admin", allowed: []string{auth.RoleUser, auth.RoleAdmin}, token: adminTok, wantStatus: http.StatusOK},
		{name: "either role user", allowed: []string{auth.RoleUser, auth.RoleAdmin}, token: userTok, wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := testRouterRequireRole(t, tt.allowed...)
			req := httptest.NewRequest(http.MethodGet, "/admin", nil)
			req.Header.Set("Authorization", "Bearer "+tt.token)
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, req)
			if rec.Code != tt.wantStatus {
				t.Fatalf("status: got %d want %d body=%q", rec.Code, tt.wantStatus, rec.Body.String())
			}
		})
	}
}
