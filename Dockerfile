# Align with go.mod and CI.
FROM golang:1.25.13-bookworm AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Pin this version instead of @latest.
RUN go install github.com/swaggo/swag/cmd/swag@v1.16.4

RUN swag init -g ./cmd/server/main.go -o ./docs

RUN CGO_ENABLED=1 go build -o /out/server ./cmd/server/main.go


FROM debian:bookworm-slim AS runtime

WORKDIR /app

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates curl \
	&& rm -rf /var/lib/apt/lists/*

RUN useradd --system --no-create-home --uid 10001 appuser

COPY --from=builder /out/server /app/server

# Default container image to Gin release mode (Security/XSS middleware in router).
ENV GIN_MODE=release

USER appuser

EXPOSE 8001

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
	CMD curl -fsS http://127.0.0.1:8001/livez >/dev/null || exit 1

CMD ["/app/server"]
