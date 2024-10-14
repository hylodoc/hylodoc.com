-- name: CreateInstallation :one
INSERT INTO installations (
	gh_installation_id, user_id
) VALUES (
	$1, $2
)
RETURNING *;

-- name: InstallationExistsForUserID :one
SELECT EXISTS (
	SELECT 1
	FROM installations
	WHERE user_id = $1
) AS installation_exists;

-- name: GetInstallationByGithubInstallationID :one
SELECT *
FROM installations
WHERE gh_installation_id = $1 AND active = true;

-- name: ListInstallationsForUser :many
SELECT *
FROM installations
WHERE user_id = $1 AND active = true;

-- name: DeleteInstallationWithGithubInstallationID :exec
DELETE
FROM installations
WHERE gh_installation_id = $1;
