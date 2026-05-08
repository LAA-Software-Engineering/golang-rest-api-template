# golang-rest-api-template

[![license](https://img.shields.io/badge/license-MIT-green)](https://raw.githubusercontent.com/araujo88/golang-rest-api-template/main/LICENSE)
[![build](https://github.com/araujo88/golang-rest-api-template/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/araujo88/golang-rest-api-template/actions/workflows/ci.yml)

## Overview

This repository provides a template for building a RESTful API using Go with features like JWT Authentication, rate limiting, Swagger documentation, and database operations using GORM. The application uses the Gin Gonic web framework and is containerized using Docker.

## Features

- RESTful API endpoints for CRUD operations.
- JWT Authentication.
- Rate Limiting.
- Swagger Documentation.
- PostgreSQL database integration using GORM.
- Redis cache.
- MongoDB for logging storage.
- Dockerized application for easy setup and deployment.

## Folder structure

```
golang-rest-api-template/
├── .github
│  ├── dependabot.yml
│  ├── pull_request_template.md
│  └── workflows
│     └── ci.yml
├── cmd
│  └── server
│     └── main.go
├── docker-compose.yml
├── Dockerfile
├── .dockerignore
├── .env.example
├── .golangci.yml
├── docs
│  ├── docs.go
│  ├── swagger.json
│  └── swagger.yaml
├── go.mod
├── go.sum
├── LICENSE
├── Makefile
├── pkg
│  ├── api
│  │  ├── books.go
│  │  ├── books_test.go
│  │  ├── book_routes_auth_test.go
│  │  ├── main_test.go
│  │  ├── probes.go
│  │  ├── probes_test.go
│  │  ├── router.go
│  │  ├── router_trusted_proxies_test.go
│  │  ├── user.go
│  │  └── user_test.go
│  ├── auth
│  │  ├── auth.go
│  │  └── auth_test.go
│  ├── cache
│  │  ├── cache.go
│  │  ├── cache_mock.go
│  │  ├── cache_test.go
│  │  └── redis_env_test.go
│  ├── database
│  │  ├── db.go
│  │  ├── db_test.go
│  │  ├── doc.go
│  │  ├── mongo.go
│  │  └── mongo_test.go
│  ├── middleware
│  │  ├── api_key.go
│  │  ├── api_key_test.go
│  │  ├── jwt_auth.go
│  │  ├── jwt_auth_test.go
│  │  ├── cors.go
│  │  ├── logger.go
│  │  ├── max_body.go
│  │  ├── max_body_test.go
│  │  ├── rate_limit.go
│  │  ├── request_id.go
│  │  ├── request_id_test.go
│  │  ├── security.go
│  │  └── xss.go
│  └── models
│     ├── book.go
│     ├── user.go
│     └── user_test.go
├── README.md
├── scripts
│  └── generate_key.go
├── tests
│  ├── e2e.py
│  └── requirements.txt
└── vendor
```

## Getting Started

### Prerequisites

- Go 1.25.10 or newer (see `go.mod`; aligns CI and Docker with `govulncheck` / patched stdlib)
- Docker
- Docker Compose

### Installation

1. Clone the repository

```bash
git clone https://github.com/araujo88/golang-rest-api-template
```

2. Navigate to the directory

```bash
cd golang-rest-api-template
```

3. Copy [`.env.example`](./.env.example) to `.env` and set secrets (at least `JWT_SECRET_KEY` and `API_SECRET_KEY`, each **32 bytes or longer**; use `go run ./scripts/generate_key.go` twice). Docker Compose reads this file for `${JWT_SECRET_KEY}` and `${API_SECRET_KEY}` interpolation.

4. Build and run the Docker containers

```bash
make up
```

Please refer to the [Makefile](./Makefile) if you need to build in the local environment. The `run-local` target also requires a populated `.env` for those two variables.

### Environment Variables

Copy [`.env.example`](./.env.example) to `.env`, adjust values for your environment, and load them into the process environment (for example `set -a && . ./.env && set +a` in Bash, or `docker compose --env-file .env up` so Compose picks up substitutions). **Do not commit `.env`.**

Names below match `os.Getenv` usage in this repository:

| Variable | Purpose |
| -------- | ------- |
| `POSTGRES_HOST` | PostgreSQL hostname (e.g. `localhost` locally, service name in Compose) |
| `POSTGRES_DB` | Database name |
| `POSTGRES_USER` | Database user |
| `POSTGRES_PASSWORD` | Database password |
| `POSTGRES_PORT` | PostgreSQL port |
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
| `JWT_SECRET_KEY` | Secret for signing JWTs (`pkg/auth/auth.go`) |
| `API_SECRET_KEY` | Secret compared to the `X-API-Key` header (`pkg/middleware/api_key.go`) |
| `GIN_MODE` | Standard Gin variable: `debug` (default if unset), `release` (enables Security + XSS middleware in `pkg/api/router.go`), or `test` |
| `GIN_TRUSTED_PROXIES` | Optional comma-separated CIDRs trusted for `X-Forwarded-For` / `ClientIP` (`pkg/api/router.go`). If unset, only the direct peer address is used. |
| `REQUEST_MAX_BODY_BYTES` | Optional cap on JSON/body bytes for `POST`/`PUT`/`PATCH` (default `1048576`, i.e. 1 MiB; `pkg/middleware/max_body.go`). |

To generate URL-safe random values for `JWT_SECRET_KEY` and `API_SECRET_KEY`, run:

```bash
go run ./scripts/generate_key.go
```

`docker-compose.yml` does **not** embed JWT or API secrets; they must come from `.env` or your shell environment so keys are not committed to the repository. The Compose file sets **`GIN_MODE=release`** for the API service so production-style security headers apply; override in `.env` if you need `debug` locally. Service images use **pinned tags** (Postgres, Redis, Mongo), **published ports bind to `127.0.0.1`** for local dev, and Postgres data uses a **named volume** (`postgres_data`, same pattern as `mongo_data`). Remove volumes with `docker compose down -v` when you want a fresh database.

### API Documentation

The API is documented using Swagger and can be accessed at:

```
http://localhost:8001/swagger/index.html
```

## Usage

### Endpoints

- `GET /livez`: Liveness probe (process up; no dependency checks; no `X-API-Key`).
- `GET /readyz`: Readiness probe (checks Postgres, Redis, Mongo; **503** if a dependency fails; no `X-API-Key`). Docker image `HEALTHCHECK` uses **`/livez`** so the container stays alive while dependencies recover.
- `GET /api/v1/`: Health check (no `X-API-Key`; lightweight app-level ping).
- `GET /api/v1/books`: Get all books.
- `GET /api/v1/books/:id`: Get a single book by ID.
- `POST /api/v1/books`: Create a new book.
- `PUT /api/v1/books/:id`: Replace a book's title and author (both fields required in the JSON body).
- `PATCH /api/v1/books/:id`: Partially update a book (send only the fields to change; at least one of `title` or `author` is required).
- `DELETE /api/v1/books/:id`: Delete a book.
- `POST /api/v1/login`: Login.
- `POST /api/v1/register`: Register a new user.
- `GET /swagger/*`: Swagger UI (no `X-API-Key`).

### JSON responses

Successful `/api/v1` JSON responses use a single envelope: **`{"data": ...}`** (implemented in `pkg/httpresp`). Examples: `GET /api/v1/` returns `{"data":"ok"}`; book list and book CRUD return the resource or collection inside `data`; `POST /api/v1/login` returns `{"data":{"token":"<jwt>"}}`; `POST /api/v1/register` returns `{"data":{"message":"Registration successful"}}`.

Error responses use **`{"error":"..."}`** (`pkg/httperr`). RFC 7807-style problem details are not used yet.

### Authentication

Under **`/api/v1`**, every route **except** `GET /api/v1/` (health) requires the **`X-API-Key`** header matching **`API_SECRET_KEY`** (service-to-service gate).

Book **mutations** (`POST`, `PUT`, `PATCH`, and `DELETE` on `/api/v1/books` and `/api/v1/books/:id`) also require a valid user JWT in `Authorization: Bearer <token>` (obtain a token from `POST /api/v1/login`; the JWT string is at **`data.token`** in the JSON body). Book **reads** (`GET` list and `GET` by id) require the API key only.

```bash
curl -H "X-API-Key: <YOUR_API_KEY>" http://localhost:8001/api/v1/books
```

```bash
curl -X POST \
  -H "X-API-Key: <YOUR_API_KEY>" \
  -H "Authorization: Bearer <YOUR_JWT>" \
  -H "Content-Type: application/json" \
  -d '{"title":"Example","author":"Author"}' \
  http://localhost:8001/api/v1/books
```

## Contributing

Pull requests are welcome. For major changes, please open an issue first to discuss what you would like to change.

## End-to-End (E2E) Tests

This project contains end-to-end (E2E) tests to verify the functionality of the API. The tests are written in Python using the `pytest` framework.

### Prerequisites

Before running the tests, ensure you have the following:

- Python 3.x installed
- `pip` (Python package manager)
- The API service running locally or on a staging server
- API key available

### Setup

#### 1. Create a virtual environment (optional but recommended):

```bash
python -m venv venv
source venv/bin/activate  # On Windows: venv\Scripts\activate
```

#### 2. Install dependencies:

```bash
pip install -r tests/requirements.txt
```

#### 3. Set up the environment variables:

E2E tests read **`BASE_URL`** and **`API_KEY`** from the environment only (no baked-in defaults). `API_KEY` must match the value the API accepts in `X-API-Key` (for a Compose-backed local run, that is the same secret as `API_SECRET_KEY` in `.env`).

With a project-root `.env` (as used by Docker Compose), load it before pytest:

```bash
set -a && . ./.env && set +a
export BASE_URL=http://127.0.0.1:8001/api/v1
export API_KEY="$API_SECRET_KEY"
pytest -v tests/e2e.py
```

For a **staging** server, export the same variables with your deployment values.

#### 4. Run the tests:

After `BASE_URL` and `API_KEY` are set (see step 3):

```bash
pytest -v tests/e2e.py
```

### Test Structure

The tests will perform the following actions:

1. Register a new user, log in, and obtain a JWT from the login response (`data.token`).
2. Create a new book in the system.
3. Retrieve all books and verify the created book is present.
4. Retrieve a specific book by its ID.
5. Replace the book's title and author via `PUT`, and patch the title only via `PATCH`.
6. Delete the book and verify it is no longer accessible.

Each test includes assertions to ensure that the API behaves as expected.
