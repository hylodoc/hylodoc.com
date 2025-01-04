-- name: DeleteReservedSubdomains :exec
DELETE FROM reserved_subdomains;

-- name: ReserveSubdomain :exec
INSERT INTO reserved_subdomains (
	subdomain
) VALUES (
	$1
);
