// Command kafka-consumer is a standalone example of consuming the domain events
// published by the API when EVENTS_DRIVER=kafka. It is intentionally NOT part of
// the API application (hence examples/, not cmd/): the template publishes events
// for whatever downstream services want them; consuming them is the consumer's
// job, not the API's.
//
// It demonstrates the shape of a well-behaved consumer:
//   - reads the "<prefix>.books" topic as part of a consumer group,
//   - decodes the stable Envelope contract,
//   - treats delivery as best-effort (events may be duplicated), so handling is
//     idempotent (here, a seen-set keyed by envelope id).
//
// Usage:
//
//	KAFKA_BROKERS=localhost:9092 \
//	KAFKA_TOPIC_PREFIX=app \
//	KAFKA_CONSUMER_GROUP=example-consumer \
//	go run ./examples/kafka-consumer
package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"golang-rest-api-template/pkg/events"

	kafkago "github.com/segmentio/kafka-go"
)

func main() {
	brokers := splitAndTrim(os.Getenv("KAFKA_BROKERS"))
	if len(brokers) == 0 {
		log.Fatal("KAFKA_BROKERS is required (comma-separated host:port list)")
	}
	prefix := envOr("KAFKA_TOPIC_PREFIX", "app")
	group := envOr("KAFKA_CONSUMER_GROUP", "example-consumer")
	topic := prefix + "." + events.AggregateBooks

	reader := kafkago.NewReader(kafkago.ReaderConfig{
		Brokers: brokers,
		GroupID: group,
		Topic:   topic,
	})
	defer func() {
		if err := reader.Close(); err != nil {
			log.Printf("reader close: %v", err)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Idempotency: best-effort delivery can redeliver events, so skip ids we've
	// already handled. A real consumer would persist this, not keep it in memory.
	handler := &idempotentHandler{seen: make(map[string]struct{})}

	log.Printf("consuming topic %q as group %q from %v", topic, group, brokers)
	for {
		msg, err := reader.ReadMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("shutting down")
				return
			}
			log.Printf("read error: %v", err)
			continue
		}
		handler.handle(msg.Value)
	}
}

type idempotentHandler struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

func (h *idempotentHandler) handle(value []byte) {
	var env events.Envelope
	if err := json.Unmarshal(value, &env); err != nil {
		log.Printf("skipping malformed envelope: %v", err)
		return
	}

	h.mu.Lock()
	if _, dup := h.seen[env.ID]; dup {
		h.mu.Unlock()
		log.Printf("duplicate event %s (%s) — skipping", env.ID, env.Type)
		return
	}
	h.seen[env.ID] = struct{}{}
	h.mu.Unlock()

	log.Printf("event id=%s type=%s source=%s at=%s data=%v",
		env.ID, env.Type, env.Source, env.OccurredAt.Format("15:04:05"), env.Data)
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func splitAndTrim(csv string) []string {
	var out []string
	for _, p := range strings.Split(csv, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
