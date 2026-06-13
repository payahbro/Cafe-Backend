-- name: GetCartByUserID :one
SELECT *
FROM public.carts
WHERE user_id = $1;

-- name: ListCartItemsByCartID :many
SELECT
    ci.id AS item_id,
    ci.product_id,
    p.name,
    p.image_url,
    p.price,
    ci.quantity,
    p.status,
    p.deleted_at
FROM public.cart_items ci
JOIN public.products p ON p.id = ci.product_id
WHERE ci.cart_id = $1
ORDER BY ci.created_at ASC, ci.id ASC;

-- name: CreateCart :one
INSERT INTO public.carts (user_id)
VALUES ($1)
RETURNING *;

-- name: AddOrIncrementCartItem :one
INSERT INTO public.cart_items (cart_id, product_id, quantity)
VALUES ($1, $2, $3)
ON CONFLICT (cart_id, product_id)
DO UPDATE SET quantity = public.cart_items.quantity + EXCLUDED.quantity
RETURNING *;

-- name: UpdateCartItemQuantity :one
UPDATE public.cart_items
SET quantity = $2
WHERE id = $1
RETURNING *;

-- name: UpdateCartItemQuantityForUser :one
UPDATE public.cart_items ci
SET quantity = $2,
    updated_at = NOW()
FROM public.carts c
WHERE ci.id = $1
  AND ci.cart_id = c.id
  AND c.user_id = $3
RETURNING ci.*;

-- name: DeleteCartItemByID :exec
DELETE FROM public.cart_items
WHERE id = $1;

-- name: DeleteCartItemForUser :one
DELETE FROM public.cart_items ci
USING public.carts c
WHERE ci.id = $1
  AND ci.cart_id = c.id
  AND c.user_id = $2
RETURNING ci.cart_id;

-- name: DeleteCartItemsByCartID :exec
DELETE FROM public.cart_items
WHERE cart_id = $1;

-- name: DeleteCartItemsByIDs :exec
DELETE FROM public.cart_items
WHERE id = ANY($1::uuid[]);

-- name: TouchCart :exec
UPDATE public.carts
SET updated_at = NOW()
WHERE id = $1;

