// Command kafka-consumer is a standalone example of consuming the domain events
// published by the API when EVENTS_DRIVER=kafka. It is intentionally NOT part of
// the API application (hence examples/, not cmd/): the template publishes events
// for whatever downstream services want them; consuming them is the consumer's
// job, not the API's.
//
// It demonstrates the shape of a well-behaved consumer:
//   - reads the "<prefix>.books" topic as part of a consumer group,
//   - decodes the stable Envelope contract,
//   - commits the offset only AFTER handling succeeds (FetchMessage +
//     CommitMessages, not ReadMessage which commits on fetch), so a crash between
//     fetch and processing redelivers the message rather than losing it —
//     at-least-once, not at-most-once,
//   - treats redelivery as expected (events may be duplicated), so handling is
//     idempotent (here, a seen-set keyed by envelope id, marked only after the
//     work succeeds).
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
		// FetchMessage does NOT commit the offset (unlike ReadMessage, which
		// commits on fetch and would drop this message if we crashed before
		// handling it).
		msg, err := reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				log.Println("shutting down")
				return
			}
			log.Printf("fetch error: %v", err)
			continue
		}

		if err := handler.handle(msg.Value); err != nil {
			// Handling failed: do NOT commit, so the message is redelivered and
			// retried. The idempotent seen-set absorbs the resulting duplicate.
			log.Printf("handle error (will retry on redelivery): %v", err)
			continue
		}

		// Commit only after successful handling → at-least-once.
		if err := reader.CommitMessages(ctx, msg); err != nil {
			log.Printf("commit error: %v", err)
		}
	}
}

type idempotentHandler struct {
	mu   sync.Mutex
	seen map[string]struct{}
}

// handle processes one message. A nil return means "safe to commit"; a non-nil
// return means the message should be redelivered and retried.
func (h *idempotentHandler) handle(value []byte) error {
	var env events.Envelope
	if err := json.Unmarshal(value, &env); err != nil {
		// A malformed message will never become valid on redelivery, so retrying
		// forever would wedge the partition. Log and commit past it.
		log.Printf("skipping malformed envelope: %v", err)
		return nil
	}

	h.mu.Lock()
	_, dup := h.seen[env.ID]
	h.mu.Unlock()
	if dup {
		// Already processed (redelivery). The effect was applied before; safe to
		// commit again.
		log.Printf("duplicate event %s (%s) — already processed, skipping", env.ID, env.Type)
		return nil
	}

	// --- do the real work here; return an error to trigger redelivery ---
	log.Printf("event id=%s type=%s source=%s at=%s data=%v",
		env.ID, env.Type, env.Source, env.OccurredAt.Format("15:04:05"), env.Data)

	// Mark seen only after the work succeeded, so a failure above leaves the
	// event un-seen and re-processable on redelivery.
	h.mu.Lock()
	h.seen[env.ID] = struct{}{}
	h.mu.Unlock()
	return nil
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
