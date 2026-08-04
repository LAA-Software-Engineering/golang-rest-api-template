package middleware

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

const (
	mongoAccessLogQueueSize     = 256
	mongoAccessLogInsertTimeout = 5 * time.Second
	mongoAccessLogDropLogEvery  = 100
)

// mongoAccessLogQueue buffers access-log documents and inserts them from a
// single worker so HTTP handlers do not block on MongoDB. If the buffer fills,
// new entries are dropped and a throttled warning is emitted.
type mongoAccessLogQueue struct {
	collection *mongo.Collection
	logger     *zap.Logger
	ch         chan bson.M
	dropped    atomic.Uint64
}

func newMongoAccessLogQueue(collection *mongo.Collection, logger *zap.Logger) *mongoAccessLogQueue {
	if collection == nil {
		return nil
	}
	q := &mongoAccessLogQueue{
		collection: collection,
		logger:     logger,
		ch:         make(chan bson.M, mongoAccessLogQueueSize),
	}
	go q.worker()
	return q
}

func (q *mongoAccessLogQueue) worker() {
	for doc := range q.ch {
		ctx, cancel := context.WithTimeout(context.Background(), mongoAccessLogInsertTimeout)
		_, err := q.collection.InsertOne(ctx, doc)
		cancel()
		if err != nil {
			q.logger.Error("Failed to log to MongoDB", zap.Error(err))
		}
	}
}

func (q *mongoAccessLogQueue) enqueue(doc bson.M) {
	if q == nil {
		return
	}
	select {
	case q.ch <- doc:
	default:
		n := q.dropped.Add(1)
		if n == 1 || n%mongoAccessLogDropLogEvery == 0 {
			q.logger.Warn("mongodb access log queue full; dropping entries",
				zap.Uint64("dropped_total", n))
		}
	}
}

func Logger(logger *zap.Logger, collection *mongo.Collection) gin.HandlerFunc {
	mongoQ := newMongoAccessLogQueue(collection, logger)
	return func(c *gin.Context) {
		// Start timer
		start := time.Now()

		// Process request
		c.Next()

		// End timer
		duration := time.Since(start)
		requestID := GetRequestID(c)

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("duration", duration),
			zap.String("ip", c.ClientIP()),
			zap.String("user-agent", c.Request.UserAgent()),
			zap.String("request_id", requestID),
		}
		logEntry := bson.M{
			"method":     c.Request.Method,
			"path":       c.Request.URL.Path,
			"status":     c.Writer.Status(),
			"duration":   duration,
			"ip":         c.ClientIP(),
			"user-agent": c.Request.UserAgent(),
			"request_id": requestID,
		}
		if sc := trace.SpanFromContext(c.Request.Context()).SpanContext(); sc.IsValid() {
			traceID := sc.TraceID().String()
			spanID := sc.SpanID().String()
			fields = append(fields,
				zap.String("trace_id", traceID),
				zap.String("span_id", spanID),
			)
			logEntry["trace_id"] = traceID
			logEntry["span_id"] = spanID
		}

		logger.Info("Request", fields...)

		mongoQ.enqueue(logEntry)
	}
}
