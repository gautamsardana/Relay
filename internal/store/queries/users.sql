-- name: CreateUser :one
INSERT INTO users (user_id, email)
VALUES ($1, $2)
RETURNING user_id, email, created_at;

-- name: GetUserByID :one
SELECT user_id, email, created_at
FROM users
WHERE user_id = $1;

-- name: GetUserByEmail :one
SELECT user_id, email, created_at
FROM users
WHERE email = $1;
