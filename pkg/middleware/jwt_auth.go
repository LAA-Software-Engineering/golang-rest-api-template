package middleware

import (
	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/httperr"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ContextUserID is the Gin context key for the authenticated user's numeric ID
// (users.id), set by JWTAuth after successful verification.
const ContextUserID = "user_id"

// ContextRole is the Gin context key for the authenticated user's role claim,
// set by JWTAuth after successful verification.
const ContextRole = "role"

// ContextJTI is the Gin context key for the access token jti claim.
const ContextJTI = "jti"

// ContextAccessExp is the Gin context key for the access token expiry time.Time.
const ContextAccessExp = "access_exp"

// JWTAuth returns Gin middleware that requires a valid Bearer JWT signed
// with HMAC-SHA256 using the application's JWT secret. Other algorithms are
// rejected before signature verification. When denylist is non-nil, revoked
// JTIs are rejected; a nil denylist skips the check (same as NoopDenylist).
func JWTAuth(denylist auth.TokenDenylist) gin.HandlerFunc {
	if denylist == nil {
		denylist = auth.NoopDenylist{}
	}
	return func(c *gin.Context) {
		const BearerSchema = "Bearer "
		header := c.GetHeader("Authorization")
		if header == "" {
			httperr.Abort(c, http.StatusUnauthorized, "Missing Authorization Header")
			return
		}

		if !strings.HasPrefix(header, BearerSchema) {
			httperr.Abort(c, http.StatusUnauthorized, "Invalid Authorization Header")
			return
		}

		signingKey := auth.JWTSigningKey()
		if len(signingKey) < auth.MinJWTSecretKeyBytes {
			httperr.Abort(c, http.StatusServiceUnavailable, "JWT signing key not configured")
			return
		}

		tokenStr := header[len(BearerSchema):]
		claims := &auth.Claims{}

		token, err := jwt.ParseWithClaims(tokenStr, claims, auth.JWTKeyFunc(signingKey))

		if err != nil {
			httperr.Abort(c, http.StatusUnauthorized, "Invalid token")
			return
		}

		if !token.Valid {
			httperr.Abort(c, http.StatusUnauthorized, "Invalid token")
			return
		}

		if claims.UserID == 0 {
			httperr.Abort(c, http.StatusUnauthorized, "Invalid token")
			return
		}

		if !auth.ValidRole(claims.Role) {
			httperr.Abort(c, http.StatusUnauthorized, "Invalid token")
			return
		}

		if claims.ID != "" {
			denied, err := denylist.IsDenied(c.Request.Context(), claims.ID)
			if err != nil {
				httperr.Abort(c, http.StatusUnauthorized, "Invalid token")
				return
			}
			if denied {
				httperr.Abort(c, http.StatusUnauthorized, "Token revoked")
				return
			}
		}

		c.Set("username", claims.Username)
		c.Set(ContextUserID, claims.UserID)
		c.Set(ContextRole, claims.Role)
		c.Set(ContextJTI, claims.ID)
		if claims.ExpiresAt != nil {
			c.Set(ContextAccessExp, claims.ExpiresAt.Time)
		}
		c.Next()
	}
}
