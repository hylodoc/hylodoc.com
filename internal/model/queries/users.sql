-- name: CreateUser :one
INSERT INTO users (
	email
) VALUES (
	$1
)
RETURNING *;

-- name: GetUserByID :one
SELECT *
FROM users
WHERE id = $1;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: GetUserForInstallation :one
SELECT *
FROM users u
JOIN installations i ON u.id = i.user_id
WHERE i.gh_installation_id = $1 AND i.active = true;
