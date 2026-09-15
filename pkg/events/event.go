// Package events defines the optional domain-event publishing contract for the
// API. It is deliberately transport-agnostic: the service layer builds an Event
// and hands it to a Publisher, and a concrete Publisher (e.g. Kafka) decides how
// to serialize and ship it.
//
// Event delivery is best-effort: enabling a real Publisher does not make
// publishing part of write correctness (a publish failure never fails the HTTP
// mutation), but it does make broker health part of write latency. Events may be
// lost or duplicated, so consumers must be idempotent.
package events

import (
	"context"
	"time"
)

// Versioned event types. These strings are part of the public integration
// contract consumers depend on; bump the version suffix on a breaking change to
// the payload rather than mutating an existing type.
const (
	TypeBookCreated = "book.created.v1"
	TypeBookUpdated = "book.updated.v1"
	TypeBookDeleted = "book.deleted.v1"
)

// Aggregate names double as topic suffixes (see the Kafka producer, which
// publishes to "<prefix>.<aggregate>"). Changes to the same aggregate key are
// ordered by keying messages on the aggregate id.
const AggregateBooks = "books"

// Event is what the service layer emits. Payload is a versioned contract struct
// (e.g. BookPayloadV1), never an internal persistence model, so that refactoring
// models does not silently break the published contract.
type Event struct {
	Type       string    // versioned event type, e.g. "book.created.v1"
	Aggregate  string    // aggregate/topic suffix, e.g. "books"
	Key        string    // partition key (aggregate id) for per-aggregate ordering
	OccurredAt time.Time // when the domain fact happened
	Payload    any       // versioned payload; becomes Envelope.Data
}

// Publisher ships domain events to a downstream transport. Implementations must
// be safe for concurrent use by multiple goroutines (Gin handlers run
// concurrently). Publish should honor context cancellation/timeout.
type Publisher interface {
	Publish(ctx context.Context, e Event) error
	Close() error
}

// Envelope is the on-the-wire message format: a stable metadata header wrapping
// the versioned payload in Data. It is the public contract external consumers
// deserialize.
type Envelope struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Source     string    `json:"source"`
	OccurredAt time.Time `json:"occurred_at"`
	Data       any       `json:"data"`
}

// BookPayloadV1 is the v1 data payload for all book.* events. It is a public
// snapshot of a book, decoupled from models.Book.
type BookPayloadV1 struct {
	ID      uint   `json:"id"`
	OwnerID uint   `json:"owner_id"`
	Title   string `json:"title"`
	Author  string `json:"author"`
}
