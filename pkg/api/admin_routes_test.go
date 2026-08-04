package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/middleware"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func testAdminRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	h := &userHandler{}
	r := gin.New()
	admin := r.Group("/admin", middleware.APIKeyAuth(), middleware.JWTAuth(), middleware.RequireRole(auth.RoleAdmin))
	admin.GET("/me", h.AdminMeHandler)
	return r
}

func TestAdminMeRequiresAdminRole(t *testing.T) {
	r := testAdminRouter(t)

	userTok, err := auth.GenerateToken("user", 1, auth.RoleUser)
	assert.NoError(t, err)
	adminTok, err := auth.GenerateToken("admin", 2, auth.RoleAdmin)
	assert.NoError(t, err)

	t.Run("user forbidden", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/me", nil)
		req.Header.Set("X-API-Key", testAPISecretKey)
		req.Header.Set("Authorization", "Bearer "+userTok)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code)
	})

	t.Run("admin ok", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/admin/me", nil)
		req.Header.Set("X-API-Key", testAPISecretKey)
		req.Header.Set("Authorization", "Bearer "+adminTok)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
		var body map[string]any
		assert.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
		data, _ := body["data"].(map[string]any)
		assert.Equal(t, "admin", data["username"])
		assert.Equal(t, auth.RoleAdmin, data["role"])
		assert.EqualValues(t, 2, data["user_id"])
	})
}
