-- name: CreateRepository :one
INSERT INTO repositories (
	installation_id, gh_repository_id, name, url, owner
) VALUES (
	$1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetRepositoriesForInstallation :many
SELECT r.id, r.name, r.url, r.owner
FROM repositories r
INNER JOIN installations i
ON r.installation_id = i.id
WHERE i.gh_installation_id = $1 AND r.active = true;

-- name: DeleteRepository :exec
DELETE
FROM repositories
WHERE gh_repository_id = $1;
