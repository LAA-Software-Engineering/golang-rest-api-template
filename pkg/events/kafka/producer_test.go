package kafka

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"golang-rest-api-template/pkg/events"

	"github.com/google/uuid"
	kafkago "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeWriter is a messageWriter test double that records what it was asked to
// write and can be made to fail.
type fakeWriter struct {
	msgs        []kafkago.Message
	ctxHadDL    bool
	err         error
	closed      bool
	closeErr    error
	writeCalled int
}

func (f *fakeWriter) WriteMessages(ctx context.Context, msgs ...kafkago.Message) error {
	f.writeCalled++
	_, f.ctxHadDL = ctx.Deadline()
	f.msgs = append(f.msgs, msgs...)
	return f.err
}

func (f *fakeWriter) Close() error {
	f.closed = true
	return f.closeErr
}

// envelopeForTest decodes the wire format with a typed Data so payload fields
// can be asserted directly.
type envelopeForTest struct {
	ID         string               `json:"id"`
	Type       string               `json:"type"`
	Source     string               `json:"source"`
	OccurredAt time.Time            `json:"occurred_at"`
	Data       events.BookPayloadV1 `json:"data"`
}

func newTestProducer(w messageWriter, cfg Config, source string) *Producer {
	return &Producer{writer: w, prefix: cfg.TopicPrefix, source: source, cfg: cfg}
}

func sampleEvent() events.Event {
	return events.Event{
		Type:       events.TypeBookCreated,
		Aggregate:  events.AggregateBooks,
		Key:        "42",
		OccurredAt: time.Date(2026, 9, 15, 22, 30, 0, 0, time.UTC),
		Payload:    events.BookPayloadV1{ID: 42, OwnerID: 7, Title: "Dune", Author: "Herbert"},
	}
}

func TestPublishWritesEnvelopeMessage(t *testing.T) {
	w := &fakeWriter{}
	p := newTestProducer(w, Config{TopicPrefix: "myapp", PublishTimeout: time.Second}, "my-books-api")

	err := p.Publish(context.Background(), sampleEvent())
	require.NoError(t, err)

	require.Len(t, w.msgs, 1)
	msg := w.msgs[0]
	assert.Equal(t, "myapp.books", msg.Topic, "topic is <prefix>.<aggregate>")
	assert.Equal(t, []byte("42"), msg.Key, "message keyed by aggregate id for ordering")

	var env envelopeForTest
	require.NoError(t, json.Unmarshal(msg.Value, &env))
	assert.Equal(t, events.TypeBookCreated, env.Type)
	assert.Equal(t, "my-books-api", env.Source)
	assert.True(t, env.OccurredAt.Equal(sampleEvent().OccurredAt))
	assert.Equal(t, events.BookPayloadV1{ID: 42, OwnerID: 7, Title: "Dune", Author: "Herbert"}, env.Data)

	_, uErr := uuid.Parse(env.ID)
	assert.NoError(t, uErr, "envelope id is a UUID")
}

func TestPublishAppliesTimeoutDeadline(t *testing.T) {
	w := &fakeWriter{}
	p := newTestProducer(w, Config{TopicPrefix: "app", PublishTimeout: 50 * time.Millisecond}, "svc")

	require.NoError(t, p.Publish(context.Background(), sampleEvent()))
	assert.True(t, w.ctxHadDL, "a positive PublishTimeout must bound the write context")
}

func TestPublishNoDeadlineWhenTimeoutZero(t *testing.T) {
	w := &fakeWriter{}
	p := newTestProducer(w, Config{TopicPrefix: "app", PublishTimeout: 0}, "svc")

	require.NoError(t, p.Publish(context.Background(), sampleEvent()))
	assert.False(t, w.ctxHadDL, "with no timeout the parent context is passed through unchanged")
}

func TestPublishPropagatesWriteError(t *testing.T) {
	wantErr := errors.New("broker unavailable")
	w := &fakeWriter{err: wantErr}
	p := newTestProducer(w, Config{TopicPrefix: "app", PublishTimeout: time.Second}, "svc")

	err := p.Publish(context.Background(), sampleEvent())
	assert.ErrorIs(t, err, wantErr, "a write failure is returned so the caller can observe it (and swallow it)")
}

func TestPublishUniqueEnvelopeIDs(t *testing.T) {
	w := &fakeWriter{}
	p := newTestProducer(w, Config{TopicPrefix: "app"}, "svc")

	require.NoError(t, p.Publish(context.Background(), sampleEvent()))
	require.NoError(t, p.Publish(context.Background(), sampleEvent()))
	require.Len(t, w.msgs, 2)

	var a, b envelopeForTest
	require.NoError(t, json.Unmarshal(w.msgs[0].Value, &a))
	require.NoError(t, json.Unmarshal(w.msgs[1].Value, &b))
	assert.NotEqual(t, a.ID, b.ID, "each published event gets a distinct id")
}

func TestTopicMapping(t *testing.T) {
	p := newTestProducer(&fakeWriter{}, Config{TopicPrefix: "orders"}, "svc")
	assert.Equal(t, "orders.books", p.topic(events.AggregateBooks))
}

func TestCloseDelegatesToWriter(t *testing.T) {
	w := &fakeWriter{}
	p := newTestProducer(w, Config{}, "svc")
	require.NoError(t, p.Close())
	assert.True(t, w.closed)

	w2 := &fakeWriter{closeErr: errors.New("close failed")}
	p2 := newTestProducer(w2, Config{}, "svc")
	assert.Error(t, p2.Close())
}

func TestNewReturnsProducerBackedByKafkaWriter(t *testing.T) {
	cfg := Config{Brokers: []string{"localhost:9092"}, TopicPrefix: "app", PublishTimeout: time.Second}
	p := New(cfg, "svc")
	require.NotNil(t, p)
	assert.Equal(t, "app", p.prefix)
	assert.Equal(t, "svc", p.source)
	// New wires the real *kafkago.Writer (no broker contacted until Publish).
	_, ok := p.writer.(*kafkago.Writer)
	assert.True(t, ok)
	require.NoError(t, p.Close())
}
