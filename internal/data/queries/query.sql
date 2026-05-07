-- name: InsertTestUser :one
INSERT INTO users(username, password_hash, salt)
VALUES ('admin', 'asdas', '123123')
RETURNING *;

-- name: CreateUser :one
INSERT INTO users (username, password_hash, salt)
VALUES ($1, $2, $3)
RETURNING id, username, password_hash, salt, created_at, updated_at;

-- name: GetUserByName :one
SELECT username, salt, id, password_hash
FROM users
WHERE username = @username;
