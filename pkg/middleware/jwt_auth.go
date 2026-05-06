package middleware

import (
	"golang-rest-api-template/pkg/auth"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// ContextUserID is the Gin context key for the authenticated user's numeric ID
// (users.id), set by JWTAuth after successful verification.
const ContextUserID = "user_id"

// JWTAuth returns Gin middleware that requires a valid Bearer JWT signed
// with HMAC-SHA256 using the application's JWT secret. Other algorithms are
// rejected before signature verification.
func JWTAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		const BearerSchema = "Bearer "
		header := c.GetHeader("Authorization")
		if header == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization Header"})
			c.Abort()
			return
		}

		if !strings.HasPrefix(header, BearerSchema) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization Header"})
			c.Abort()
			return
		}

		signingKey := auth.JWTSigningKey()
		if len(signingKey) < auth.MinJWTSecretKeyBytes {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "JWT signing key not configured"})
			c.Abort()
			return
		}

		tokenStr := header[len(BearerSchema):]
		claims := &auth.Claims{}

		token, err := jwt.ParseWithClaims(tokenStr, claims, auth.JWTKeyFunc(signingKey))

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		if !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		if claims.UserID == 0 {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
			c.Abort()
			return
		}

		c.Set("username", claims.Username)
		c.Set(ContextUserID, claims.UserID)
		c.Next()
	}
}
