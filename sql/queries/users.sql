-- name: CreateUser :one
INSERT INTO users (id, created_at, updated_at, email, hashed_password)
VALUES (gen_random_uuid(), NOW(), NOW(), $1, $2)
RETURNING *;

-- name: Reset :one
DELETE FROM users RETURNING NULL;

-- name: GetUserByEmail :one
SELECT * FROM users WHERE email = $1;

-- name: UpdateUser :one
UPDATE users
SET email = $2, hashed_password = $3
WHERE id = $1
RETURNING *;
