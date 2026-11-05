-- +goose Up

ALTER TABLE auth
ADD COLUMN access_token_id VARCHAR(32);

-- Existing JWTs have no ID and must stop authorizing requests after rollout.
-- The placeholder lets their refresh tokens perform one authenticated rotation
-- into the new session format without preserving access-token validity.
UPDATE auth
SET access_token_id = md5(
    installation_id::text || refresh_token || clock_timestamp()::text
);

ALTER TABLE auth
ALTER COLUMN access_token_id SET NOT NULL;

ALTER TABLE auth
ADD CONSTRAINT auth_access_token_id_format
CHECK (access_token_id ~ '^[0-9a-f]{32}$');
