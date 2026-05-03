package main

import (
	"errors"
	"golang-rest-api-template/pkg/api"
	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/database"
	"golang-rest-api-template/pkg/middleware"
	"log"
	"os"
	"syscall"

	"go.uber.org/zap"
)

// @title           Swagger Example API
// @version         1.0
// @description     This is a sample server celler server.
// @termsOfService  http://swagger.io/terms/

// @contact.name   API Support
// @contact.url    http://www.swagger.io/support
// @contact.email  support@swagger.io

// @license.name  Apache 2.0
// @license.url   http://www.apache.org/licenses/LICENSE-2.0.html

// @host      localhost:8001
// @BasePath  /api/v1

// @securityDefinitions.apikey JwtAuth
// @in header
// @name Authorization

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name X-API-Key

// @externalDocs.description  OpenAPI
// @externalDocs.url          https://swagger.io/resources/open-api/

// ignorableZapSyncErr reports known-benign Sync failures (e.g. stderr not flushable).
func ignorableZapSyncErr(err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, syscall.EINVAL) || errors.Is(err, syscall.EBADF) {
		return true
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		if errors.Is(pathErr.Err, syscall.EINVAL) || errors.Is(pathErr.Err, syscall.EBADF) {
			return true
		}
	}
	return false
}

func main() {
	if err := auth.SetJWTSigningKey([]byte(os.Getenv("JWT_SECRET_KEY"))); err != nil {
		log.Fatalf("invalid JWT_SECRET_KEY: %v", err)
	}
	if err := middleware.SetAPISecretKey([]byte(os.Getenv("API_SECRET_KEY"))); err != nil {
		log.Fatalf("invalid API_SECRET_KEY: %v", err)
	}

	redisClient, err := cache.NewRedisClient()
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	db := database.NewDatabase()
	dbWrapper := &database.GormDatabase{DB: db}
	mongo := database.SetupMongoDB()
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("logger: %v", err)
	}
	defer func() {
		if err := logger.Sync(); err != nil && !ignorableZapSyncErr(err) {
			log.Printf("logger sync: %v", err)
		}
	}()

	// Gin mode comes from GIN_MODE (debug | release | test); see gin.EnvGinMode.
	// Gin's init already applied os.Getenv("GIN_MODE"); do not override here.
	// Use GIN_MODE=release in production so Security/XSS middleware run (pkg/api/router.go).

	r := api.NewRouter(logger, mongo, dbWrapper, redisClient)

	if err := r.Run(":8001"); err != nil {
		log.Fatal(err)
	}
}
