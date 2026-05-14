-- name: CreateWidget :one
INSERT INTO widgets (name) VALUES ($1) RETURNING *;

-- name: GetWidget :one
SELECT * FROM widgets WHERE id = $1;

-- name: ListWidgets :many
SELECT * FROM widgets ORDER BY id DESC LIMIT $1 OFFSET $2;

-- name: DeleteWidget :exec
DELETE FROM widgets WHERE id = $1;
