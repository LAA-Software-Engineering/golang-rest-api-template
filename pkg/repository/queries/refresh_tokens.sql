-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (user_id, token_hash, family_id, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetRefreshTokenByHash :one
SELECT * FROM refresh_tokens
WHERE token_hash = $1;

-- ConsumeRefreshTokenIfActive marks a token consumed only when it is still active.
-- The :execrows count lets the caller detect a lost race (0 rows) vs success (1 row).
-- name: ConsumeRefreshTokenIfActive :execrows
UPDATE refresh_tokens
SET consumed_at = $2,
    updated_at = now()
WHERE id = $1
  AND consumed_at IS NULL
  AND revoked_at IS NULL;

-- name: RevokeRefreshFamily :exec
UPDATE refresh_tokens
SET revoked_at = $2,
    updated_at = now()
WHERE family_id = $1
  AND revoked_at IS NULL;

-- name: RevokeAllRefreshForUser :exec
UPDATE refresh_tokens
SET revoked_at = $2,
    updated_at = now()
WHERE user_id = $1
  AND revoked_at IS NULL;
