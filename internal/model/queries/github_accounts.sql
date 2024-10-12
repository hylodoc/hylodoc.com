-- name: CreateGithubAccount :one
INSERT INTO github_accounts (
	user_id, gh_user_id, gh_email, gh_username
) VALUES (
	$1, $2, $3, $4
)
RETURNING *;

-- name: GetGithubAccountByUserID :one
SELECT *
FROM github_accounts
WHERE user_id = $1;

-- name: GetUserByGhUserID :one
SELECT *
FROM github_accounts
WHERE gh_user_id = $1;

