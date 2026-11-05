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
