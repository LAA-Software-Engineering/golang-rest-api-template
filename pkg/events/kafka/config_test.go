package kafka

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigFromEnvRequiresBrokers(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "")
	_, err := ConfigFromEnv()
	require.Error(t, err)
}

func TestConfigFromEnvDefaults(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "localhost:9092")
	t.Setenv("KAFKA_TOPIC_PREFIX", "")
	t.Setenv("KAFKA_PUBLISH_TIMEOUT", "")

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, []string{"localhost:9092"}, cfg.Brokers)
	assert.Equal(t, defaultTopicPrefix, cfg.TopicPrefix)
	assert.Equal(t, defaultPublishTimeout, cfg.PublishTimeout)
}

func TestConfigFromEnvParsesValues(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "a:9092, b:9092 ,c:9092")
	t.Setenv("KAFKA_TOPIC_PREFIX", "myapp")
	t.Setenv("KAFKA_PUBLISH_TIMEOUT", "500ms")

	cfg, err := ConfigFromEnv()
	require.NoError(t, err)
	assert.Equal(t, []string{"a:9092", "b:9092", "c:9092"}, cfg.Brokers)
	assert.Equal(t, "myapp", cfg.TopicPrefix)
	assert.Equal(t, 500*time.Millisecond, cfg.PublishTimeout)
}

func TestConfigFromEnvRejectsBadTimeout(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "localhost:9092")
	t.Setenv("KAFKA_PUBLISH_TIMEOUT", "nope")
	_, err := ConfigFromEnv()
	require.Error(t, err)
}

func TestConfigFromEnvRejectsNonPositiveTimeout(t *testing.T) {
	t.Setenv("KAFKA_BROKERS", "localhost:9092")
	t.Setenv("KAFKA_PUBLISH_TIMEOUT", "0s")
	_, err := ConfigFromEnv()
	require.Error(t, err)
}
