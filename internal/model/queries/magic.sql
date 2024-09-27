-- name: CreateMagicRegister :one
INSERT INTO magic (
	token, email, link_type
) VALUES (
	$1, $2, 'register'
)
RETURNING *;

-- name: GetMagicRegisterByToken :one
SELECT *
FROM magic
WHERE token = $1 AND link_type = 'register';

-- name: CreateMagicLogin :one
INSERT INTO magic (
	token, email, link_type
) VALUES (
	$1, $2, 'login'
)
RETURNING *;

-- name: GetMagicLoginByToken :one
SELECT *
FROM magic
WHERE token = $1 AND link_type = 'login';
