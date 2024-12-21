-- name: CreateBlog :one
INSERT INTO blogs (
	user_id,
	gh_repository_id,
	gh_url,
	repository_path,
	theme,
	subdomain,
	test_branch,
	live_branch,
	live_hash,
	from_address,
	blog_type,
	email_mode
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
)
RETURNING *;

-- name: DomainExists :one
SELECT EXISTS (
	SELECT 1
	FROM blogs
	WHERE domain = $1::VARCHAR
);

-- name: SubdomainExists :one
SELECT EXISTS (
	SELECT 1
	FROM blogs
	WHERE subdomain = $1
);

-- name: UpdateSubdomainByID :exec
UPDATE blogs
SET subdomain = $1
WHERE id = $2;

-- name: UpdateDomainByID :exec
UPDATE blogs
SET domain = $1
WHERE id = $2;

-- name: UpdateBlogName :exec
UPDATE blogs
SET name = @name::VARCHAR
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
WHERE gh_repository_id = $1 AND blog_type = 'repository';

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

-- name: ListBlogRepoPathsByUserID :many
SELECT repository_path
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

-- name: ListBlogsForInstallationByGhInstallationID :many
SELECT *
FROM blogs b
INNER JOIN repositories r
ON b.gh_repository_id = r.id
WHERE r.installation_id = $1;

-- name: CountBlogsByUserID :one
SELECT COUNT(*) AS blog_count
FROM blogs
WHERE user_id = $1;

-- name: SetBlogThemeByID :exec
UPDATE blogs
SET
	theme = $1
WHERE id = $2;

-- name: SetTestBranchByID :exec
UPDATE blogs
SET
	test_branch = $1
WHERE id = $2;

-- name: SetLiveBranchByID :exec
UPDATE blogs
SET
	live_branch = $1
WHERE id = $2;

-- name: DeleteBlogWithGhRepositoryID :exec
DELETE
FROM blogs
WHERE gh_repository_id = $1;
