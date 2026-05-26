INSERT INTO public.products (
    id,
    name,
    description,
    price,
    category,
    status,
    image_url,
    attributes,
    stock,
    rating,
    total_sold
) VALUES
(
    '11111111-1111-4111-8111-111111111111',
    'Americano',
    'Espresso dengan air panas, rasa kopi tegas dan ringan.',
    25000,
    'coffee',
    'available',
    'https://project-ref.supabase.co/storage/v1/object/public/products/americano.png',
    '{
        "temperature": ["hot", "iced"],
        "sugar_levels": ["normal", "less", "no_sugar"],
        "ice_levels": ["normal", "less", "no_ice"],
        "sizes": ["small", "medium", "large"]
    }'::jsonb,
    40,
    4.50,
    120
),
(
    '22222222-2222-4222-8222-222222222222',
    'Cafe Latte',
    'Espresso dengan susu steamed dan foam lembut.',
    30000,
    'coffee',
    'available',
    'https://project-ref.supabase.co/storage/v1/object/public/products/cafe-latte.png',
    '{
        "temperature": ["hot", "iced"],
        "sugar_levels": ["normal", "less", "no_sugar"],
        "ice_levels": ["normal", "less", "no_ice"],
        "sizes": ["small", "medium", "large"]
    }'::jsonb,
    35,
    4.70,
    98
),
(
    '33333333-3333-4333-8333-333333333333',
    'Chicken Katsu Rice',
    'Nasi hangat dengan chicken katsu renyah dan saus gurih.',
    38000,
    'food',
    'available',
    'https://project-ref.supabase.co/storage/v1/object/public/products/chicken-katsu-rice.png',
    '{
        "portions": ["regular", "large"],
        "spicy_levels": ["no_spicy", "mild", "medium"]
    }'::jsonb,
    25,
    4.60,
    85
),
(
    '44444444-4444-4444-8444-444444444444',
    'Nasi Goreng Spesial',
    'Nasi goreng dengan telur, ayam, dan bumbu khas cafe.',
    35000,
    'food',
    'out_of_stock',
    'https://project-ref.supabase.co/storage/v1/object/public/products/nasi-goreng-spesial.png',
    '{
        "portions": ["regular", "large"],
        "spicy_levels": ["no_spicy", "mild", "medium", "hot"]
    }'::jsonb,
    0,
    4.40,
    76
),
(
    '55555555-5555-4555-8555-555555555555',
    'French Fries',
    'Kentang goreng renyah dengan pilihan saus.',
    18000,
    'snack',
    'available',
    'https://project-ref.supabase.co/storage/v1/object/public/products/french-fries.png',
    '{
        "portions": ["regular", "large"],
        "spicy_levels": ["no_spicy", "mild"]
    }'::jsonb,
    50,
    4.30,
    140
)
ON CONFLICT ((LOWER(name))) DO UPDATE
SET
    description = EXCLUDED.description,
    price = EXCLUDED.price,
    category = EXCLUDED.category,
    status = EXCLUDED.status,
    image_url = EXCLUDED.image_url,
    attributes = EXCLUDED.attributes,
    stock = EXCLUDED.stock,
    rating = EXCLUDED.rating,
    total_sold = EXCLUDED.total_sold,
    deleted_at = NULL;
