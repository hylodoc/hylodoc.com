-- name: CreateStripeSubscription :one
INSERT INTO stripe_subscriptions (
	user_id, stripe_subscription_id, stripe_customer_id, stripe_price_id, status, current_period_start, current_period_end
) VALUES (
	$1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: StripeSubscriptionExists :one
SELECT EXISTS (
	SELECT 1
	FROM stripe_subscriptions
	WHERE stripe_subscription_id = $1
) AS stripe_subscription_exists;

-- name: UpdateStripeSubscription :one
UPDATE stripe_subscriptions
SET
	stripe_price_id = $1,
	status = $2,
	current_period_start = $3,
	current_period_end = $4,
	updated_at = now()
WHERE stripe_subscription_id = $5
RETURNING *;
