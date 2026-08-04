package main

import (
	"context"
	"errors"
	"golang-rest-api-template/pkg/api"
	"golang-rest-api-template/pkg/auth"
	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/database"
	"golang-rest-api-template/pkg/middleware"
	"golang-rest-api-template/pkg/tracing"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// @title           golang-rest-api-template
// @version         1.0
// @description     Go/Gin REST API template: books CRUD, register/login, Redis-backed list cache, Postgres via GORM, Mongo access logs, rate limiting, and Swagger.
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

	tracerProvider, err := tracing.Init(context.Background(), tracing.ConfigFromEnv())
	if err != nil {
		log.Fatalf("tracing: %v", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tracerProvider.Shutdown(shutdownCtx); err != nil {
			log.Printf("tracing shutdown: %v", err)
		}
	}()

	redisClient, err := cache.NewRedisClient()
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	db := database.NewDatabase()
	if db == nil {
		log.Fatal("database: could not connect or migrate (see logs above)")
	}
	mongo, err := database.SetupMongoDB()
	if err != nil {
		log.Fatalf("mongo: %v", err)
	}
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

	r := api.NewRouter(logger, mongo, db, redisClient)

	const (
		serverAddr          = ":8001"
		shutdownGracePeriod = 30 * time.Second
	)
	srv := &http.Server{
		Addr:              serverAddr,
		Handler:           r,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	sigCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	case <-sigCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownGracePeriod)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Printf("server shutdown: %v", err)
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("server exit: %v", err)
		}
	}
}
