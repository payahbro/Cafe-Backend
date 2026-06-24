WITH grouped AS (
    SELECT
        cart_id,
        product_id,
        MIN(id::text)::uuid AS keep_id,
        SUM(quantity)::integer AS total_quantity
    FROM public.cart_items
    GROUP BY cart_id, product_id
),
updated AS (
    UPDATE public.cart_items ci
    SET quantity = grouped.total_quantity
    FROM grouped
    WHERE ci.id = grouped.keep_id
    RETURNING ci.id
)
DELETE FROM public.cart_items ci
USING grouped
WHERE ci.cart_id = grouped.cart_id
  AND ci.product_id = grouped.product_id
  AND ci.id <> grouped.keep_id;

ALTER TABLE public.cart_items
DROP CONSTRAINT cart_items_cart_product_attributes_unique;

ALTER TABLE public.cart_items
DROP COLUMN attributes_key,
DROP COLUMN selected_attributes;

ALTER TABLE public.cart_items
ADD CONSTRAINT cart_items_cart_product_unique UNIQUE (cart_id, product_id);
