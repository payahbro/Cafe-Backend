-- name: GetUserById :one
SELECT
    id::text,
    email,
    full_name,
    role,
    is_verified,
    is_active,
    avatar_url,
    phone_number
FROM public.users
WHERE id = $1;

-- name: UpdateUserProfile :one
UPDATE public.users
SET
    full_name = COALESCE(sqlc.narg('full_name'), full_name),
    phone_number = COALESCE(sqlc.narg('phone_number'), phone_number),
    avatar_url = COALESCE(sqlc.narg('avatar_url'), avatar_url)
WHERE id = sqlc.arg('id')
RETURNING
    id::text,
    email,
    full_name,
    role,
    is_verified,
    is_active,
    avatar_url,
    phone_number;
