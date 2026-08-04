package tracing

import (
	"os"
	"strings"
)

const (
	// DefaultServiceName is used when OTEL_SERVICE_NAME is unset.
	DefaultServiceName = "golang-rest-api-template"

	envTracesEnabled  = "OTEL_TRACES_ENABLED"
	envServiceName    = "OTEL_SERVICE_NAME"
	envTracesExporter = "OTEL_TRACES_EXPORTER"

	exporterOTLP   = "otlp"
	exporterStdout = "stdout"
	exporterNone   = "none"
)

// Config holds tracing settings resolved from the process environment.
type Config struct {
	// Enabled turns on the OpenTelemetry tracer provider and HTTP middleware spans.
	Enabled bool
	// ServiceName is the resource service.name attribute.
	ServiceName string
	// Exporter selects how spans are exported: "otlp" (default), "stdout", or "none".
	Exporter string
}

// ConfigFromEnv reads tracing configuration from environment variables.
//
// OTEL_TRACES_ENABLED accepts true/1/yes/on (case-insensitive). When unset or
// false, tracing stays disabled and Init is a no-op.
//
// OTEL_SERVICE_NAME defaults to DefaultServiceName.
//
// OTEL_TRACES_EXPORTER defaults to "otlp" when enabled. Supported values:
// "otlp", "stdout", "none". Unknown values fall back to "otlp".
//
// OTLP endpoint and headers use the standard OpenTelemetry environment
// variables (for example OTEL_EXPORTER_OTLP_ENDPOINT).
func ConfigFromEnv() Config {
	cfg := Config{
		Enabled:     envTruthy(os.Getenv(envTracesEnabled)),
		ServiceName: strings.TrimSpace(os.Getenv(envServiceName)),
		Exporter:    strings.ToLower(strings.TrimSpace(os.Getenv(envTracesExporter))),
	}
	if cfg.ServiceName == "" {
		cfg.ServiceName = DefaultServiceName
	}
	if cfg.Exporter == "" {
		cfg.Exporter = exporterOTLP
	}
	switch cfg.Exporter {
	case exporterOTLP, exporterStdout, exporterNone:
	default:
		cfg.Exporter = exporterOTLP
	}
	return cfg
}

func envTruthy(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
