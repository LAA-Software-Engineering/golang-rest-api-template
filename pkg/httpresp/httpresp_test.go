package httpresp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestOKStringEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	OK(c, "ok")
	assert.Equal(t, http.StatusOK, w.Code)
	var out struct {
		Data string `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, "ok", out.Data)
}

func TestCreatedStructEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	type item struct {
		ID int `json:"id"`
	}
	Created(c, item{ID: 42})
	assert.Equal(t, http.StatusCreated, w.Code)
	var out struct {
		Data struct {
			ID int `json:"id"`
		} `json:"data"`
	}
	assert.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, 42, out.Data.ID)
}
