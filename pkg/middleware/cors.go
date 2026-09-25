package middleware

import (
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

func Cors() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowOrigins = []string{
		"http://127.0.0.1",
		"http://127.0.0.1:8001",
		"http://localhost",
		"http://localhost:8001"}
	config.AddAllowHeaders("Authorization", "X-API-Key", "X-Request-Id")
	config.AllowCredentials = true
	return cors.New(config)
}
