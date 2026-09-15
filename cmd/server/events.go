package main

import (
	"fmt"
	"os"
	"strings"

	"golang-rest-api-template/pkg/events"
	eventskafka "golang-rest-api-template/pkg/events/kafka"
)

// buildPublisher selects the domain-event publisher from EVENTS_DRIVER. This
// switch is the single place that knows which backends exist, so it lives at the
// composition root (cmd/server) rather than in pkg/events or the kafka adapter:
// pkg/events stays backend-agnostic and pkg/events/kafka stays cleanly removable.
//
//	EVENTS_DRIVER unset|none -> events.NopPublisher (default; no publishing)
//	EVENTS_DRIVER=kafka       -> Kafka producer from KAFKA_* + SERVICE_NAME
func buildPublisher() (events.Publisher, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EVENTS_DRIVER"))) {
	case "", "none":
		return events.NopPublisher{}, nil
	case "kafka":
		cfg, err := eventskafka.ConfigFromEnv()
		if err != nil {
			return nil, err
		}
		source := strings.TrimSpace(os.Getenv("SERVICE_NAME"))
		if source == "" {
			source = "golang-rest-api-template"
		}
		return eventskafka.New(cfg, source), nil
	default:
		return nil, fmt.Errorf("unsupported EVENTS_DRIVER %q (want \"none\" or \"kafka\")", os.Getenv("EVENTS_DRIVER"))
	}
}
