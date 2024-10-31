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
	url, blog
) VALUES (
	$1, $2
);

-- name: GetPostExists :one
SELECT 1
FROM posts
WHERE url = $1 AND blog = $2;
