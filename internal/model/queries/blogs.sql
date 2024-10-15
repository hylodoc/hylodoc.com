-- name: CreateBlog :one
INSERT INTO blogs (
	user_id,
	gh_repository_id,
	gh_url,
	repository_path,
	subdomain,
	test_branch,
	live_branch,
	from_address,
	blog_type
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9
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
WHERE gh_repository_id = $1;

-- name: ListBlogsByUserID :many
SELECT *
FROM blogs b
WHERE user_id = $1;

-- name: ListBlogsForInstallationByGhInstallationID :many
SELECT *
FROM blogs b
INNER JOIN repositories r
ON b.gh_repository_id = r.id
WHERE r.installation_id = $1;

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

-- name: SetBlogStatusByID :exec
UPDATE blogs
SET
	status = $1
WHERE id = $2;

-- name: DeleteBlogWithGhRepositoryID :exec
DELETE
FROM blogs
WHERE gh_repository_id = $1;
