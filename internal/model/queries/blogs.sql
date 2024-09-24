-- name: CreateBlog :one
INSERT INTO blogs (
	installation_id, gh_repository_id, name, full_name, url
) VALUES (
	$1, $2, $3, $4, $5
)
RETURNING *;

-- name: GetBlogWithGithubRepositoryID :one
SELECT *
FROM blogs
WHERE gh_repository_id = $1;

-- name: GetBlogsForInstallation :many
SELECT r.id, r.name, r.full_name, r.url
FROM blogs r
INNER JOIN installations i
ON r.installation_id = i.id
WHERE i.gh_installation_id = $1 AND r.active = true;

-- name: DeleteBlogWithGithubRepositoryID :exec
DELETE
FROM blogs
WHERE gh_repository_id = $1;
