// Package tracing configures OpenTelemetry distributed tracing for the API.
//
// Tracing is opt-in via OTEL_TRACES_ENABLED. When enabled, HTTP spans include
// the existing X-Request-Id value as the http.request_id attribute so logs and
// traces can be correlated.
package tracing
