-- +goose Up

CREATE TABLE installations (
    id BIGSERIAL PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    disabled_at TIMESTAMPTZ
);

CREATE TABLE auth (
    installation_id BIGINT PRIMARY KEY
        REFERENCES installations(id) ON DELETE CASCADE,
    refresh_token VARCHAR(64) NOT NULL
        CHECK (refresh_token ~ '^[0-9a-f]{64}$')
);

CREATE TABLE daily_usage (
    installation_id BIGINT NOT NULL
        REFERENCES installations(id) ON DELETE CASCADE,

    usage_date DATE NOT NULL DEFAULT CURRENT_DATE,

    translation_requests INTEGER NOT NULL DEFAULT 0
        CHECK (translation_requests >= 0),
    interpretation_requests INTEGER NOT NULL DEFAULT 0
        CHECK (interpretation_requests >= 0),
    failed_requests INTEGER NOT NULL DEFAULT 0
        CHECK (failed_requests >= 0),

    input_characters BIGINT NOT NULL DEFAULT 0
        CHECK (input_characters >= 0),
    input_tokens BIGINT NOT NULL DEFAULT 0
        CHECK (input_tokens >= 0),
    output_tokens BIGINT NOT NULL DEFAULT 0
        CHECK (output_tokens >= 0),

    PRIMARY KEY (installation_id, usage_date)
);

CREATE TABLE application_feedback (
    id BIGSERIAL PRIMARY KEY,
    installation_id BIGINT NOT NULL
        REFERENCES installations(id) ON DELETE CASCADE,
    rating SMALLINT NOT NULL CHECK (rating IN (-1, 1)),
    text TEXT NOT NULL DEFAULT '' CHECK (char_length(text) <= 128),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE feedback_leads (
    id BIGSERIAL PRIMARY KEY,
    installation_id BIGINT NOT NULL
        REFERENCES installations(id) ON DELETE CASCADE,
    email TEXT NOT NULL CHECK (
        char_length(email) BETWEEN 3 AND 320
        AND email = btrim(email)
    ),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
