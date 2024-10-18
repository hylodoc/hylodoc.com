-- name: CreateAuthSession :one
INSERT INTO auth_sessions (
	user_id, expires_at
) VALUES (
	$1, $2
)
RETURNING *;

-- name: GetAuthSession :one
SELECT s.id, s.user_id, u.username, u.email, g.gh_email, s.expires_at
FROM auth_sessions AS s
INNER JOIN users AS u
	ON s.user_id = u.id
LEFT JOIN github_accounts AS g
	ON s.user_id = g.user_id
WHERE s.id = $1 AND active = true;

-- name: ExtendAuthSession :exec
UPDATE auth_sessions
SET expires_at = $1
WHERE id = $2 AND active = true;

-- name: EndAuthSession :exec
UPDATE auth_sessions
SET active = false
WHERE id = $1;
