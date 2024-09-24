-- name: CreateUser :one
INSERT INTO users (
	gh_user_id, email, username
) VALUES (
	$1, $2, $3
)
RETURNING *;

-- name: GetUserByGithubId :one
SELECT * FROM users
WHERE gh_user_id = $1
LIMIT 1;

-- name: GetUserForInstallation :one
SELECT u.id, u.gh_user_id, u.email, u.username
FROM users u
JOIN installations i ON u.id = i.user_id
WHERE i.gh_installation_id = $1 AND i.active = true;
