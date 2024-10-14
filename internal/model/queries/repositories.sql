-- name: CreateRepository :one
INSERT INTO repositories (
	user_id,
	installation_id,
	repository_id,
	url,
	name,
	full_name
) VALUES (
	$1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetRepositoryByGhRepositoryID :one
SELECT *
FROM repositories
WHERE repository_id = $1;

-- name: GetRepositoryByBlogID :one
SELECT *
FROM repositories r
INNER JOIN blogs b
ON b.gh_repository_id = r.repository_id
WHERE b.id = $1;

-- name: ListOrderedRepositoriesByUserID :many
SELECT *
FROM repositories
WHERE user_id = $1
ORDER BY created_at ASC;

-- name: DeleteRepositoryWithGhRepositoryID :exec
DELETE
FROM repositories
WHERE repository_id = $1;
