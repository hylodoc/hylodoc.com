-- name: CreateUnauthSession :one
INSERT INTO unauth_sessions (token) VALUES ($1)
RETURNING token;

-- name: GetUnauthSession :one
SELECT *
FROM unauth_sessions
WHERE token = $1 AND active = true
LIMIT 1;

-- name: EndUnauthSession :exec
UPDATE unauth_sessions SET active = false WHERE token = $1;
