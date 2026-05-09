package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestRequestContextTimeoutNoOp(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestContextTimeout(0))
	r.GET("/ok", func(c *gin.Context) {
		assert.NoError(t, c.Request.Context().Err())
		c.Status(http.StatusNoContent)
	})
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

func TestRequestContextTimeoutDurationFromEnv(t *testing.T) {
	t.Run("empty uses default", func(t *testing.T) {
		t.Setenv("REQUEST_CONTEXT_TIMEOUT", "")
		assert.Equal(t, defaultRequestContextTimeout, requestContextTimeoutDurationFromEnv())
	})
	t.Run("off disables", func(t *testing.T) {
		t.Setenv("REQUEST_CONTEXT_TIMEOUT", "off")
		assert.Equal(t, time.Duration(0), requestContextTimeoutDurationFromEnv())
	})
	t.Run("parse duration", func(t *testing.T) {
		t.Setenv("REQUEST_CONTEXT_TIMEOUT", "2m")
		assert.Equal(t, 2*time.Minute, requestContextTimeoutDurationFromEnv())
	})
}

func TestRequestContextTimeoutFiresBeforeSlowWork(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(RequestContextTimeout(40 * time.Millisecond))
	r.GET("/slow", func(c *gin.Context) {
		ctx := c.Request.Context()
		select {
		case <-time.After(500 * time.Millisecond):
			c.Status(http.StatusOK)
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				c.AbortWithStatus(http.StatusGatewayTimeout)
				return
			}
			c.AbortWithStatus(http.StatusInternalServerError)
		}
	})
	rec := httptest.NewRecorder()
	start := time.Now()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/slow", nil))
	assert.Less(t, time.Since(start), 200*time.Millisecond, "handler should stop when context times out")
	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
}
