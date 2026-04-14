DROP TRIGGER IF EXISTS trg_outbox_events_set_updated_at ON public.outbox_events;
DROP TRIGGER IF EXISTS trg_payments_set_updated_at ON public.payments;
DROP TRIGGER IF EXISTS trg_orders_set_updated_at ON public.orders;
DROP TRIGGER IF EXISTS trg_cart_items_set_updated_at ON public.cart_items;
DROP TRIGGER IF EXISTS trg_carts_set_updated_at ON public.carts;
DROP TRIGGER IF EXISTS trg_products_set_updated_at ON public.products;
DROP TRIGGER IF EXISTS trg_users_set_updated_at ON public.users;

DROP FUNCTION IF EXISTS public.set_updated_at();

DROP TABLE IF EXISTS public.outbox_events;
DROP TABLE IF EXISTS public.payment_refunds;
DROP TABLE IF EXISTS public.payments;
DROP TABLE IF EXISTS public.order_items;
DROP TABLE IF EXISTS public.orders;
DROP TABLE IF EXISTS public.cart_items;
DROP TABLE IF EXISTS public.carts;
DROP TABLE IF EXISTS public.products;
DROP TABLE IF EXISTS public.users;

DROP TYPE IF EXISTS public.outbox_status;
DROP TYPE IF EXISTS public.payment_status;
DROP TYPE IF EXISTS public.order_status;
DROP TYPE IF EXISTS public.product_status;
DROP TYPE IF EXISTS public.product_category;
DROP TYPE IF EXISTS public.user_role;

