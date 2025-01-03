-- name: CreateReservedSubdomain :one
INSERT INTO reserved_subdomains (
	subdomain
) VALUES (
	$1
)
RETURNING *;

-- name: ReservedSubdomainExists :one
SELECT EXISTS (
	SELECT 1
	FROM reserved_subdomains
	WHERE subdomain = $1
);
