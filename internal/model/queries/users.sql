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
