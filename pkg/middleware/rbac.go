package middleware

import (
	"net/http"

	"golang-rest-api-template/pkg/httperr"

	"github.com/gin-gonic/gin"
)

// RequireRole returns Gin middleware that allows the request only when the
// authenticated role (ContextRole, set by JWTAuth) matches one of allowed.
// It must run after JWTAuth. Missing or disallowed roles yield 403 forbidden.
func RequireRole(allowed ...string) gin.HandlerFunc {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, r := range allowed {
		if r == "" {
			continue
		}
		allowedSet[r] = struct{}{}
	}
	return func(c *gin.Context) {
		v, ok := c.Get(ContextRole)
		if !ok {
			httperr.Abort(c, http.StatusForbidden, "forbidden")
			return
		}
		role, _ := v.(string)
		if _, ok := allowedSet[role]; !ok {
			httperr.Abort(c, http.StatusForbidden, "forbidden")
			return
		}
		c.Next()
	}
}
