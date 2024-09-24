-- name: CreateRepository :one
INSERT INTO repositories (
	installation_id, gh_repository_id, name, full_name, url
) VALUES (
	$1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetRepositoryWithGithubRepositoryID :one
SELECT *
FROM repositories
WHERE gh_repository_id = $1;

-- name: GetRepositoriesForInstallation :many
SELECT r.id, r.name, r.full_name, r.url
FROM repositories r
INNER JOIN installations i
ON r.installation_id = i.id
WHERE i.gh_installation_id = $1 AND r.active = true;

-- name: DeleteRepositoryWithGithubRepositoryID :exec
DELETE
FROM repositories
WHERE gh_repository_id = $1;
