-- name: InsertQueuedEmail :one
INSERT INTO queued_emails (
	from_addr, to_addr, subject, body, mode, stream
) VALUES (
	$1, $2, $3, $4, $5, $6
)
RETURNING id;

-- name: MarkQueuedEmailSent :exec
UPDATE queued_emails
SET
	status = 'sent',
	ended_at = now()
WHERE id = $1;

-- name: MarkQueuedEmailFailed :exec
UPDATE queued_emails
SET
	status = 'failed',
	ended_at = now()
WHERE id = $1;

-- name: IncrementQueuedEmailFailCount :exec
UPDATE queued_emails
SET fail_count = fail_count+1
WHERE id = $1;

-- name: InsertQueuedEmailPostmarkError :exec
INSERT INTO queued_email_postmark_error (
	email, code, message
) VALUES (
	$1, $2, $3
);

-- name: GetTopNQueuedEmails :many
SELECT *
FROM queued_emails
WHERE status = 'pending'
ORDER BY created_at DESC
LIMIT $1::INT;

-- name: InsertQueuedEmailHeader :exec
INSERT INTO queued_email_headers (
	email, name, value
) VALUES (
	$1, $2, $3
);

-- name: GetQueuedEmailHeaders :many
SELECT name, value
FROM queued_email_headers
WHERE email = $1;
