package tracing

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv(envTracesEnabled, "")
	t.Setenv(envServiceName, "")
	t.Setenv(envTracesExporter, "")

	cfg := ConfigFromEnv()
	assert.False(t, cfg.Enabled)
	assert.Equal(t, DefaultServiceName, cfg.ServiceName)
	assert.Equal(t, exporterOTLP, cfg.Exporter)
}

func TestConfigFromEnvEnabledVariants(t *testing.T) {
	t.Setenv(envServiceName, "api-test")
	t.Setenv(envTracesExporter, "")

	for _, v := range []string{"true", "TRUE", "1", "yes", "on"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv(envTracesEnabled, v)
			cfg := ConfigFromEnv()
			assert.True(t, cfg.Enabled)
			assert.Equal(t, "api-test", cfg.ServiceName)
		})
	}
}

func TestConfigFromEnvDisabledVariants(t *testing.T) {
	for _, v := range []string{"", "0", "false", "no", "off", "maybe"} {
		t.Run(v, func(t *testing.T) {
			t.Setenv(envTracesEnabled, v)
			assert.False(t, ConfigFromEnv().Enabled)
		})
	}
}

func TestConfigFromEnvExporter(t *testing.T) {
	t.Setenv(envTracesEnabled, "true")

	t.Run("stdout", func(t *testing.T) {
		t.Setenv(envTracesExporter, "stdout")
		assert.Equal(t, exporterStdout, ConfigFromEnv().Exporter)
	})
	t.Run("none", func(t *testing.T) {
		t.Setenv(envTracesExporter, "NONE")
		assert.Equal(t, exporterNone, ConfigFromEnv().Exporter)
	})
	t.Run("unknown falls back to otlp", func(t *testing.T) {
		t.Setenv(envTracesExporter, "jaeger")
		assert.Equal(t, exporterOTLP, ConfigFromEnv().Exporter)
	})
}
