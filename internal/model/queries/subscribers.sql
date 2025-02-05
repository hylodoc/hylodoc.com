-- name: CreateSubscriber :one
INSERT INTO subscribers (
	blog_id, email
) VALUES (
	$1, $2
)
RETURNING unsubscribe_token;

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

-- name: GetSubscriberByToken :one
SELECT *
FROM subscribers
WHERE unsubscribe_token = $1;

-- name: DeleteSubscriber :exec
UPDATE subscribers
SET status = 'unsubscribed'
WHERE id = $1;

-- name: DeleteSubscriberByEmail :exec
UPDATE subscribers
SET status = 'unsubscribed'
WHERE email = $1 AND blog_id = $2;

-- name: InsertSubscriberEmail :one
INSERT INTO subscriber_emails (
	subscriber, url, blog
) VALUES (
	$1, $2, $3
)
RETURNING token;

-- name: SetSubscriberEmailClicked :exec
UPDATE subscriber_emails
SET clicked = true
WHERE token = $1;
