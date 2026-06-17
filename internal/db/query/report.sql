-- name: GetRevenueReport :one
SELECT COALESCE(SUM(amount), 0)::bigint AS total_revenue
FROM public.payments
WHERE status = 'SUCCESS'
  AND ($1::timestamptz IS NULL OR created_at >= $1)
  AND ($2::timestamptz IS NULL OR created_at < $2);

-- name: ListProductsSoldReport :many
SELECT
    oi.product_id,
    oi.product_name,
    COALESCE(SUM(oi.quantity), 0)::integer AS quantity_sold
FROM public.order_items oi
JOIN public.orders o ON o.id = oi.order_id
WHERE o.status IN ('CONFIRMED', 'COMPLETED')
  AND EXISTS (
      SELECT 1
      FROM public.payments p
      WHERE p.order_id = o.id
        AND p.status = 'SUCCESS'
        AND ($1::timestamptz IS NULL OR p.created_at >= $1)
        AND ($2::timestamptz IS NULL OR p.created_at < $2)
  )
GROUP BY oi.product_id, oi.product_name
ORDER BY quantity_sold DESC, oi.product_name ASC;
