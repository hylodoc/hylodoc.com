-- name: InsertGeneration :one
INSERT INTO generations (
	blog
) VALUES (
	$1
)
RETURNING id;

-- name: GetLastGenerationBySubdomain :one
SELECT g.id
FROM generations g
INNER JOIN blogs b
	ON b.id = g.blog
WHERE subdomain = $1
ORDER BY b.id DESC
LIMIT 1;

-- name: DeactivateGenerations :exec
UPDATE generations
SET active = false
WHERE blog = $1;

-- name: InsertBinding :exec
INSERT INTO bindings (
	gen, url, file
) VALUES (
	$1, $2, $3
);

-- name: GetBinding :one
SELECT file
FROM bindings
WHERE gen = @generation AND url = $1;

-- name: InsertRPost :exec
INSERT INTO _r_posts (
	url, blog, published_at, title
) VALUES (
	$1, $2, $3, $4
);

-- name: UpdateRPost :exec
UPDATE _r_posts
SET
	published_at = $3,
	title = $4
WHERE url = $1 AND blog = $2;

-- name: GetPostExists :one
SELECT 1
FROM posts
WHERE url = $1 AND blog = $2;

-- name: RecordBlogVisit :exec
INSERT INTO visits (
	url, blog
) VALUES (
	$1, $2
);

-- name: ListPostsByBlog :many
SELECT *
FROM posts
WHERE blog = $1
ORDER BY published_at DESC;

-- name: CountVisits :one
SELECT COUNT(*)
FROM visits
WHERE url = $1 AND blog = $2;
