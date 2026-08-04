package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

func installTestTracer(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		_ = tp.Shutdown(context.Background())
		otel.SetTracerProvider(prev)
	})
	return sr
}

func TestTracingAttachesRequestIDAttribute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sr := installTestTracer(t)

	r := gin.New()
	r.Use(RequestID())
	r.Use(Tracing())
	r.GET("/books/:id", func(c *gin.Context) {
		sc := trace.SpanFromContext(c.Request.Context()).SpanContext()
		assert.True(t, sc.IsValid())
		c.Status(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/books/42", nil)
	req.Host = "api.example.test"
	req.Header.Set(RequestIDHeader, "req-trace-1")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	spans := sr.Ended()
	require.Len(t, spans, 1)

	span := spans[0]
	assert.Equal(t, "/books/:id", span.Name())
	assert.Equal(t, trace.SpanKindServer, span.SpanKind())

	attrs := span.Attributes()
	assert.True(t, hasAttr(attrs, attribute.String(requestIDSpanAttrKey, "req-trace-1")))
	assert.True(t, hasAttr(attrs, attribute.String("http.request.method", "GET")))
	assert.True(t, hasAttr(attrs, attribute.String("server.address", "api.example.test")))
	assert.True(t, hasAttr(attrs, attribute.String("url.path", "/books/42")))
	assert.True(t, hasAttr(attrs, attribute.String("http.route", "/books/:id")))
	assert.True(t, hasAttr(attrs, attribute.Int("http.response.status_code", 200)))
}

func TestTracingUnmatchedRouteUsesLowCardinalityName(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sr := installTestTracer(t)

	r := gin.New()
	r.Use(RequestID())
	r.Use(Tracing())
	r.NoRoute(func(c *gin.Context) {
		c.Status(http.StatusNotFound)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/books/42", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "GET unmatched", spans[0].Name())
	attrs := spans[0].Attributes()
	assert.True(t, hasAttr(attrs, attribute.String("url.path", "/api/v1/books/42")))
	assert.False(t, hasAttrKey(attrs, "http.route"))
}

func TestTracingMarksServerErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sr := installTestTracer(t)

	r := gin.New()
	r.Use(RequestID())
	r.Use(Tracing())
	r.GET("/boom", func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
	})

	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	spans := sr.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, codes.Error, spans[0].Status().Code)
}

func TestTracingNoopProviderStillServes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(noop.NewTracerProvider())
	t.Cleanup(func() { otel.SetTracerProvider(prev) })

	r := gin.New()
	r.Use(RequestID())
	r.Use(Tracing())
	r.GET("/ok", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "ok", w.Body.String())
}

func hasAttr(attrs []attribute.KeyValue, want attribute.KeyValue) bool {
	for _, a := range attrs {
		if a.Key == want.Key && a.Value.Type() == want.Value.Type() && a.Value.String() == want.Value.String() {
			return true
		}
	}
	return false
}

func hasAttrKey(attrs []attribute.KeyValue, key attribute.Key) bool {
	for _, a := range attrs {
		if a.Key == key {
			return true
		}
	}
	return false
}
