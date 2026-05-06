package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestLivez(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/livez", livez)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/livez", nil)
	r.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestPingPostgresNilDB(t *testing.T) {
	err := pingPostgres(context.Background(), nil)
	assert.Error(t, err)
}

func TestPingRedisNilClient(t *testing.T) {
	err := pingRedis(context.Background(), nil)
	assert.Error(t, err)
}

func TestPingMongoNilCollection(t *testing.T) {
	err := pingMongo(context.Background(), nil)
	assert.Error(t, err)
}
