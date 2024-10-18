-- name: CreateUnauthSession :one
INSERT INTO unauth_sessions (
	expires_at
) VALUES (
	$1
)
RETURNING *;

-- name: GetUnauthSession :one
SELECT *
FROM unauth_sessions
WHERE id = $1 AND active = true
LIMIT 1;

-- name: ExtendUnauthSession :exec
UPDATE unauth_sessions
SET expires_at = $1
WHERE id = $2 AND active = true;

-- name: EndUnauthSession :exec
UPDATE unauth_sessions
SET active = false
WHERE id = $1;
