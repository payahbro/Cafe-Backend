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

-- name: GetProductByID :one
SELECT *
FROM public.products
WHERE id = $1
  AND deleted_at IS NULL;

-- name: GetProductByNameCI :one
SELECT *
FROM public.products
WHERE LOWER(name) = LOWER($1)
  AND deleted_at IS NULL;

-- name: ListProducts :many
SELECT *
FROM public.products
WHERE deleted_at IS NULL
ORDER BY created_at DESC
LIMIT $1 OFFSET $2;

-- name: UpdateProductStatus :one
UPDATE public.products
SET status = $2
WHERE id = $1
RETURNING *;

