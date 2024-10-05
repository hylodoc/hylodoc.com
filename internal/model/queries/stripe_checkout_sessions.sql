-- name: CreateStripeCheckoutSession :one
INSERT INTO stripe_checkout_sessions (
	stripe_session_id, user_id
) VALUES (
	$1, $2
)
RETURNING *;

-- name: GetPendingStripeCheckoutSession :one
SELECT *
FROM stripe_checkout_sessions
WHERE stripe_session_id = $1 AND status = 'pending';

-- name: UpdateStripeCheckoutSession :one
UPDATE stripe_checkout_sessions
SET
	status = $1
WHERE
	stripe_session_id = $2
RETURNING *;

