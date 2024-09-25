-- name: CreateSession :one
INSERT INTO sessions (
	token, user_id
) VALUES (
	$1, $2
)
RETURNING *;

-- name: GetSession :one
SELECT s.user_id, u.email, u.username, s.token, s.expires_at
FROM sessions AS s
	INNER JOIN users AS u ON s.user_id = u.id
WHERE token = $1 AND active = true;

-- name: EndSession :exec
UPDATE sessions
SET active = false
WHERE token = $1;
