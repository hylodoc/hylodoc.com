-- name: Boot :one
INSERT INTO boots DEFAULT VALUES
RETURNING id;
