package tracing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func TestInitDisabledIsNoop(t *testing.T) {
	prevTP := otel.GetTracerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		setRuntime(false, DefaultServiceName)
	})

	p, err := Init(context.Background(), Config{Enabled: false, ServiceName: "disabled-svc"})
	require.NoError(t, err)
	require.NotNil(t, p)
	assert.False(t, Enabled())
	assert.Equal(t, "disabled-svc", ServiceName())
	assert.NoError(t, p.Shutdown(context.Background()))
}

func TestInitNoneExporter(t *testing.T) {
	prevTP := otel.GetTracerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		setRuntime(false, DefaultServiceName)
	})

	p, err := Init(context.Background(), Config{
		Enabled:     true,
		ServiceName: "test-svc",
		Exporter:    exporterNone,
	})
	require.NoError(t, err)
	require.NotNil(t, p)
	t.Cleanup(func() {
		_ = p.Shutdown(context.Background())
	})

	assert.True(t, Enabled())
	assert.Equal(t, "test-svc", ServiceName())
	assert.NotNil(t, p.tp)
}

func TestInitStdoutExporter(t *testing.T) {
	prevTP := otel.GetTracerProvider()
	t.Cleanup(func() {
		otel.SetTracerProvider(prevTP)
		setRuntime(false, DefaultServiceName)
	})

	p, err := Init(context.Background(), Config{
		Enabled:     true,
		ServiceName: "stdout-svc",
		Exporter:    exporterStdout,
	})
	require.NoError(t, err)
	require.NotNil(t, p)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	assert.NoError(t, p.Shutdown(ctx))
}

func TestProviderShutdownNilSafe(t *testing.T) {
	var p *Provider
	assert.NoError(t, p.Shutdown(context.Background()))
	assert.NoError(t, (&Provider{}).Shutdown(context.Background()))
}
