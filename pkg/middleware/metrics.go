package middleware

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	// DefaultMetricsPath is the HTTP path where Prometheus scrapes metrics.
	DefaultMetricsPath = "/metrics"

	unmatchedRouteLabel = "unmatched"
)

// MetricsConfig configures the Prometheus HTTP metrics middleware and endpoint.
type MetricsConfig struct {
	// Enabled turns metrics collection and the scrape endpoint on or off.
	Enabled bool
	// Path is the scrape path (default /metrics).
	Path string
}

// Metrics holds Prometheus collectors and Gin helpers for HTTP observability.
type Metrics struct {
	cfg      MetricsConfig
	registry *prometheus.Registry
	requests *prometheus.CounterVec
	duration *prometheus.HistogramVec
	inFlight prometheus.Gauge
}

// MetricsConfigFromEnv loads MetricsConfig from environment variables.
//
//	METRICS_ENABLED — true/false (default true; false/0/off/none disables)
//	METRICS_PATH    — scrape path (default /metrics); must start with /
//
// Invalid values log a warning and fall back to defaults.
func MetricsConfigFromEnv() MetricsConfig {
	cfg := MetricsConfig{
		Enabled: true,
		Path:    DefaultMetricsPath,
	}

	if s := strings.TrimSpace(os.Getenv("METRICS_ENABLED")); s != "" {
		switch strings.ToLower(s) {
		case "0", "false", "no", "off", "none":
			cfg.Enabled = false
		case "1", "true", "yes", "on":
			cfg.Enabled = true
		default:
			log.Printf("middleware: invalid METRICS_ENABLED=%q, using default true", s)
		}
	}

	if s := strings.TrimSpace(os.Getenv("METRICS_PATH")); s != "" {
		if !strings.HasPrefix(s, "/") || strings.Contains(s, " ") {
			log.Printf("middleware: invalid METRICS_PATH=%q, using default %s", s, DefaultMetricsPath)
		} else {
			cfg.Path = s
		}
	}

	return cfg
}

// NewMetrics creates HTTP metrics collectors on a dedicated registry.
// When cfg.Enabled is false, Middleware and Mount are no-ops.
func NewMetrics(cfg MetricsConfig) *Metrics {
	if cfg.Path == "" {
		cfg.Path = DefaultMetricsPath
	}

	m := &Metrics{cfg: cfg}
	if !cfg.Enabled {
		return m
	}

	reg := prometheus.NewRegistry()
	requests := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests processed.",
		},
		[]string{"method", "path", "status"},
	)
	duration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)
	inFlight := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed.",
		},
	)

	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		requests,
		duration,
		inFlight,
	)

	m.registry = reg
	m.requests = requests
	m.duration = duration
	m.inFlight = inFlight
	return m
}

// MetricsFromEnv builds Metrics using MetricsConfigFromEnv.
func MetricsFromEnv() *Metrics {
	return NewMetrics(MetricsConfigFromEnv())
}

// Mount registers the Prometheus scrape endpoint on r when metrics are enabled.
// Register it before r.Use middleware (same pattern as probes) so scrapes skip
// rate limiting and auth.
func (m *Metrics) Mount(r *gin.Engine) {
	if m == nil || !m.cfg.Enabled || r == nil {
		return
	}
	r.GET(m.cfg.Path, m.Handler())
}

// Handler serves Prometheus metrics in the text exposition format.
func (m *Metrics) Handler() gin.HandlerFunc {
	if m == nil || !m.cfg.Enabled || m.registry == nil {
		return func(c *gin.Context) {
			c.Status(http.StatusNotFound)
		}
	}
	h := promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// Middleware records request count, latency, and in-flight gauges.
// It skips the scrape path and Swagger UI to avoid noisy or recursive series.
func (m *Metrics) Middleware() gin.HandlerFunc {
	if m == nil || !m.cfg.Enabled {
		return func(c *gin.Context) { c.Next() }
	}
	return func(c *gin.Context) {
		if m.shouldSkip(c) {
			c.Next()
			return
		}

		m.inFlight.Inc()
		defer m.inFlight.Dec()

		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = unmatchedRouteLabel
		}
		status := strconv.Itoa(c.Writer.Status())
		method := c.Request.Method

		m.requests.WithLabelValues(method, route, status).Inc()
		m.duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	}
}

func (m *Metrics) shouldSkip(c *gin.Context) bool {
	path := c.Request.URL.Path
	if path == m.cfg.Path {
		return true
	}
	if strings.HasPrefix(path, "/swagger") {
		return true
	}
	return false
}

// Enabled reports whether metrics collection is active.
func (m *Metrics) Enabled() bool {
	return m != nil && m.cfg.Enabled
}

// Path returns the configured scrape path.
func (m *Metrics) Path() string {
	if m == nil || m.cfg.Path == "" {
		return DefaultMetricsPath
	}
	return m.cfg.Path
}
