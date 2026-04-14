# CafeTelkom — Backend Service (Golang)

Backend service untuk aplikasi cafe (single-branch store) yang mengimplementasikan business rules untuk modul:
**User/Auth, Product, Cart, Order, Payment, Back Office**.

Dokumentasi di repo ini bersifat **docs-first**: README ini merangkum gambaran besar, sedangkan detail aturan & kontrak ada di folder `docs/`.


## Gambaran Sistem

Sistem menggunakan kombinasi:

- **Supabase** untuk Authentication, PostgreSQL Database, dan Storage (asset avatar / image produk)
- **Golang API Service** untuk business logic & enforcement rules (JWT validation, role validation, `is_active`)
- **Redis** untuk caching selektif (Product & Reporting)
- **Midtrans** untuk payment gateway (Snap + webhook + refund)

## Modul & Tanggung Jawab

- **User/Auth**: register via API, sebagian auth flow via Supabase (login/logout/reset/refresh); enforcement `is_active` di API.
- **Product**: CRUD + status & soft delete/restore; public read-only; cache list/detail.
- **Cart**: 1 user customer = 1 cart persisten; real-time (tanpa cache).
- **Order**: checkout dari cart, lifecycle `PENDING -> CONFIRMED -> COMPLETED/CANCELLED`, auto-expire.
- **Payment**: initiate Snap, webhook Midtrans, refund; idempotent webhook; sync ke order via internal endpoint.
- **Back Office**: operasional & reporting; banyak endpoint merupakan irisan modul lain (Order/Product/User).

## Aktor, Akses, dan Keamanan

Ringkasan dari dokumen BR & API spec:

- **Customer**: resource milik sendiri (profil, cart, order, payment).
- **Pegawai**: operasional terbatas (mis. `CONFIRMED -> COMPLETED`).
- **Admin**: akses penuh + refund + reporting.
- **Internal service-to-service**: endpoint internal menggunakan header **`X-Internal-Api-Key`** (bukan JWT user).
- Semua endpoint bisnis protected memverifikasi JWT Supabase + cek `public.users.is_active = true`.

## Data Model (ERD)

ERD dapat dilihat di `docs/erd/erd-backend-service.md` (Mermaid).

Entitas inti:
- `users` (profil + role + status bisnis) & relasi ke `auth.users`
- `products` (soft delete, status, attributes)
- `carts`, `cart_items`
- `orders`, `order_items` (snapshot saat checkout)
- `payments`, `payment_refunds`
- `outbox_events` (reliability: retry/DLQ)

## Architecture & Tech Stack

Ringkasan stack:
- **Go** + **Gin** (`github.com/gin-gonic/gin`)
- PostgreSQL Supabase + **pgx/v5**, SQL-first + **sqlc** (typed query) + migrations
- Redis (`go-redis/v9`) untuk cache selektif
- Midtrans (Snap, webhook, refund)
- Observability: structured logging (`zap`)

## API Specs (Index)

Semua API Golang berada di base path ` /api/v1 `.

### Konvensi Auth (ringkas)

- Endpoint protected wajib header: `Authorization: Bearer <supabase_jwt>`
- Internal service endpoint wajib header: `X-Internal-Api-Key: <secret>` (tanpa JWT user)

### Daftar Endpoint

#### Health

- `GET /health`
- `GET /api/v1/health`

#### User/Auth

Endpoint via Golang API:

- `POST /auth/register`
- `GET /users/profile`
- `PATCH /users/profile`

Catatan: Login/logout/refresh/reset password dilakukan langsung via Supabase (frontend).

#### Product

- `GET /products`
- `GET /products/:id`
- `POST /products` (Admin)
- `PUT /products/:id` (Admin)
- `PATCH /products/:id/status` (Pegawai/Admin)
- `DELETE /products/:id` (Admin, soft delete)
- `PATCH /products/:id/restore` (Admin)

#### Cart

- `GET /cart` (Customer)
- `POST /cart/items` (Customer)
- `PATCH /cart/items/:item_id` (Customer)
- `DELETE /cart/items/:item_id` (Customer)
- `DELETE /cart/items` (Customer, clear all items)
- `DELETE /internal/cart/items` (Internal: Order Service → Cart Service)

#### Order

- `POST /orders/checkout` (Customer)
- `GET /orders` (Customer: own, Pegawai/Admin: all)
- `GET /orders/:order_id` (Customer: own, Pegawai/Admin: all)
- `PATCH /orders/:order_id/cancel` (Customer: own, Admin)
- `PATCH /orders/:order_id/status` (Pegawai: hanya `CONFIRMED->COMPLETED`, Admin: sesuai rules)
- `PATCH /internal/orders/:order_id/status` (Internal: Payment Service → Order Service)

#### Payment

- `POST /payments/initiate` (Customer)
- `POST /payments/webhook` (Midtrans → Service; public tanpa JWT, wajib signature validation)
- `GET /payments/order/:order_id` (Customer: own, Admin)
- `GET /payments` (Admin)
- `GET /payments/me` (Customer)
- `POST /payments/:payment_id/refund` (Admin)

#### Back Office

Back Office pada fase ini memanfaatkan endpoint modul lain (irisan), misalnya:

- Order management: gunakan endpoint Order (`GET /orders`, `GET /orders/:order_id`, `PATCH /orders/:order_id/status`, `PATCH /orders/:order_id/cancel`)
- Product management: gunakan endpoint Product
- Reporting/export: (akan mengikuti implementasi modul Back Office di service)



## Struktur Repository

Mengacu ke rekomendasi di dokumen arsitektur:

```text
/cmd
  /api              # entrypoint API
  /worker           # entrypoint worker (outbox, retry, scheduler)
/internal
  /app              # wiring/bootstrapping
  /config           # config .env
  /http             # handler, middleware, dto, router
  /service          # business logic
  /repository       # db access layer (sqlc + wrapper)
  /db
    /query          # .sql untuk sqlc
    /migrations     # migration SQL
  /integrations
    /midtrans
    /supabase
  /outbox
  /scheduler
  /cache
  /logger
```

## Menjalankan Service

### Prasyarat

- Go (sesuai `go.mod`)
- Akses ke **Supabase PostgreSQL** (biasanya via session pooler) + Supabase Auth/Storage
- Redis (local via compose atau managed)

### Quick Start (Local)

1) Copy env:

```bash
cp .env.example .env
```

2) Isi variabel penting di `.env` (minimal):

- koneksi DB Supabase (URL/host/user/password sesuai template)
- Redis address (opsional jika tidak dipakai)
- secret untuk internal call: `X-Internal-Api-Key`
- credential Midtrans (untuk modul Payment)

> Jangan commit `.env` ke version control.

3) Run API:

```bash
go run ./cmd/api
```

4) Check health:

```bash
curl http://localhost:8080/health
```

### Run dengan Docker Compose

```bash
docker compose up --build
```

## Catatan Operasional

- Supabase adalah dependency eksternal (bukan Postgres container lokal).
- Set `DB_REQUIRED=true` dan/atau `REDIS_REQUIRED=true` untuk mode **fail-fast** saat dependency wajib tidak tersedia.
- Untuk flow Payment → Order dan Order → Cart clearing, gunakan endpoint internal yang memerlukan `X-Internal-Api-Key`.

