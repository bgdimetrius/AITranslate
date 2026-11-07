-- +goose Up

CREATE TABLE translation_feedback (
    installation_id BIGINT NOT NULL
        REFERENCES installations(id) ON DELETE CASCADE,
    translation_id UUID NOT NULL,
    revision INTEGER NOT NULL CHECK (revision > 0),
    provider TEXT NOT NULL CHECK (provider <> '' AND provider = btrim(provider)),
    source_lang TEXT NOT NULL CHECK (source_lang = btrim(source_lang)),
    target_lang TEXT NOT NULL CHECK (target_lang <> '' AND target_lang = btrim(target_lang)),
    source_text TEXT NOT NULL CHECK (source_text <> ''),
    translation_text TEXT NOT NULL CHECK (translation_text <> ''),
    assessment TEXT NOT NULL CHECK (assessment IN ('like', 'dislike')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    PRIMARY KEY (installation_id, translation_id)
);
