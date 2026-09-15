# Configuration

All runtime configuration is supplied through environment variables. This document is the complete reference; see the [README](../README.md) for the higher-level getting-started walkthrough.

## Loading configuration

Copy [`.env.example`](../.env.example) to `.env`, adjust values for your environment, and load them into the process environment (for example `set -a && . ./.env && set +a` in Bash, or `docker compose --env-file .env up` so Compose picks up substitutions). **Do not commit `.env`.**

At minimum, set `JWT_SECRET_KEY` and `API_SECRET_KEY` to two distinct secrets, each **32 bytes or longer**. To generate URL-safe random values, run:

```bash
go run ../scripts/generate_key.go   # from docs/, or ./scripts/generate_key.go from the repo root
```

`docker-compose.yml` does **not** embed JWT or API secrets; they must come from `.env` or your shell environment so keys are not committed to the repository. The Compose file sets **`GIN_MODE=release`** for the API service so production-style security headers apply; override in `.env` if you need `debug` locally. Service images use **pinned tags** (Postgres, Redis, Mongo), **published ports bind to `127.0.0.1`** for local dev, and Postgres data uses a **named volume** (`postgres_data`, same pattern as `mongo_data`). Remove volumes with `docker compose down -v` when you want a fresh database.

## Variables

Names below match `os.Getenv` usage in this repository.

### Database (PostgreSQL)

| Variable | Purpose |
| -------- | ------- |
| `POSTGRES_HOST` | PostgreSQL hostname (e.g. `localhost` locally, service name in Compose) |
| `POSTGRES_DB` | Database name |
| `POSTGRES_USER` | Database user |
| `POSTGRES_PASSWORD` | Database password |
| `POSTGRES_PORT` | PostgreSQL port |
| `POSTGRES_MAX_OPEN_CONNS` | Optional max open pool connections (default `25`; `pkg/database/db.go`) |
| `POSTGRES_MAX_IDLE_CONNS` | Optional minimum idle pool connections, capped at max open (default `5`) |
| `POSTGRES_CONN_MAX_LIFETIME` | Optional max connection lifetime (Go duration, default `1h`) |
| `POSTGRES_CONN_MAX_IDLE_TIME` | Optional max connection idle time (Go duration, default `10m`) |

### Redis

| Variable | Purpose |
| -------- | ------- |
| `REDIS_ADDR` | Optional full `host:port` for Redis; when set, overrides `REDIS_HOST` / `REDIS_PORT` (`pkg/cache/cache.go`) |
| `REDIS_HOST` | Redis hostname when `REDIS_ADDR` is unset (default `127.0.0.1`) |
| `REDIS_PORT` | Redis TCP port when `REDIS_ADDR` is unset (default `6379`) |
| `REDIS_PASSWORD` | Redis `AUTH` password (optional) |
| `REDIS_USERNAME` | Redis ACL username (optional; Redis 6+) |
| `REDIS_DB` | Logical database index (default `0`) |
| `REDIS_TLS` | Set `true` / `1` / `yes` / `on` to use TLS (`MinVersion` TLS 1.2) |
| `REDIS_TLS_INSECURE` | Set `true` / `1` / `yes` / `on` to skip server certificate verification (**never in production**) |
| `REDIS_DIAL_TIMEOUT` | Dial timeout (Go duration, default `5s`) |
| `REDIS_READ_TIMEOUT` | Read timeout (default `3s`) |
| `REDIS_WRITE_TIMEOUT` | Write timeout (default `3s`) |

### Authentication

| Variable | Purpose |
| -------- | ------- |
| `JWT_SECRET_KEY` | Secret for signing JWTs (`pkg/auth/auth.go`) |
| `ACCESS_TOKEN_TTL` | Optional access JWT lifetime (Go duration; default `5m`) |
| `REFRESH_TOKEN_TTL` | Optional opaque refresh token lifetime (Go duration; default `168h` / 7 days) |
| `TOKEN_DENYLIST_ENABLED` | Optional Redis access-token denylist for logout (`true`/`false`; default on). When on: per-`jti` denylist plus per-user `revoke_before` on logout-all. When off or Redis is unavailable, reads fail open (tokens accepted); login/refresh/logout still work via Postgres. |
| `BCRYPT_COST` | Optional bcrypt work factor for **new** password hashes (integer `10`–`31`; default **`12`**, was 14). Values below `10` clamp to `10` with a log line. See [#128](https://github.com/LAA-Software-Engineering/golang-rest-api-template/issues/128). |
| `API_SECRET_KEY` | Secret compared to the `X-API-Key` header (`pkg/middleware/api_key.go`) |

### HTTP server and middleware

| Variable | Purpose |
| -------- | ------- |
| `GIN_MODE` | Standard Gin variable: `debug` (default if unset), `release` (enables Security + XSS middleware in `pkg/api/router.go`), or `test` |
| `GIN_TRUSTED_PROXIES` | Optional comma-separated CIDRs trusted for `X-Forwarded-For` / `ClientIP` (`pkg/api/router.go`). If unset, only the direct peer address is used. |
| `REQUEST_MAX_BODY_BYTES` | Optional cap on JSON/body bytes for `POST`/`PUT`/`PATCH` (default `1048576`, i.e. 1 MiB; `pkg/middleware/max_body.go`). |
| `REQUEST_CONTEXT_TIMEOUT` | Optional per-request deadline for **`/api/v1/**` only** (Go duration, e.g. `60s`); default `60s`. Set to `0`, `off`, or `none` to disable (`pkg/middleware/request_timeout.go`). Probes and Swagger are outside this group. |

### Rate limiting

| Variable | Purpose |
| -------- | ------- |
| `RATE_LIMIT_ENABLED` | Per-client rate limiting on/off (`true`/`false`; default on). Set `0`/`off`/`none` to disable (`pkg/middleware/rate_limit.go`). |
| `RATE_LIMIT_REQUESTS` | Max requests per client per window (default `60`). |
| `RATE_LIMIT_WINDOW` | Fixed window duration (Go duration, default `1m`). |
| `RATE_LIMIT_BACKEND` | Counter store: `redis` (default, shared across instances) or `memory` (single process only). |

### Observability (metrics and tracing)

| Variable | Purpose |
| -------- | ------- |
| `METRICS_ENABLED` | Prometheus metrics on/off (`true`/`false`; default on). Set `0`/`off`/`none` to disable (`pkg/middleware/metrics.go`). |
| `METRICS_PATH` | Scrape path for Prometheus (default `/metrics`). Must start with `/`. |
| `OTEL_TRACES_ENABLED` | Opt-in OpenTelemetry tracing (`true` / `1` / `yes` / `on`). Default off (`pkg/tracing`). |
| `OTEL_SERVICE_NAME` | Resource `service.name` for traces (default `golang-rest-api-template`). |
| `OTEL_TRACES_EXPORTER` | Trace exporter when enabled: `otlp` (default), `stdout`, or `none`. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP collector base URL (standard OpenTelemetry env; e.g. `http://localhost:4318`). |

When tracing is enabled, each request gets a server span after `X-Request-Id` is assigned. The request id is recorded on the span as `http.request_id`, and access logs include `trace_id` / `span_id` when a valid span is present.

### Domain events (optional Kafka publishing)

Disabled by default. Delivery is synchronous and best-effort; see **[events.md](./events.md)** for semantics and the event contract.

| Variable | Purpose |
| -------- | ------- |
| `EVENTS_DRIVER` | `none` (default; no publishing) or `kafka`. Selected in `cmd/server`. |
| `SERVICE_NAME` | Envelope `source` field (default `golang-rest-api-template`); rename per deployment. |
| `KAFKA_BROKERS` | Comma-separated `host:port` broker list. **Required** when `EVENTS_DRIVER=kafka`. |
| `KAFKA_TOPIC_PREFIX` | Topic prefix; topic is `<prefix>.books` (default `app`). |
| `KAFKA_PUBLISH_TIMEOUT` | Per-publish timeout, Go duration (default `2s`). Keep short: it bounds added write latency when the broker is slow. |
| `KAFKA_CONSUMER_GROUP` | Only used by the example consumer in `examples/kafka-consumer` (default `example-consumer`). |

## End-to-end test variables

The Python E2E suite reads **`BASE_URL`** and **`API_KEY`** from the environment only (no baked-in defaults). `API_KEY` must match the value the API accepts in `X-API-Key` (for a Compose-backed local run, that is the same secret as `API_SECRET_KEY` in `.env`). See [End-to-End (E2E) Tests](../README.md#end-to-end-e2e-tests) in the README for the full workflow.
