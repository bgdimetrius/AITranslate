-- name: SaveApplicationFeedback :exec
INSERT INTO application_feedback (installation_id, rating, text)
VALUES (
    sqlc.arg(installation_id),
    sqlc.arg(rating),
    sqlc.arg(feedback_text)
);

-- name: SaveFeedbackLead :exec
INSERT INTO feedback_leads (installation_id, email)
VALUES (
    sqlc.arg(installation_id),
    sqlc.arg(email)
);

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
