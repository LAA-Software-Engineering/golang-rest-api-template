package kafka

import (
	"context"
	"encoding/json"
	"fmt"

	"golang-rest-api-template/pkg/events"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
)

// Producer is a synchronous, best-effort events.Publisher backed by Kafka.
//
// It uses a single Writer with RequiredAcks=RequireAll and Async=false, so
// Publish blocks until the brokers acknowledge the write (or the context
// deadline elapses). Messages are keyed by the aggregate id via a Hash balancer,
// so all changes to the same aggregate land on one partition and preserve order.
//
// Delivery remains best-effort at the application level: the service ignores
// Publish errors so a broker problem never fails an already-committed mutation.
// Because kafka-go retries writes and a canceled context leaves the outcome
// ambiguous, events may be lost or duplicated; consumers must be idempotent.
type Producer struct {
	writer *kafkago.Writer
	prefix string
	source string
	cfg    Config
}

// New builds a Kafka Producer. source is the envelope "source" field (the
// SERVICE_NAME of the deploying application), kept separate from Config because
// it is application identity, not a Kafka setting.
func New(cfg Config, source string) *Producer {
	w := &kafkago.Writer{
		Addr:                   kafkago.TCP(cfg.Brokers...),
		Balancer:               &kafkago.Hash{},
		RequiredAcks:           kafkago.RequireAll,
		Async:                  false,
		AllowAutoTopicCreation: true,
	}
	return &Producer{writer: w, prefix: cfg.TopicPrefix, source: source, cfg: cfg}
}

// Publish wraps the event payload in an Envelope and writes it synchronously.
func (p *Producer) Publish(ctx context.Context, e events.Event) error {
	env := events.Envelope{
		ID:         uuid.NewString(),
		Type:       e.Type,
		Source:     p.source,
		OccurredAt: e.OccurredAt,
		Data:       e.Payload,
	}
	value, err := json.Marshal(env)
	if err != nil {
		return fmt.Errorf("kafka: marshal event %s: %w", e.Type, err)
	}

	if p.cfg.PublishTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.cfg.PublishTimeout)
		defer cancel()
	}

	return p.writer.WriteMessages(ctx, kafkago.Message{
		Topic: p.topic(e.Aggregate),
		Key:   []byte(e.Key),
		Value: value,
	})
}

// topic maps an aggregate name to its topic: "<prefix>.<aggregate>".
func (p *Producer) topic(aggregate string) string {
	return p.prefix + "." + aggregate
}

// Close flushes and closes the underlying writer.
func (p *Producer) Close() error { return p.writer.Close() }

// Compile-time assertion that Producer satisfies events.Publisher.
var _ events.Publisher = (*Producer)(nil)
