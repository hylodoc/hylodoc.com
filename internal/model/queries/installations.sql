-- name: CreateInstallation :one
INSERT INTO installations (
	gh_installation_id, user_id
) VALUES (
	$1, $2
)
RETURNING *;

-- name: GetInstallationWithGithubInstallationID :one
SELECT *
FROM installations
WHERE gh_installation_id = $1 AND active = true;

-- name: GetInstallationsForUser :many
SELECT *
FROM installations
WHERE user_id = $1 AND active = true;

-- name: DeleteInstallationWithGithubInstallationID :exec
DELETE
FROM installations
WHERE gh_installation_id = $1;
