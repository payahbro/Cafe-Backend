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

-- name: ListCheckoutCartItemsForUser :many
SELECT
    ci.id AS cart_item_id,
    ci.product_id,
    p.name AS product_name,
    p.price,
    p.category,
    p.status,
    p.attributes,
    p.stock,
    p.deleted_at,
    ci.quantity
FROM public.cart_items ci
JOIN public.carts c ON c.id = ci.cart_id
JOIN public.products p ON p.id = ci.product_id
WHERE c.user_id = $1
  AND ci.id = ANY($2::uuid[])
ORDER BY ci.created_at ASC, ci.id ASC
FOR UPDATE OF ci;

-- name: CountPendingOrderItemsByCartItemIDs :one
SELECT COUNT(*)::bigint
FROM public.order_items oi
JOIN public.orders o ON o.id = oi.order_id
WHERE o.user_id = $1
  AND o.status = 'PENDING'
  AND oi.cart_item_id = ANY($2::uuid[]);

-- name: AcquireOrderNumberDateLock :exec
SELECT pg_advisory_xact_lock(hashtext($1));

-- name: CountOrdersByOrderNumberPrefix :one
SELECT COUNT(*)::bigint
FROM public.orders
WHERE order_number LIKE $1 || '%';

-- name: LockProductByIDForUpdate :one
SELECT *
FROM public.products
WHERE id = $1
FOR UPDATE;

-- name: LockOrderByIDForUpdate :one
SELECT *
FROM public.orders
WHERE id = $1
FOR UPDATE;

-- name: DecrementProductStock :one
UPDATE public.products
SET stock = stock - $2
WHERE id = $1
  AND stock >= $2
RETURNING *;

-- name: IncrementProductStock :one
UPDATE public.products
SET stock = stock + $2
WHERE id = $1
RETURNING *;

-- name: IncrementProductTotalSold :one
UPDATE public.products
SET total_sold = total_sold + $2
WHERE id = $1
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

-- name: ListOrders :many
SELECT
    id,
    order_number,
    user_id,
    status,
    total_amount,
    created_at
FROM public.orders
WHERE ($1::uuid IS NULL OR user_id = $1)
  AND ($2::text = '' OR status = NULLIF($2, '')::public.order_status)
ORDER BY created_at DESC, id DESC
LIMIT $3 OFFSET $4;

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

