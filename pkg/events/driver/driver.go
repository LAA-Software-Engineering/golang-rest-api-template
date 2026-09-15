// Package driver selects and constructs a concrete events.Publisher from
// configuration (EVENTS_DRIVER). It is the single place that knows which event
// backends exist, deliberately kept separate from pkg/events so that package
// stays backend-agnostic and from pkg/events/kafka so that adapter stays cleanly
// removable: deleting the kafka adapter only requires editing the switch here.
package driver

import (
	"fmt"
	"os"
	"strings"

	"golang-rest-api-template/pkg/events"
	"golang-rest-api-template/pkg/events/kafka"
)

// NewFromEnv builds the domain-event publisher selected by EVENTS_DRIVER.
//
//	EVENTS_DRIVER unset|none -> events.NopPublisher (default; no publishing)
//	EVENTS_DRIVER=kafka       -> Kafka producer from KAFKA_* + SERVICE_NAME
func NewFromEnv() (events.Publisher, error) {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("EVENTS_DRIVER"))) {
	case "", "none":
		return events.NopPublisher{}, nil
	case "kafka":
		cfg, err := kafka.ConfigFromEnv()
		if err != nil {
			return nil, err
		}
		source := strings.TrimSpace(os.Getenv("SERVICE_NAME"))
		if source == "" {
			source = "golang-rest-api-template"
		}
		return kafka.New(cfg, source), nil
	default:
		return nil, fmt.Errorf("unsupported EVENTS_DRIVER %q (want \"none\" or \"kafka\")", os.Getenv("EVENTS_DRIVER"))
	}
}
