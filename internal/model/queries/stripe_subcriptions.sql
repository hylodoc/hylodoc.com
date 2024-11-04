-- name: CreateStripeSubscription :one
INSERT INTO stripe_subscriptions (
	user_id,
	sub_name,
	stripe_subscription_id,
	stripe_customer_id,
	stripe_price_id,
	amount,
	status,
	current_period_start,
	current_period_end
) VALUES (
	$1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING *;

-- name: DeactivateStripeSubscriptionByUserID :exec
UPDATE stripe_subscriptions
SET active = false
WHERE user_id = $1;

-- name: GetStripeSubscriptionByUserID :one
SELECT *
FROM stripe_subscriptions
WHERE user_id = $1 and active = true;

-- name: StripeSubscriptionExists :one
SELECT EXISTS (
	SELECT 1
	FROM stripe_subscriptions
	WHERE stripe_subscription_id = $1
) AS stripe_subscription_exists;
