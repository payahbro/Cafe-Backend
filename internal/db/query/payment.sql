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

-- name: ListPayments :many
SELECT
    p.id,
    p.order_id,
    o.order_number,
    o.user_id,
    p.status,
    p.amount,
    p.payment_method,
    p.midtrans_transaction_id,
    p.snap_redirect_url,
    p.refund_amount,
    p.refund_reason,
    p.refunded_at,
    p.created_at,
    p.updated_at
FROM public.payments p
JOIN public.orders o ON o.id = p.order_id
WHERE ($1::uuid IS NULL OR o.user_id = $1)
  AND ($2::uuid IS NULL OR p.order_id = $2)
  AND ($3::text = '' OR p.status = NULLIF($3, '')::public.payment_status)
  AND ($4::text = '' OR p.payment_method = $4)
ORDER BY p.created_at DESC, p.id DESC
LIMIT $5 OFFSET $6;

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

