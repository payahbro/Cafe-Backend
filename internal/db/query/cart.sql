-- name: GetCartByUserID :one
SELECT *
FROM public.carts
WHERE user_id = $1;

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

-- name: DeleteCartItemByID :exec
DELETE FROM public.cart_items
WHERE id = $1;

-- name: DeleteCartItemsByIDs :exec
DELETE FROM public.cart_items
WHERE id = ANY($1::uuid[]);

-- name: TouchCart :exec
UPDATE public.carts
SET updated_at = NOW()
WHERE id = $1;

