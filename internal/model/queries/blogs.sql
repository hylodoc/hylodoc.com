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
	from_address,
	blog_type
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9, $10
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

-- name: GetBlogIDBySubdomain :one
SELECT id
FROM blogs
WHERE subdomain = @subdomain::VARCHAR;

-- name: ListBlogIDsByUserID :many
SELECT id
FROM blogs b
WHERE user_id = $1;

-- name: ListBlogRepoPathsByUserID :many
SELECT repository_path
FROM blogs b
WHERE user_id = $1;

-- name: GetBlogIsLive :one
SELECT EXISTS (
	SELECT 1
	FROM generations
	WHERE blog = $1 AND active = true
);

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
