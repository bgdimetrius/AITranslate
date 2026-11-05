-- name: RecordDailyUsage :exec
INSERT INTO daily_usage (
    installation_id,
    translation_requests,
    interpretation_requests,
    failed_requests,
    input_characters,
    input_tokens,
    output_tokens
)
VALUES (
    sqlc.arg(installation_id),
    sqlc.arg(translation_requests),
    sqlc.arg(interpretation_requests),
    sqlc.arg(failed_requests),
    sqlc.arg(input_characters),
    sqlc.arg(input_tokens),
    sqlc.arg(output_tokens)
)
ON CONFLICT (installation_id, usage_date)
DO UPDATE SET
    translation_requests =
        daily_usage.translation_requests + EXCLUDED.translation_requests,
    interpretation_requests =
        daily_usage.interpretation_requests + EXCLUDED.interpretation_requests,
    failed_requests =
        daily_usage.failed_requests + EXCLUDED.failed_requests,
    input_characters =
        daily_usage.input_characters + EXCLUDED.input_characters,
    input_tokens =
        daily_usage.input_tokens + EXCLUDED.input_tokens,
    output_tokens =
        daily_usage.output_tokens + EXCLUDED.output_tokens;
