-- name: NewUser :one
INSERT INTO installations DEFAULT VALUES
RETURNING id;

-- name: ValidateUserId :one
SELECT EXISTS (
    SELECT 1
    FROM installations
    WHERE id = sqlc.arg(installation_id)
      AND disabled_at IS NULL
);
