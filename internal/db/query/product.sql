-- name: CreateProduct :one
INSERT INTO public.products (
    name,
    description,
    price,
    category,
    status,
    image_url,
    attributes,
    stock
) VALUES (
    $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING *;

-- name: CreateProductWithDefaultStock :one
INSERT INTO public.products (
    name,
    description,
    price,
    category,
    status,
    image_url,
    attributes
) VALUES (
    $1, $2, $3, $4, $5, $6, $7
)
RETURNING *;

-- name: GetProductByID :one
SELECT *
FROM public.products
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetProductByIDIncludingDeleted :one
SELECT *
FROM public.products
WHERE id = $1;

-- name: GetProductByNameCI :one
SELECT *
FROM public.products
WHERE LOWER(name) = LOWER($1)
  AND deleted_at IS NULL;

-- name: ListProducts :many
SELECT *
FROM public.products
WHERE ($3::boolean OR deleted_at IS NULL)
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateProduct :one
UPDATE public.products
SET
    name = $2,
    description = $3,
    price = $4,
    category = $5,
    status = $6,
    image_url = $7,
    attributes = $8
WHERE id = $1
  AND deleted_at IS NULL
RETURNING *;

-- name: UpdateProductStatus :one
UPDATE public.products
SET status = $2
WHERE id = $1
  AND deleted_at IS NULL
RETURNING *;

-- name: SoftDeleteProduct :one
UPDATE public.products
SET
    status = 'unavailable',
    deleted_at = NOW()
WHERE id = $1
  AND deleted_at IS NULL
RETURNING *;

-- name: RestoreProduct :one
UPDATE public.products
SET
    status = 'available',
    deleted_at = NULL
WHERE id = $1
  AND deleted_at IS NOT NULL
RETURNING *;
