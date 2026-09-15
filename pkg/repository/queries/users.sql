-- name: CreateUser :one
INSERT INTO users (username, password, role)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByUsername :one
SELECT * FROM users
WHERE username = $1;

-- name: GetUserByID :one
SELECT * FROM users
WHERE id = $1;
