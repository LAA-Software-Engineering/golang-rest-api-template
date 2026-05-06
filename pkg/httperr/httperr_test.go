package httperr

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestWrite(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	Write(c, http.StatusTeapot, "short and stout")
	assert.Equal(t, http.StatusTeapot, w.Code)
	var body ErrorBody
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "short and stout", body.Error)
}

func TestAbort(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	Abort(c, http.StatusForbidden, "nope")
	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.True(t, c.IsAborted())
	var body ErrorBody
	assert.NoError(t, json.NewDecoder(w.Body).Decode(&body))
	assert.Equal(t, "nope", body.Error)
}
