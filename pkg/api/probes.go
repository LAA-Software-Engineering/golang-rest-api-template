package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"golang-rest-api-template/pkg/cache"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"gorm.io/gorm"
)

// registerProbeRoutes mounts Kubernetes-style liveness and readiness endpoints.
// They are registered before request logging, rate limiting, and the /api/v1
// group so orchestrators can probe without API keys or JWTs.
func registerProbeRoutes(r *gin.Engine, db *gorm.DB, redisClient cache.Cache, mongoCol *mongo.Collection) {
	r.GET("/livez", livez)
	r.GET("/readyz", readyzHandler(db, redisClient, mongoCol))
}

func livez(c *gin.Context) {
	c.Status(http.StatusOK)
}

func readyzHandler(db *gorm.DB, redisClient cache.Cache, mongoCol *mongo.Collection) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		if err := pingPostgres(ctx, db); err != nil {
			c.String(http.StatusServiceUnavailable, "postgres: %v", err)
			return
		}
		if err := pingRedis(ctx, redisClient); err != nil {
			c.String(http.StatusServiceUnavailable, "redis: %v", err)
			return
		}
		if err := pingMongo(ctx, mongoCol); err != nil {
			c.String(http.StatusServiceUnavailable, "mongo: %v", err)
			return
		}
		c.Status(http.StatusOK)
	}
}

func pingPostgres(ctx context.Context, db *gorm.DB) error {
	if db == nil {
		return errors.New("database not configured")
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// redisPinger matches *redis.Client without importing the concrete type in Cache.
type redisPinger interface {
	Ping(ctx context.Context) *redis.StatusCmd
}

func pingRedis(ctx context.Context, c cache.Cache) error {
	if c == nil {
		return errors.New("redis not configured")
	}
	p, ok := c.(redisPinger)
	if !ok {
		return errors.New("redis client type does not support ping")
	}
	if err := p.Ping(ctx).Err(); err != nil {
		return err
	}
	return nil
}

func pingMongo(ctx context.Context, col *mongo.Collection) error {
	if col == nil {
		return errors.New("mongo collection not configured")
	}
	client := col.Database().Client()
	if client == nil {
		return fmt.Errorf("mongo client nil")
	}
	return client.Ping(ctx, readpref.Primary())
}
