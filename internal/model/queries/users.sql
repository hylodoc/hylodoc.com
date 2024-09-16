-- name: CreateUser :one
INSERT INTO users (
	email, username
) VALUES (
	$1, $2
)
RETURNING *;

-- name: GetUserByEmail :one
SELECT * FROM users
WHERE email = $1
LIMIT 1;
