-- name: CreateStripeSubscription :one
INSERT INTO stripe_subscriptions (
	user_id, stripe_subscription_id, stripe_customer_id, stripe_price_id, amount, status, current_period_start, current_period_end
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetStripeSubscriptionByUserID :one
SELECT *
FROM stripe_subscriptions
WHERE user_id = $1;

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
	amount = $2,
	current_period_start = $3,
	current_period_end = $4,
	status = $5,
	updated_at = now()
WHERE stripe_subscription_id = $6
RETURNING *;
