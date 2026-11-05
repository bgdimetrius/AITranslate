-- name: SaveSession :exec
INSERT INTO auth (installation_id, refresh_token, access_token_id)
VALUES (
    sqlc.arg(installation_id),
    sqlc.arg(refresh_token),
    sqlc.arg(access_token_id)
)
ON CONFLICT (installation_id)
DO UPDATE SET
    refresh_token = EXCLUDED.refresh_token,
    access_token_id = EXCLUDED.access_token_id;

-- name: RotateSession :execrows
UPDATE auth
SET
    refresh_token = sqlc.arg(new_refresh_token),
    access_token_id = sqlc.arg(new_access_token_id)
WHERE installation_id = sqlc.arg(installation_id)
  AND refresh_token = sqlc.arg(previous_refresh_token);

-- name: GetRefreshToken :one
SELECT refresh_token
FROM auth
WHERE installation_id = sqlc.arg(installation_id);

-- name: GetAccessTokenID :one
SELECT access_token_id
FROM auth
WHERE installation_id = sqlc.arg(installation_id);
