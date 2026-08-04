package tracing

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
)

var (
	mu      sync.RWMutex
	enabled bool
	service string = DefaultServiceName
)

// Provider owns the process-wide TracerProvider. Shutdown flushes pending spans.
type Provider struct {
	tp *sdktrace.TracerProvider
}

// Init configures the global OpenTelemetry tracer provider from cfg.
// When cfg.Enabled is false, Init returns a no-op Provider whose Shutdown
// succeeds immediately and leaves the global no-op tracer in place.
func Init(ctx context.Context, cfg Config) (*Provider, error) {
	setRuntime(cfg.Enabled, cfg.ServiceName)
	if !cfg.Enabled {
		return &Provider{}, nil
	}

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
		resource.WithTelemetrySDK(),
		resource.WithHost(),
	)
	if err != nil {
		return nil, fmt.Errorf("tracing: resource: %w", err)
	}

	exporter, err := newSpanExporter(ctx, cfg.Exporter)
	if err != nil {
		return nil, err
	}

	opts := []sdktrace.TracerProviderOption{
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.AlwaysSample())),
	}
	if exporter != nil {
		opts = append(opts, sdktrace.WithBatcher(exporter))
	}

	tp := sdktrace.NewTracerProvider(opts...)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &Provider{tp: tp}, nil
}

// Shutdown flushes and closes the tracer provider. It is safe to call on a
// no-op Provider.
func (p *Provider) Shutdown(ctx context.Context) error {
	if p == nil || p.tp == nil {
		return nil
	}
	if err := p.tp.Shutdown(ctx); err != nil {
		return fmt.Errorf("tracing: shutdown: %w", err)
	}
	return nil
}

// Enabled reports whether tracing was activated by the last successful Init.
func Enabled() bool {
	mu.RLock()
	defer mu.RUnlock()
	return enabled
}

// ServiceName returns the service name from the last Init (or the default).
func ServiceName() string {
	mu.RLock()
	defer mu.RUnlock()
	return service
}

func setRuntime(on bool, name string) {
	mu.Lock()
	enabled = on
	if name != "" {
		service = name
	} else {
		service = DefaultServiceName
	}
	mu.Unlock()
}

func newSpanExporter(ctx context.Context, kind string) (sdktrace.SpanExporter, error) {
	switch kind {
	case exporterNone:
		return nil, nil
	case exporterStdout:
		exp, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("tracing: stdout exporter: %w", err)
		}
		return exp, nil
	default:
		exp, err := otlptracehttp.New(ctx)
		if err != nil {
			return nil, fmt.Errorf("tracing: otlp http exporter: %w", err)
		}
		return exp, nil
	}
}
