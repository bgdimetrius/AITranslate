-- name: SaveApplicationFeedback :exec
INSERT INTO application_feedback (
    installation_id,
    rating,
    text
)
VALUES (
    sqlc.arg(installation_id),
    sqlc.arg(rating),
    sqlc.arg(feedback_text)
)
ON CONFLICT (installation_id) DO UPDATE
SET
    rating = EXCLUDED.rating,
    text   = EXCLUDED.text
WHERE
    application_feedback.rating IS DISTINCT FROM EXCLUDED.rating
    OR application_feedback.text IS DISTINCT FROM EXCLUDED.text;

-- name: SaveFeedbackLead :one
WITH inserted AS (
    INSERT INTO feedback_leads (installation_id, email)
    VALUES (
        sqlc.arg(installation_id),
        sqlc.arg(email)
    )
    ON CONFLICT (installation_id, email) DO NOTHING
    RETURNING TRUE AS created
)
SELECT EXISTS (SELECT 1 FROM inserted) AS created;

-- name: SaveTranslationFeedback :exec
INSERT INTO translation_feedback (
    installation_id,
    translation_id,
    revision,
    provider,
    source_lang,
    target_lang,
    source_text,
    translation_text,
    assessment
)
VALUES (
    sqlc.arg(installation_id),
    sqlc.arg(translation_id),
    sqlc.arg(revision),
    sqlc.arg(provider),
    sqlc.arg(source_lang),
    sqlc.arg(target_lang),
    sqlc.arg(source_text),
    sqlc.arg(translation_text),
    sqlc.arg(assessment)
)
ON CONFLICT (installation_id, translation_id)
DO UPDATE SET
    assessment = EXCLUDED.assessment,
    revision = EXCLUDED.revision,
    updated_at = NOW()
WHERE translation_feedback.revision < EXCLUDED.revision;
