-- Dynamic listing (filters + allowlisted sort/order + pagination) is implemented
-- as hand-written parameterized pgx in book_sqlc.go; it cannot be expressed as a
-- single static sqlc query. The statements below cover the type-safe CRUD paths.

-- name: CreateBook :one
INSERT INTO books (owner_id, title, author)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetBook :one
SELECT * FROM books
WHERE id = $1;

-- name: UpdateBookFields :one
UPDATE books
SET title = $2,
    author = $3,
    updated_at = now()
WHERE id = $1
RETURNING *;

-- name: PatchBookFields :one
UPDATE books
SET title = COALESCE(sqlc.narg('title'), title),
    author = COALESCE(sqlc.narg('author'), author),
    updated_at = now()
WHERE id = sqlc.arg('id')
RETURNING *;

-- name: DeleteBook :execrows
DELETE FROM books
WHERE id = $1;
