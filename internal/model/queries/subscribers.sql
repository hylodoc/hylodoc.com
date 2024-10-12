-- name: CreateSubscriber :one
INSERT INTO subscribers (
	blog_id, email, unsubscribe_token
) VALUES (
	$1, $2, $3
)
RETURNING *;

-- name: GetSubscriberForBlog :one
SELECT *
FROM subscribers
WHERE blog_id = $1 AND email = $2 AND status = 'active';

-- name: ListActiveSubscribersForGhRepositoryID :many
SELECT s.email, s.unsubscribe_token
FROM subscribers s
INNER JOIN blogs b
ON s.blog_id = b.id
WHERE b.gh_repository_id = $1 AND b.active = true AND s.status = 'active';

-- name: ListActiveSubscribersByBlogID :many
SELECT *
FROM subscribers
WHERE blog_id = $1 and status = 'active';

-- name: DeleteSubscriberForBlog :exec
UPDATE subscribers
SET status = 'unsubscribed'
WHERE unsubscribe_token = $1 AND blog_id = $2;

-- name: DeleteSubscriberByEmail :exec
UPDATE subscribers
SET status = 'unsubscribed'
WHERE email = $1 AND blog_id = $2;

