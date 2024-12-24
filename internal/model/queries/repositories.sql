-- name: CreateRepository :one
INSERT INTO repositories (
	installation_id,
	repository_id,
	url,
	name,
	full_name
) VALUES (
	$1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetRepositoryByGhRepositoryID :one
SELECT *
FROM repositories
WHERE repository_id = $1;

-- name: ListOrderedRepositoriesByUserID :many
SELECT r.*
FROM repositories r
INNER JOIN installations i
ON r.installation_id = i.gh_installation_id
WHERE i.user_id = $1
ORDER BY r.created_at ASC;

-- name: DeleteRepositoryWithGhRepositoryID :exec
DELETE
FROM repositories
WHERE repository_id = $1;
