-- +goose Up

ALTER TABLE feedback_leads
    ADD CONSTRAINT feedback_leads_installation_id_email_key
    UNIQUE (installation_id, email);

CREATE UNIQUE INDEX application_feedback_installation_id_uidx
    ON application_feedback (installation_id);
