-- name: InsertGeneration :one
INSERT INTO generations (
	hash, boot_id
) VALUES (
	$1, (SELECT id FROM boot_id)
)
RETURNING id;

-- name: GetFreshGeneration :one
SELECT g.id
FROM generations g
INNER JOIN blogs b
	ON b.live_hash = g.hash
WHERE b.id = @blog_id
	AND g.boot_id = (SELECT id FROM boot_id)
	AND g.stale = false
LIMIT 1;

-- name: InsertBinding :exec
INSERT INTO bindings (
	gen, url, path
) VALUES (
	$1, $2, $3
);

-- name: GetBinding :one
SELECT path
FROM bindings
WHERE gen = @generation AND url = $1;

-- name: InsertPostEmailBinding :exec
INSERT INTO post_email_bindings (
	gen, url, html, text
) VALUES (
	$1, $2, $3, $4
);

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

-- name: CountEmailClicks :one
SELECT COUNT(*)
FROM subscriber_emails
WHERE clicked = true AND url = $1 AND blog = $2;

-- name: GetPostByToken :one
SELECT *
FROM posts
WHERE email_token = $1;

-- name: SetPostEmailSent :exec
UPDATE _r_posts
SET
	email_sent = true
WHERE url = $1 AND blog = $2;
