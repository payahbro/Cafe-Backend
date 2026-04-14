CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TYPE public.user_role AS ENUM ('CUSTOMER', 'PEGAWAI', 'ADMIN');
CREATE TYPE public.product_category AS ENUM ('coffee', 'food', 'snack');
CREATE TYPE public.product_status AS ENUM ('available', 'out_of_stock', 'unavailable');
CREATE TYPE public.order_status AS ENUM ('PENDING', 'CONFIRMED', 'COMPLETED', 'CANCELLED');
CREATE TYPE public.payment_status AS ENUM ('PENDING_PAYMENT', 'SUCCESS', 'FAILED', 'EXPIRED', 'REFUNDED');
CREATE TYPE public.outbox_status AS ENUM ('PENDING', 'PROCESSING', 'RETRY', 'SENT', 'DEAD');

CREATE TABLE public.users (
    id UUID PRIMARY KEY,
    email TEXT NOT NULL,
    full_name VARCHAR(50) NOT NULL,
    role public.user_role NOT NULL DEFAULT 'CUSTOMER',
    is_verified BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    avatar_url TEXT,
    phone_number VARCHAR(20),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT users_email_unique UNIQUE (email),
    CONSTRAINT users_auth_user_fk FOREIGN KEY (id) REFERENCES auth.users(id)
);

CREATE TABLE public.products (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(100) NOT NULL,
    description TEXT,
    price INTEGER NOT NULL,
    category public.product_category NOT NULL,
    status public.product_status NOT NULL DEFAULT 'available',
    image_url TEXT,
    attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    stock INTEGER NOT NULL DEFAULT 0,
    rating NUMERIC(3,2) NOT NULL DEFAULT 0,
    total_sold INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ,
    CONSTRAINT products_price_non_negative CHECK (price >= 0),
    CONSTRAINT products_stock_non_negative CHECK (stock >= 0),
    CONSTRAINT products_rating_valid CHECK (rating >= 0 AND rating <= 5),
    CONSTRAINT products_total_sold_non_negative CHECK (total_sold >= 0)
);

CREATE TABLE public.carts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT carts_user_unique UNIQUE (user_id),
    CONSTRAINT carts_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE
);

CREATE TABLE public.cart_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    cart_id UUID NOT NULL,
    product_id UUID NOT NULL,
    quantity INTEGER NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT cart_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT cart_items_cart_product_unique UNIQUE (cart_id, product_id),
    CONSTRAINT cart_items_cart_fk FOREIGN KEY (cart_id) REFERENCES public.carts(id) ON DELETE CASCADE,
    CONSTRAINT cart_items_product_fk FOREIGN KEY (product_id) REFERENCES public.products(id)
);

CREATE TABLE public.orders (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_number VARCHAR(30) NOT NULL,
    user_id UUID NOT NULL,
    status public.order_status NOT NULL DEFAULT 'PENDING',
    notes TEXT,
    total_amount INTEGER NOT NULL,
    expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT orders_order_number_unique UNIQUE (order_number),
    CONSTRAINT orders_total_amount_non_negative CHECK (total_amount >= 0),
    CONSTRAINT orders_user_fk FOREIGN KEY (user_id) REFERENCES public.users(id)
);

CREATE TABLE public.order_items (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    product_id UUID NOT NULL,
    cart_item_id UUID,
    product_name VARCHAR(100) NOT NULL,
    price_at_checkout INTEGER NOT NULL,
    quantity INTEGER NOT NULL,
    subtotal INTEGER NOT NULL,
    selected_attributes JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT order_items_price_non_negative CHECK (price_at_checkout >= 0),
    CONSTRAINT order_items_quantity_positive CHECK (quantity > 0),
    CONSTRAINT order_items_subtotal_non_negative CHECK (subtotal >= 0),
    CONSTRAINT order_items_order_fk FOREIGN KEY (order_id) REFERENCES public.orders(id) ON DELETE CASCADE,
    CONSTRAINT order_items_product_fk FOREIGN KEY (product_id) REFERENCES public.products(id)
);

CREATE TABLE public.payments (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    status public.payment_status NOT NULL DEFAULT 'PENDING_PAYMENT',
    amount INTEGER NOT NULL,
    payment_method VARCHAR(50),
    midtrans_order_id VARCHAR(100) NOT NULL,
    midtrans_transaction_id VARCHAR(100),
    snap_redirect_url TEXT,
    refund_amount INTEGER,
    refund_reason TEXT,
    refunded_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payments_midtrans_order_unique UNIQUE (midtrans_order_id),
    CONSTRAINT payments_amount_non_negative CHECK (amount >= 0),
    CONSTRAINT payments_refund_amount_valid CHECK (refund_amount IS NULL OR refund_amount >= 0),
    CONSTRAINT payments_order_fk FOREIGN KEY (order_id) REFERENCES public.orders(id)
);

CREATE TABLE public.payment_refunds (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    payment_id UUID NOT NULL,
    midtrans_refund_id VARCHAR(100),
    amount INTEGER NOT NULL,
    reason TEXT NOT NULL,
    created_by UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT payment_refunds_payment_unique UNIQUE (payment_id),
    CONSTRAINT payment_refunds_amount_positive CHECK (amount > 0),
    CONSTRAINT payment_refunds_payment_fk FOREIGN KEY (payment_id) REFERENCES public.payments(id),
    CONSTRAINT payment_refunds_created_by_fk FOREIGN KEY (created_by) REFERENCES public.users(id)
);

CREATE TABLE public.outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type TEXT NOT NULL,
    aggregate_id UUID NOT NULL,
    event_type TEXT NOT NULL,
    payload JSONB NOT NULL,
    status public.outbox_status NOT NULL DEFAULT 'PENDING',
    retry_count INTEGER NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ,
    last_error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT outbox_events_retry_count_non_negative CHECK (retry_count >= 0)
);

CREATE UNIQUE INDEX idx_products_name_lower_unique ON public.products (LOWER(name));
CREATE INDEX idx_products_status_deleted_at ON public.products (status, deleted_at);
CREATE INDEX idx_products_category_status_deleted_at ON public.products (category, status, deleted_at);

CREATE INDEX idx_cart_items_cart_id ON public.cart_items (cart_id);
CREATE INDEX idx_cart_items_product_id ON public.cart_items (product_id);

CREATE INDEX idx_orders_user_created_at ON public.orders (user_id, created_at DESC);
CREATE INDEX idx_orders_status_created_at ON public.orders (status, created_at DESC);
CREATE INDEX idx_orders_expires_at_pending ON public.orders (expires_at) WHERE status = 'PENDING';

CREATE INDEX idx_order_items_order_id ON public.order_items (order_id);
CREATE INDEX idx_order_items_product_id ON public.order_items (product_id);
CREATE INDEX idx_order_items_cart_item_id ON public.order_items (cart_item_id);

CREATE INDEX idx_payments_order_created_at ON public.payments (order_id, created_at DESC);
CREATE INDEX idx_payments_status_created_at ON public.payments (status, created_at DESC);
CREATE INDEX idx_payments_midtrans_transaction_id ON public.payments (midtrans_transaction_id);

CREATE INDEX idx_payment_refunds_created_by ON public.payment_refunds (created_by);

CREATE INDEX idx_outbox_status_next_retry ON public.outbox_events (status, next_retry_at);
CREATE INDEX idx_outbox_aggregate ON public.outbox_events (aggregate_type, aggregate_id);
CREATE INDEX idx_outbox_created_at ON public.outbox_events (created_at);

CREATE OR REPLACE FUNCTION public.set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_users_set_updated_at
BEFORE UPDATE ON public.users
FOR EACH ROW
EXECUTE FUNCTION public.set_updated_at();

CREATE TRIGGER trg_products_set_updated_at
BEFORE UPDATE ON public.products
FOR EACH ROW
EXECUTE FUNCTION public.set_updated_at();

CREATE TRIGGER trg_carts_set_updated_at
BEFORE UPDATE ON public.carts
FOR EACH ROW
EXECUTE FUNCTION public.set_updated_at();

CREATE TRIGGER trg_cart_items_set_updated_at
BEFORE UPDATE ON public.cart_items
FOR EACH ROW
EXECUTE FUNCTION public.set_updated_at();

CREATE TRIGGER trg_orders_set_updated_at
BEFORE UPDATE ON public.orders
FOR EACH ROW
EXECUTE FUNCTION public.set_updated_at();

CREATE TRIGGER trg_payments_set_updated_at
BEFORE UPDATE ON public.payments
FOR EACH ROW
EXECUTE FUNCTION public.set_updated_at();

CREATE TRIGGER trg_outbox_events_set_updated_at
BEFORE UPDATE ON public.outbox_events
FOR EACH ROW
EXECUTE FUNCTION public.set_updated_at();

