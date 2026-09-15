# Optional domain event publishing

This template can expose resource lifecycle changes as **domain events** so other
services can react to them (search indexing, analytics, notifications, audit
pipelines, …) without calling the API or polling it.

Event publishing is **completely optional and disabled by default**. Clone the
template, never configure it, and the API behaves exactly as it would without
this feature. Enabling it does not make Kafka part of the application's
architecture — it adds an event-producing side effect to write operations.

```
                            ┌── analytics
                            ├── search indexing
REST API  ──(Kafka)──────── ├── notifications
 (producer)                 ├── audit pipeline
                            └── any downstream consumer
```

## Delivery semantics

Publishing is **synchronous** and **best-effort**:

- The DB mutation commits first; then the event is published; then the HTTP
  response is returned.
- A publish failure is recorded as a metric and **swallowed** — it never turns an
  already-committed mutation into a `5xx`. Returning an error there would tell the
  client the write failed when it did not, inviting a retry that duplicates data.
- Because the Kafka writer retries and a canceled context leaves a write's outcome
  ambiguous, **events may be lost or duplicated**. Consumers must be **idempotent**
  (e.g. dedupe on the envelope `id`).

**The key trade-off:** enabling Kafka does not make Kafka part of write
*correctness*, but it does make broker health part of write *latency*. A slow or
unavailable broker adds up to `KAFKA_PUBLISH_TIMEOUT` (default `2s`) to every
`POST`/`PUT`/`PATCH`/`DELETE`. Keep that timeout short.

> Need "if the transaction commits, the event must eventually be delivered"? That
> requires a transactional outbox (write the event in the same DB transaction, relay
> it asynchronously). It is intentionally **not** part of v1 — it would turn a REST
> template into an event-processing framework. It is the natural upgrade path, added
> as an alternative `events.Publisher` when those guarantees are actually needed.

## Configuration

| Variable | Default | Meaning |
|---|---|---|
| `EVENTS_DRIVER` | `none` | `none` disables publishing; `kafka` enables it. |
| `SERVICE_NAME` | `golang-rest-api-template` | Envelope `source`; rename per deployment. |
| `KAFKA_BROKERS` | — | Comma-separated `host:port` list. Required when driver is `kafka`. |
| `KAFKA_TOPIC_PREFIX` | `app` | Topic is `<prefix>.books`. |
| `KAFKA_PUBLISH_TIMEOUT` | `2s` | Per-publish timeout. |

Driver selection lives in `cmd/server` (`buildPublisher`). The `pkg/events/kafka`
adapter only knows about `KAFKA_*` settings — it does not know that `none` exists —
so Kafka stays a cleanly removable adapter.

## The contract (what consumers depend on)

Events are published as a stable **envelope** wrapping a **versioned payload**.
Internal persistence models are never published directly, so refactoring
`models.Book` cannot accidentally break the public contract.

```json
{
  "id": "018f...-uuid",
  "type": "book.created.v1",
  "source": "my-books-api",
  "occurred_at": "2026-09-15T22:30:00Z",
  "data": { "id": 123, "owner_id": 42, "title": "Dune", "author": "Frank Herbert" }
}
```

Event types (books aggregate):

| Type | Emitted on |
|---|---|
| `book.created.v1` | `POST /books` |
| `book.updated.v1` | `PUT /books/:id` and `PATCH /books/:id` |
| `book.deleted.v1` | `DELETE /books/:id` (carries the pre-delete snapshot) |

### Topics and ordering

One topic per aggregate: `<prefix>.books`. The event `type` distinguishes events
within the topic. Messages are keyed by the aggregate id (the book id) with a hash
balancer, so all changes to the same book land on one partition and preserve order
(for delivered events).

## Observability

Publish attempts increment the Prometheus counter
`book_events_published_total{type, result}` (`result` is `success` or `error`),
scraped on the existing `/metrics` endpoint.

## Try it locally

Start Postgres/Redis/Mongo plus the optional Kafka node and run the API with the
Kafka driver:

```bash
EVENTS_DRIVER=kafka docker compose --profile kafka up
```

In another terminal, run the example consumer (a standalone program under
`examples/`, deliberately not part of the API) against the host listener:

```bash
KAFKA_BROKERS=localhost:29092 go run ./examples/kafka-consumer
```

Then create a book (`POST /api/v1/books`) and watch the `book.created.v1` event
arrive in the consumer's log.
