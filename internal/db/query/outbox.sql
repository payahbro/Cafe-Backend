-- name: CreateOutboxEvent :one
INSERT INTO public.outbox_events (
    aggregate_type,
    aggregate_id,
    event_type,
    payload,
    status,
    retry_count,
    next_retry_at
) VALUES (
    $1, $2, $3, $4, 'PENDING', 0, NOW()
)
RETURNING *;

-- name: LockPendingOutboxEvents :many
SELECT *
FROM public.outbox_events
WHERE status IN ('PENDING', 'RETRY')
  AND (next_retry_at IS NULL OR next_retry_at <= NOW())
ORDER BY created_at ASC
LIMIT $1
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxProcessing :exec
UPDATE public.outbox_events
SET status = 'PROCESSING'
WHERE id = $1;

-- name: MarkOutboxSent :exec
UPDATE public.outbox_events
SET status = 'SENT',
    last_error = NULL,
    next_retry_at = NULL
WHERE id = $1;

-- name: MarkOutboxRetry :exec
UPDATE public.outbox_events
SET status = CASE WHEN $3 THEN 'DEAD' ELSE 'RETRY' END,
    retry_count = retry_count + 1,
    next_retry_at = $2,
    last_error = $4
WHERE id = $1;

