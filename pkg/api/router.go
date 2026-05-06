package api

import (
	"os"
	"strconv"
	"strings"
	"time"

	"golang-rest-api-template/pkg/cache"
	"golang-rest-api-template/pkg/middleware"
	"golang-rest-api-template/pkg/repository"

	docs "golang-rest-api-template/docs"

	"github.com/gin-gonic/gin"
	swaggerfiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"go.mongodb.org/mongo-driver/mongo"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"golang.org/x/time/rate"
)

// NewRouter builds the Gin engine with middleware, Swagger, and API routes.
func NewRouter(logger *zap.Logger, mongoCollection *mongo.Collection, db *gorm.DB, redisClient cache.Cache) *gin.Engine {
	bookRepository := NewBookRepository(repository.NewGormBookStore(db), redisClient)
	userRepository := NewUserRepository(repository.NewGormUserStore(db))

	r := gin.Default()
	if err := configureTrustedProxies(r); err != nil {
		panic("api: trusted proxies: " + err.Error())
	}
	registerProbeRoutes(r, db, redisClient, mongoCollection)

	r.Use(middleware.MaxRequestBody(maxRequestBodyBytesFromEnv()))
	r.Use(middleware.RequestID())

	//r.Use(gin.Logger())
	r.Use(middleware.Logger(logger, mongoCollection))
	if gin.Mode() == gin.ReleaseMode {
		r.Use(middleware.Security())
		r.Use(middleware.Xss())
	}
	r.Use(middleware.Cors())
	r.Use(middleware.RateLimiter(rate.Every(1*time.Minute), 60)) // 60 requests per minute

	docs.SwaggerInfo.BasePath = "/api/v1"
	v1 := r.Group("/api/v1")
	{
		v1.GET("/", bookRepository.Healthcheck)
		v1.GET("/books", middleware.APIKeyAuth(), bookRepository.FindBooks)
		v1.POST("/books", middleware.APIKeyAuth(), middleware.JWTAuth(), bookRepository.CreateBook)
		v1.GET("/books/:id", middleware.APIKeyAuth(), bookRepository.FindBook)
		v1.PUT("/books/:id", middleware.APIKeyAuth(), middleware.JWTAuth(), bookRepository.UpdateBook)
		v1.DELETE("/books/:id", middleware.APIKeyAuth(), middleware.JWTAuth(), bookRepository.DeleteBook)

		v1.POST("/login", middleware.APIKeyAuth(), userRepository.LoginHandler)
		v1.POST("/register", middleware.APIKeyAuth(), userRepository.RegisterHandler)
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerfiles.Handler))

	return r
}

// configureTrustedProxies sets which upstreams may influence ClientIP via
// X-Forwarded-For and related headers. If GIN_TRUSTED_PROXIES is unset or
// blank, no proxies are trusted (equivalent to SetTrustedProxies(nil)), so
// ClientIP reflects the direct TCP peer only. Otherwise the value is a
// comma-separated list of IPs or CIDRs accepted by gin.Engine.SetTrustedProxies.
func configureTrustedProxies(engine *gin.Engine) error {
	raw := strings.TrimSpace(os.Getenv("GIN_TRUSTED_PROXIES"))
	if raw == "" {
		return engine.SetTrustedProxies(nil)
	}
	var list []string
	for _, p := range strings.Split(raw, ",") {
		if s := strings.TrimSpace(p); s != "" {
			list = append(list, s)
		}
	}
	return engine.SetTrustedProxies(list)
}

// maxRequestBodyBytesFromEnv returns REQUEST_MAX_BODY_BYTES or the middleware
// default (1 MiB). Invalid or non-positive values panic at process startup.
func maxRequestBodyBytesFromEnv() int64 {
	s := strings.TrimSpace(os.Getenv("REQUEST_MAX_BODY_BYTES"))
	if s == "" {
		return middleware.DefaultMaxRequestBodyBytes
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		panic("api: REQUEST_MAX_BODY_BYTES must be a positive integer (bytes): " + s)
	}
	return n
}
