-- name: CreatePayment :one
INSERT INTO public.payments (
    id,
    order_id,
    status,
    amount,
    midtrans_order_id,
    snap_redirect_url
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: GetPaymentByID :one
SELECT *
FROM public.payments
WHERE id = $1;

-- name: GetActivePaymentByOrderID :one
SELECT *
FROM public.payments
WHERE order_id = $1
  AND status = 'PENDING_PAYMENT'
ORDER BY created_at DESC
LIMIT 1;

-- name: ListPaymentsByOrderID :many
SELECT *
FROM public.payments
WHERE order_id = $1
ORDER BY created_at DESC;

-- name: UpdatePaymentAfterWebhook :one
UPDATE public.payments
SET status = $2::public.payment_status,
    payment_method = $3,
    midtrans_transaction_id = $4,
    snap_redirect_url = CASE
        WHEN $2::public.payment_status IN (
            'SUCCESS'::public.payment_status,
            'FAILED'::public.payment_status,
            'EXPIRED'::public.payment_status,
            'REFUNDED'::public.payment_status
        ) THEN NULL
        ELSE snap_redirect_url
    END
WHERE id = $1
RETURNING *;

-- name: CreatePaymentRefund :one
INSERT INTO public.payment_refunds (
    payment_id,
    midtrans_refund_id,
    amount,
    reason,
    created_by
) VALUES (
    $1, $2, $3, $4, $5
)
RETURNING *;

-- name: MarkPaymentRefunded :one
UPDATE public.payments
SET status = 'REFUNDED',
    refund_amount = $2,
    refund_reason = $3,
    refunded_at = NOW()
WHERE id = $1
RETURNING *;

