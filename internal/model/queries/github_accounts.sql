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

-- name: GetGithubAccountByGhUserID :one
SELECT *
from github_accounts
WHERE gh_user_id = $1;

-- name: GetUserByGhUserID :one
SELECT u.*
FROM github_accounts gh
JOIN users u 
	ON gh.user_id = u.id
WHERE gh.gh_user_id = $1;

