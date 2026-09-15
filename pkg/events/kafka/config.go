// Package kafka provides a Kafka-backed events.Publisher. It knows only about
// Kafka concerns (brokers, topics, timeouts, acknowledgements). It deliberately
// does not know whether Kafka is the selected events backend or not; driver
// selection (EVENTS_DRIVER) lives in cmd/server so this package stays a cleanly
// removable adapter.
package kafka

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultTopicPrefix    = "app"
	defaultPublishTimeout = 2 * time.Second
)

// Config holds Kafka producer settings sourced from KAFKA_* environment
// variables. It carries no notion of the source service name or driver
// selection; those are application-level and passed in by the caller.
type Config struct {
	// Brokers is the list of bootstrap broker addresses (host:port).
	Brokers []string
	// TopicPrefix is prepended to the aggregate name to form the topic:
	// "<TopicPrefix>.<aggregate>" (e.g. "app.books").
	TopicPrefix string
	// PublishTimeout bounds a single synchronous publish. Keep it short: with
	// synchronous best-effort delivery, a slow or unavailable broker adds up to
	// this much latency to every write request.
	PublishTimeout time.Duration
}

// ConfigFromEnv builds a Config from KAFKA_* variables.
//
//	KAFKA_BROKERS         comma-separated host:port list (required)
//	KAFKA_TOPIC_PREFIX    topic prefix (default "app")
//	KAFKA_PUBLISH_TIMEOUT Go duration, e.g. "2s" (default 2s)
func ConfigFromEnv() (Config, error) {
	var cfg Config

	raw := strings.TrimSpace(os.Getenv("KAFKA_BROKERS"))
	if raw == "" {
		return cfg, fmt.Errorf("kafka: KAFKA_BROKERS is required when EVENTS_DRIVER=kafka")
	}
	for _, b := range strings.Split(raw, ",") {
		if b = strings.TrimSpace(b); b != "" {
			cfg.Brokers = append(cfg.Brokers, b)
		}
	}
	if len(cfg.Brokers) == 0 {
		return cfg, fmt.Errorf("kafka: KAFKA_BROKERS contained no valid broker addresses")
	}

	cfg.TopicPrefix = strings.TrimSpace(os.Getenv("KAFKA_TOPIC_PREFIX"))
	if cfg.TopicPrefix == "" {
		cfg.TopicPrefix = defaultTopicPrefix
	}

	cfg.PublishTimeout = defaultPublishTimeout
	if s := strings.TrimSpace(os.Getenv("KAFKA_PUBLISH_TIMEOUT")); s != "" {
		d, err := time.ParseDuration(s)
		if err != nil {
			return cfg, fmt.Errorf("kafka: invalid KAFKA_PUBLISH_TIMEOUT %q: %w", s, err)
		}
		if d <= 0 {
			return cfg, fmt.Errorf("kafka: KAFKA_PUBLISH_TIMEOUT must be > 0, got %q", s)
		}
		cfg.PublishTimeout = d
	}

	return cfg, nil
}
