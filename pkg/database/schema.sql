-- Application schema for golang-rest-api-template.
--
-- This file is the single source of truth for the database structure. It is:
--   1. embedded and executed at startup by pkg/database (replacing GORM AutoMigrate), and
--   2. the schema input for sqlc code generation (see sqlc.yaml).
--
-- All statements must be idempotent (IF NOT EXISTS) so startup is safe to repeat.

CREATE TABLE IF NOT EXISTS books (
    id         BIGSERIAL PRIMARY KEY,
    owner_id   BIGINT NOT NULL,
    title      TEXT NOT NULL DEFAULT '',
    author     TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_books_owner_id ON books (owner_id);

CREATE TABLE IF NOT EXISTS users (
    id         BIGSERIAL PRIMARY KEY,
    username   TEXT NOT NULL UNIQUE,
    password   TEXT NOT NULL DEFAULT '',
    role       VARCHAR(32) NOT NULL DEFAULT 'user',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL,
    token_hash  VARCHAR(64) NOT NULL UNIQUE,
    family_id   VARCHAR(64) NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    consumed_at TIMESTAMPTZ,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_family_id ON refresh_tokens (family_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens (expires_at);
