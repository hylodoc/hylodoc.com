-- name: CreateBlog :one
INSERT INTO blogs (
	installation_id, gh_repository_id, gh_name, gh_full_name, gh_url, subdomain, demo_subdomain, from_address
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: SubdomainExists :one
SELECT EXISTS (
	SELECT 1
	FROM blogs
	WHERE subdomain = $1
) AS sub_exists;

-- name: UpdateSubdomainByID :exec
UPDATE blogs
SET subdomain = $1
WHERE id = $2;

-- name: CheckBlogOwnership :one
SELECT EXISTS (
	SELECT 1
	FROM blogs b
	INNER JOIN installations i ON b.installation_id = i.id
	WHERE b.id = $1 -- blogID
	AND i.user_id = $2 -- userID
) AS owns_blog;

-- name: GetBlogByID :one
SELECT *
FROM blogs
WHERE id = $1;

-- name: GetBlogByGhRepositoryID :one
SELECT *
FROM blogs
WHERE gh_repository_id = $1;

-- name: ListBlogsForInstallationByGhInstallationID :many
SELECT *
FROM blogs b
INNER JOIN installations i
ON b.installation_id = i.id
WHERE i.gh_installation_id = $1;

-- name: DeleteBlogWithGhRepositoryID :exec
DELETE
FROM blogs
WHERE gh_repository_id = $1;
