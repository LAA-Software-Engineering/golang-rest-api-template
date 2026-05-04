# Pin swag to the same version as Dockerfile (github.com/swaggo/swag/cmd/swag@v1.16.4).
setup:
	go install github.com/swaggo/swag/cmd/swag@v1.16.4
	swag init -g ./cmd/server/main.go -o ./docs

build-docker:
	docker compose build --no-cache

# Run API against local Docker DBs. Requires `.env` for secrets and DB settings (see .env.example).
# Only overrides service hostnames to localhost for containers started outside Compose.
# Optional: set GIN_MODE=release in `.env` for the same security middleware as Docker Compose.
run-local:
	docker start dockerPostgres
	docker start dockerRedis
	docker start dockerMongo
	test -f .env || { echo >&2 "Missing .env — copy .env.example to .env and set secrets (>=32 bytes each)."; exit 1; }
	set -euo pipefail; \
	set -a && . ./.env && set +a; \
	export REDIS_HOST=localhost POSTGRES_HOST=localhost; \
	go run cmd/server/main.go

up:
	docker compose up

down:
	docker compose down

restart:
	docker compose restart

build:
	go build -v ./...

test:
	go test -v ./... -race -cover

test-cover:
	PKGS=$$(go list ./... | grep -vE '(^|/)(cmd/server|docs|scripts)$$'); \
	go test -race -coverprofile=coverage.out $$PKGS; \
	go tool cover -html=coverage.out -o coverage.html

clean:
	docker stop go-rest-api-template
	docker stop dockerPostgres
	docker rm go-rest-api-template
	docker rm dockerPostgres
	docker rm dockerRedis
	docker image rm golang-rest-api-template-backend
	rm -rf .dbdata
