package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricsConfigFromEnv(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		t.Setenv("METRICS_ENABLED", "")
		t.Setenv("METRICS_PATH", "")
		cfg := MetricsConfigFromEnv()
		assert.True(t, cfg.Enabled)
		assert.Equal(t, DefaultMetricsPath, cfg.Path)
	})
	t.Run("disabled", func(t *testing.T) {
		t.Setenv("METRICS_ENABLED", "off")
		cfg := MetricsConfigFromEnv()
		assert.False(t, cfg.Enabled)
	})
	t.Run("custom path", func(t *testing.T) {
		t.Setenv("METRICS_PATH", "/internal/metrics")
		cfg := MetricsConfigFromEnv()
		assert.Equal(t, "/internal/metrics", cfg.Path)
	})
	t.Run("invalid path falls back", func(t *testing.T) {
		t.Setenv("METRICS_PATH", "metrics")
		cfg := MetricsConfigFromEnv()
		assert.Equal(t, DefaultMetricsPath, cfg.Path)
	})
}

func TestMetricsEndpointAndInstrumentation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewMetrics(MetricsConfig{Enabled: true, Path: DefaultMetricsPath})
	r := gin.New()
	m.Mount(r)
	r.Use(m.Middleware())
	r.GET("/api/v1/books", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	r.GET("/api/v1/books/:id", func(c *gin.Context) {
		c.Status(http.StatusNotFound)
	})

	// Instrumented request
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/books", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	// Unmatched route
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// Scrape
	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "http_requests_total")
	assert.Contains(t, body, "http_request_duration_seconds")
	assert.Contains(t, body, "http_requests_in_flight")
	assert.Contains(t, body, `path="/api/v1/books"`)
	assert.Contains(t, body, `path="unmatched"`)
	// Scrape itself must not appear as an instrumented path series.
	assert.NotContains(t, body, `path="/metrics"`)
}

func TestMetricsDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewMetrics(MetricsConfig{Enabled: false})
	r := gin.New()
	m.Mount(r)
	r.Use(m.Middleware())
	r.GET("/ok", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ok", nil))
	assert.Equal(t, http.StatusNoContent, rec.Code)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestMetricsSkipsSwagger(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewMetrics(MetricsConfig{Enabled: true})
	r := gin.New()
	m.Mount(r)
	r.Use(m.Middleware())
	r.GET("/swagger/*any", func(c *gin.Context) {
		c.String(http.StatusOK, "docs")
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil))
	assert.Equal(t, http.StatusOK, rec.Code)

	rec = httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), `path="/swagger/*any"`)
}

func TestMetricsCounterValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewMetrics(MetricsConfig{Enabled: true})
	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
		assert.Equal(t, http.StatusOK, rec.Code)
	}

	metric := &dto.Metric{}
	err := m.requests.WithLabelValues(http.MethodGet, "/ping", "200").Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(3), metric.GetCounter().GetValue())
}

func TestMetricsConcurrent(t *testing.T) {
	gin.SetMode(gin.TestMode)
	m := NewMetrics(MetricsConfig{Enabled: true})
	r := gin.New()
	r.Use(m.Middleware())
	r.GET("/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	const n = 50
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/ping", nil))
			assert.Equal(t, http.StatusOK, rec.Code)
		}()
	}
	wg.Wait()

	metric := &dto.Metric{}
	err := m.requests.WithLabelValues(http.MethodGet, "/ping", "200").Write(metric)
	require.NoError(t, err)
	assert.Equal(t, float64(n), metric.GetCounter().GetValue())
}

func TestMetricsGathererIncludesGoCollectors(t *testing.T) {
	m := NewMetrics(MetricsConfig{Enabled: true})
	// CounterVec/HistogramVec series appear only after the first observation.
	m.requests.WithLabelValues(http.MethodGet, "/ping", "200").Inc()
	families, err := m.registry.Gather()
	require.NoError(t, err)
	var names []string
	for _, f := range families {
		names = append(names, f.GetName())
	}
	joined := strings.Join(names, ",")
	assert.Contains(t, joined, "go_")
	assert.Contains(t, joined, "process_")
	assert.Contains(t, joined, "http_requests_total")
}

// Ensure Metrics implements expected registration without touching the global registry.
func TestMetricsUsesDedicatedRegistry(t *testing.T) {
	m1 := NewMetrics(MetricsConfig{Enabled: true})
	m2 := NewMetrics(MetricsConfig{Enabled: true})
	require.NotNil(t, m1.registry)
	require.NotNil(t, m2.registry)
	assert.NotSame(t, m1.registry, m2.registry)

	// Global default registerer should not own our collectors.
	assert.NotEqual(t, prometheus.DefaultRegisterer, m1.registry)
}
