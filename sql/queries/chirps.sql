-- name: CreateChirp :one
INSERT INTO chirps (id, created_at, updated_at, user_id, body)
VALUES (gen_random_uuid(), NOW(), NOW(), $1, $2)
RETURNING *;

-- TODO figure out a way to collate GetChirps<sortorder> and GetUserChirps<sortorder>
-- might need to shift away from sqlc to something that allows for dynamic query generation

-- name: GetChirps :many
SELECT * FROM chirps ORDER BY created_at ASC;

-- name: GetChirpsDesc :many
SELECT * FROM chirps ORDER BY created_at DESC;

-- name: GetUserChirps :many
SELECT * FROM chirps WHERE user_id = $1 ORDER BY created_at ASC;

-- name: GetUserChirpsDesc :many
SELECT * FROM chirps WHERE user_id = $1 ORDER BY created_at DESC;

-- name: GetChirp :one
SELECT * FROM chirps WHERE id = $1;

-- name: DeleteChirp :one
DELETE FROM chirps WHERE id = $1 RETURNING NULL;
