package httperr

import "github.com/gin-gonic/gin"

// ErrorBody is the standard JSON error envelope for this API: {"error":"..."}.
type ErrorBody struct {
	Error string `json:"error"`
}

// Write responds with a JSON error body and the given HTTP status. The caller
// should return immediately after (handlers do not abort the Gin chain).
func Write(c *gin.Context, status int, message string) {
	if c == nil {
		return
	}
	c.JSON(status, ErrorBody{Error: message})
}

// Abort responds with a JSON error body, sets the HTTP status, and aborts
// the Gin chain so no later handlers run. Use in middleware.
func Abort(c *gin.Context, status int, message string) {
	if c == nil {
		return
	}
	c.AbortWithStatusJSON(status, ErrorBody{Error: message})
}
