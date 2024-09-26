-- name: CreateBlog :one
INSERT INTO blogs (
	installation_id, gh_repository_id, gh_name, gh_full_name, gh_url, subdomain, from_address
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: GetBlogWithGhRepositoryID :one
SELECT *
FROM blogs
WHERE gh_repository_id = $1;

-- name: ListBlogsForInstallationWithGhInstallationID :many
SELECT b.*
FROM blogs b
INNER JOIN installations i
ON b.installation_id = i.id
WHERE i.gh_installation_id = $1 AND b.active = true;

-- name: DeleteBlogWithGhRepositoryID :exec
DELETE
FROM blogs
WHERE gh_repository_id = $1;
