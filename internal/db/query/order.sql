-- name: CreateOrder :one
INSERT INTO public.orders (
    order_number,
    user_id,
    status,
    notes,
    total_amount,
    expires_at
) VALUES (
    $1, $2, $3, $4, $5, $6
)
RETURNING *;

-- name: CreateOrderItem :one
INSERT INTO public.order_items (
    order_id,
    product_id,
    cart_item_id,
    product_name,
    price_at_checkout,
    quantity,
    subtotal,
    selected_attributes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: GetOrderByID :one
SELECT *
FROM public.orders
WHERE id = $1;

-- name: ListOrdersByUserID :many
SELECT *
FROM public.orders
WHERE user_id = $1
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: ListOrderItemsByOrderID :many
SELECT *
FROM public.order_items
WHERE order_id = $1
ORDER BY created_at ASC;

-- name: UpdateOrderStatus :one
UPDATE public.orders
SET status = $2,
    expires_at = CASE WHEN $2 = 'PENDING'::public.order_status THEN expires_at ELSE NULL END
WHERE id = $1
RETURNING *;

