-- name: CreateBlog :one
INSERT INTO blogs (
	user_id,
	gh_repository_id,
	theme,
	subdomain,
	live_branch,
	live_hash,
	from_address,
	email_mode
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: BlogDomainExists :one
SELECT EXISTS (
	SELECT 1
	FROM blogs
	WHERE domain = $1::VARCHAR
);

-- name: SubdomainIsTaken :one
SELECT EXISTS (
	SELECT 1 FROM blogs b WHERE b.subdomain = $1
	UNION
	SELECT 1 FROM reserved_subdomains r WHERE r.subdomain = $1
);

-- name: UpdateBlogSubdomainByID :exec
UPDATE blogs
SET subdomain = $1
WHERE id = $2;

-- name: UpdateBlogDomainByID :exec
UPDATE blogs
SET domain = $1
WHERE id = $2;

-- name: UpdateBlogName :exec
UPDATE blogs
SET name = @name::VARCHAR
WHERE id = $1;

-- name: UpdateBlogLiveHash :exec
UPDATE blogs
SET live_hash = @live_hash::VARCHAR
WHERE id = $1;

-- name: CheckBlogOwnership :one
SELECT EXISTS (
	SELECT 1
	FROM blogs
	WHERE id = $1 -- blogID
	AND user_id = $2 -- userID
) AS owns_blog;

-- name: GetBlogByID :one
SELECT *
FROM blogs
WHERE id = $1;

-- name: GetBlogByGhRepositoryID :one
SELECT *
FROM blogs
WHERE gh_repository_id = $1;

-- name: GetBlogBySubdomain :one
SELECT *
FROM blogs
WHERE subdomain = $1;

-- name: GetBlogByDomain :one
SELECT *
FROM blogs
WHERE domain = $1::VARCHAR;

-- name: ListBlogIDsByUserID :many
SELECT id
FROM blogs b
WHERE user_id = $1;

-- name: GetBlogIsLive :one
SELECT is_live
FROM blogs
WHERE id = $1;

-- name: SetBlogToLive :exec
UPDATE blogs
SET is_live = true
WHERE id = $1;

-- name: SetBlogToOffline :exec
UPDATE blogs
SET is_live = false
WHERE id = $1;

-- name: SetBlogEmailMode :exec
UPDATE blogs
SET email_mode = $1
WHERE id = $2;

-- name: ListBlogsForInstallationByGhInstallationID :many
SELECT b.*
FROM blogs b
INNER JOIN repositories r
ON b.gh_repository_id = r.id
WHERE r.installation_id = $1;

-- name: CountLiveBlogsByUserID :one
SELECT COUNT(*) AS blog_count
FROM blogs
WHERE user_id = $1 AND is_live = true;

-- name: SetBlogThemeByID :exec
UPDATE blogs
SET
	theme = $1
WHERE id = $2;

-- name: SetLiveBranchByID :exec
UPDATE blogs
SET
	live_branch = $1
WHERE id = $2;

-- name: DeleteBlogByID :exec
DELETE
FROM blogs
WHERE id = $1;
