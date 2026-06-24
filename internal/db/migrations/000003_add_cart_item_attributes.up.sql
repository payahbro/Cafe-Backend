ALTER TABLE public.cart_items
DROP CONSTRAINT cart_items_cart_product_unique;

ALTER TABLE public.cart_items
ADD COLUMN selected_attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
ADD COLUMN attributes_key VARCHAR(64) NOT NULL DEFAULT '';

ALTER TABLE public.cart_items
ADD CONSTRAINT cart_items_cart_product_attributes_unique
UNIQUE (cart_id, product_id, attributes_key);
