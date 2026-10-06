package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCredentialedCORSPreflightAllowsAPIHeadersAndMethods(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Cors())
	router.PATCH("/api/v1/books/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	request := httptest.NewRequest(http.MethodOptions, "/api/v1/books/1", nil)
	request.Header.Set("Origin", "http://localhost:8001")
	request.Header.Set("Access-Control-Request-Method", http.MethodPatch)
	request.Header.Set("Access-Control-Request-Headers", "authorization,x-api-key,content-type")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", response.Code)
	}
	if got := response.Header().Get("Access-Control-Allow-Origin"); got != "http://localhost:8001" {
		t.Errorf("allowed origin = %q", got)
	}
	if got := response.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Errorf("allow credentials = %q", got)
	}
	for _, method := range []string{"PATCH", "DELETE", "PUT"} {
		if got := response.Header().Get("Access-Control-Allow-Methods"); !strings.Contains(got, method) {
			t.Errorf("allowed methods %q omit %s", got, method)
		}
	}
	for _, header := range []string{"authorization", "x-api-key", "content-type"} {
		if got := strings.ToLower(response.Header().Get("Access-Control-Allow-Headers")); !strings.Contains(got, header) {
			t.Errorf("allowed headers %q omit %s", got, header)
		}
	}
}
