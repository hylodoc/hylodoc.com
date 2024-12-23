-- name: InsertQueuedEmail :exec
INSERT INTO queued_emails (
	from_addr, to_addr, subject, body, mode
) VALUES (
	$1, $2, $3, $4, $5
);

-- name: MarkQueuedEmailSent :exec
UPDATE queued_emails
SET status = 'sent'
WHERE id = $1;

-- name: MarkQueuedEmailFailed :exec
UPDATE queued_emails
SET status = 'failed'
WHERE id = $1;

-- name: MarkQueuedEmailAttempt :exec
UPDATE queued_emails
SET status = status+1
WHERE id = $1;

-- name: GetTopNQueuedEmails :many
SELECT *
FROM queued_emails
WHERE status = 'pending'
ORDER BY created_at DESC
LIMIT $1::INT;

-- name: GetQueuedEmailHeaders :many
SELECT
	name,
	value
FROM queued_email_headers
WHERE email = $1;
