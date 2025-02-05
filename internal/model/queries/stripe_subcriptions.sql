-- name: CreateStripeSubscription :one
INSERT INTO stripe_subscriptions (
	user_id,
	sub_name,
	stripe_subscription_id,
	stripe_customer_id,
	stripe_status
) VALUES (
	$1, $2, $3, $4, $5
)
RETURNING *;

-- name: UpdateStripeSubscription :exec
UPDATE stripe_subscriptions
SET
	stripe_subscription_id = $1,
	sub_name = $2,
	stripe_status = $3,
	updated_at = now()
WHERE stripe_customer_id = $4;

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
