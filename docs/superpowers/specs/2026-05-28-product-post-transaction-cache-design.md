# Product POST Transaction and Cache Invalidation Design

## Scope

This change updates only `POST /api/v1/products`.

It adds DB transaction handling and minimal product list cache invalidation after a product is created. It does not implement `PUT /products/:id`, product detail cache invalidation, or cache-aside reads for `GET /products`.

## Goals

- Ensure product creation runs inside one DB transaction.
- Keep duplicate-name check and insert in the same transaction.
- Invalidate stale product list cache after a successful commit.
- Keep product creation successful if Redis is unavailable or invalidation fails.
- Preserve the existing HTTP contract, validation behavior, and response body.

## Non-Goals

- No changes to GET list/detail caching behavior.
- No new product update endpoint.
- No hard dependency on Redis for product creation.
- No broad refactor outside the product create flow.

## Design

`ProductService` will receive two new optional collaborators:

- a transaction beginner, backed by `*pgxpool.Pool`
- a product cache invalidator, backed by Redis when Redis is configured

`CreateProduct` will:

1. Trim and normalize input as it does today.
2. Begin a DB transaction.
3. Use `repo.WithTx(tx)` for all DB calls inside the transaction.
4. Check product name uniqueness using `GetProductByNameCI`.
5. Insert using `CreateProduct`.
6. Commit the transaction.
7. After commit, invalidate `products:list:*`.
8. Return the created product even if cache invalidation fails.

Cache invalidation happens after commit because cache should only be cleared once the DB mutation is durable.

## Cache Invalidation

A small interface will be used:

```go
type ProductCacheInvalidator interface {
	InvalidateProductLists(ctx context.Context) error
}
```

The Redis implementation will scan keys matching `products:list:*` and delete them. It will use `SCAN`, not `KEYS`, to avoid blocking Redis on larger datasets.

If Redis is not configured, the service receives no cache invalidator and skips invalidation.

If Redis returns an error, the service ignores that error for the API result. The created product still returns `201`, matching the documented fallback policy that Redis failures must not break product responses.

## Error Behavior

Existing behavior stays the same:

- duplicate name returns `409 PRODUCT_NAME_ALREADY_EXISTS`
- DB or transaction errors return `500 INTERNAL_SERVER_ERROR`
- validation remains in the handler and returns `400 VALIDATION_ERROR`

Cache invalidation errors do not change the HTTP response.

## Files Expected To Change

- `internal/service/product.go`
- `internal/service/product_test.go`
- `internal/cache/product.go`
- `internal/http/router/router.go`

No API spec changes are required because the documented behavior already requires transaction-backed mutation and list cache invalidation for product create.

## Test Plan

- Unit test that `CreateProduct` uses transaction-backed repository calls.
- Unit test duplicate name path returns `ErrProductNameAlreadyExists`.
- Unit test cache invalidation is called after successful create.
- Unit test cache invalidation error does not fail create.
- Run `C:\sdk\go1.26.2\bin\go.exe test ./...`.

## Open Decisions

- Logging cache invalidation failures can be added later when product cache behavior becomes broader. For this minimal scope, Redis failure remains non-blocking and silent at service level unless an existing logger is introduced into the service.
