# Product POST Transaction Cache Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make `POST /api/v1/products` run DB work inside a transaction and invalidate product list cache after a successful commit.

**Architecture:** Keep handler behavior unchanged. Add transaction/cache collaborators to `ProductService`, wrap create DB calls through a product transaction runner, and add a Redis-backed product cache invalidator that deletes `products:list:*` with `SCAN`.

**Tech Stack:** Go 1.26.2, Gin, pgx/v5, sqlc-generated repository, go-redis/v9, standard `testing` package.

---

## File Structure

- Modify `internal/service/product.go`: add product service options, transaction runner, cache invalidator interface, and transaction-backed `CreateProduct`.
- Modify `internal/service/product_test.go`: add fake transaction runner and cache invalidator tests for create flow.
- Create `internal/cache/product.go`: Redis-backed product list cache invalidator using `SCAN` and batched `DEL`.
- Modify `internal/http/router/router.go`: wire `dbPool` and Redis product cache into `ProductService`.

No handler, DTO, API spec, SQL, or generated repository files are required for this scope.

---

### Task 1: Add Transaction and Cache Tests for Product Create

**Files:**
- Modify: `internal/service/product_test.go`

- [ ] **Step 1: Replace the existing create success test with a transaction-aware test**

Replace `TestProductServiceCreateProductCreatesRow` with:

```go
func TestProductServiceCreateProductCreatesRowInTransactionAndInvalidatesListCache(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	baseRepo := &fakeProductRepo{}
	txRepo := &fakeProductRepo{
		getByNameErr: pgx.ErrNoRows,
		product:      productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}
	txRunner := &fakeProductTxRunner{repo: txRepo}
	cache := &fakeProductCacheInvalidator{}
	cache.txRunner = txRunner
	description := "Espresso dengan air panas"
	service := NewProductService(baseRepo, WithProductTxRunner(txRunner), WithProductCache(cache))

	product, err := service.CreateProduct(context.Background(), CreateProductInput{
		Name:        " Americano ",
		Description: &description,
		Price:       25000,
		Category:    "coffee",
		Status:      "available",
		ImageURL:    "https://example.supabase.co/storage/v1/object/public/products/americano.png",
		Attributes:  []byte(`{"temperature":["hot"],"sugar_levels":["normal"],"ice_levels":["normal"],"sizes":["medium"]}`),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	if !txRunner.called {
		t.Fatalf("expected transaction runner to be called")
	}
	if baseRepo.createCalled {
		t.Fatalf("base repository should not create product outside transaction")
	}
	if !txRepo.createCalled {
		t.Fatalf("expected transaction repository to create product")
	}
	if txRepo.createArg.Name != "Americano" {
		t.Fatalf("create name = %q", txRepo.createArg.Name)
	}
	if txRepo.createArg.Status != repository.ProductStatusAvailable {
		t.Fatalf("create status = %q", txRepo.createArg.Status)
	}
	if txRepo.createArg.Stock != 0 {
		t.Fatalf("create stock = %d", txRepo.createArg.Stock)
	}
	if product.Name != "Americano" {
		t.Fatalf("product name = %q", product.Name)
	}
	if !cache.invalidated {
		t.Fatalf("expected product list cache invalidation")
	}
	if cache.invalidatedBeforeTxDone {
		t.Fatalf("cache invalidation should run after transaction runner finishes")
	}
}
```

- [ ] **Step 2: Replace the duplicate-name test with a transaction-aware test**

Replace `TestProductServiceCreateProductRejectsDuplicateName` with:

```go
func TestProductServiceCreateProductRejectsDuplicateNameInTransaction(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	txRepo := &fakeProductRepo{
		product: productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}
	txRunner := &fakeProductTxRunner{repo: txRepo}
	cache := &fakeProductCacheInvalidator{}
	service := NewProductService(&fakeProductRepo{}, WithProductTxRunner(txRunner), WithProductCache(cache))

	_, err := service.CreateProduct(context.Background(), CreateProductInput{
		Name:       "Americano",
		Price:      25000,
		Category:   "coffee",
		ImageURL:   "https://example.supabase.co/storage/v1/object/public/products/americano.png",
		Attributes: []byte(`{"temperature":["hot"],"sugar_levels":["normal"],"ice_levels":["normal"],"sizes":["medium"]}`),
	})
	if !errors.Is(err, ErrProductNameAlreadyExists) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.createCalled {
		t.Fatalf("create should not be called for duplicate name")
	}
	if cache.invalidated {
		t.Fatalf("cache should not be invalidated when create fails")
	}
}
```

- [ ] **Step 3: Add a cache-error non-blocking test**

Add this test after the duplicate-name test:

```go
func TestProductServiceCreateProductIgnoresCacheInvalidationError(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	txRepo := &fakeProductRepo{
		getByNameErr: pgx.ErrNoRows,
		product:      productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}
	txRunner := &fakeProductTxRunner{repo: txRepo}
	cache := &fakeProductCacheInvalidator{err: errors.New("redis unavailable")}
	service := NewProductService(&fakeProductRepo{}, WithProductTxRunner(txRunner), WithProductCache(cache))

	product, err := service.CreateProduct(context.Background(), CreateProductInput{
		Name:       "Americano",
		Price:      25000,
		Category:   "coffee",
		ImageURL:   "https://example.supabase.co/storage/v1/object/public/products/americano.png",
		Attributes: []byte(`{"temperature":["hot"],"sugar_levels":["normal"],"ice_levels":["normal"],"sizes":["medium"]}`),
	})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	if product.Name != "Americano" {
		t.Fatalf("product name = %q", product.Name)
	}
	if !cache.invalidated {
		t.Fatalf("expected cache invalidation attempt")
	}
}
```

- [ ] **Step 4: Add test fakes**

Add these fakes near the existing `fakeProductRepo` type:

```go
type fakeProductTxRunner struct {
	repo   productRepository
	called bool
	done   bool
	err    error
}

func (f *fakeProductTxRunner) Run(ctx context.Context, fn func(productRepository) error) error {
	f.called = true
	if f.err != nil {
		f.done = true
		return f.err
	}
	err := fn(f.repo)
	f.done = true
	return err
}

type fakeProductCacheInvalidator struct {
	txRunner                *fakeProductTxRunner
	invalidated             bool
	invalidatedBeforeTxDone bool
	err                     error
}

func (f *fakeProductCacheInvalidator) InvalidateProductLists(ctx context.Context) error {
	f.invalidated = true
	if f.txRunner != nil && !f.txRunner.done {
		f.invalidatedBeforeTxDone = true
	}
	return f.err
}
```

- [ ] **Step 5: Run service tests and verify they fail for missing API**

Run:

```powershell
& 'C:\sdk\go1.26.2\bin\go.exe' test ./internal/service
```

Expected: FAIL with compile errors mentioning `WithProductTxRunner`, `WithProductCache`, or `productTxRunner` not defined.

---

### Task 2: Implement Product Service Transaction and Cache Hooks

**Files:**
- Modify: `internal/service/product.go`
- Test: `internal/service/product_test.go`

- [ ] **Step 1: Add imports**

Update the imports in `internal/service/product.go` to include the DB helper package:

```go
import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	cafedb "cafeTelkom/internal/db"
	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)
```

- [ ] **Step 2: Add product transaction/cache types and options**

Add this block after `type productRepository interface`:

```go
type productTxRunner interface {
	Run(ctx context.Context, fn func(productRepository) error) error
}

type ProductCacheInvalidator interface {
	InvalidateProductLists(ctx context.Context) error
}

type ProductServiceOption func(*ProductService)

type productTransactionRunner struct {
	db   cafedb.TxBeginner
	repo *repository.Queries
}

func WithProductTxRunner(txRunner productTxRunner) ProductServiceOption {
	return func(s *ProductService) {
		s.txRunner = txRunner
	}
}

func WithProductTransaction(db cafedb.TxBeginner, repo *repository.Queries) ProductServiceOption {
	return func(s *ProductService) {
		if db != nil && repo != nil {
			s.txRunner = productTransactionRunner{db: db, repo: repo}
		}
	}
}

func WithProductCache(cache ProductCacheInvalidator) ProductServiceOption {
	return func(s *ProductService) {
		s.cache = cache
	}
}

func (r productTransactionRunner) Run(ctx context.Context, fn func(productRepository) error) error {
	if r.db == nil || r.repo == nil {
		return errors.New("product transaction runner missing")
	}

	return cafedb.WithTx(ctx, r.db, func(ctx context.Context, tx pgx.Tx) error {
		return fn(r.repo.WithTx(tx))
	})
}
```

- [ ] **Step 3: Add fields and option handling to ProductService**

Replace the `ProductService` struct and constructor with:

```go
type ProductService struct {
	repo     productRepository
	txRunner productTxRunner
	cache    ProductCacheInvalidator
}

func NewProductService(repo productRepository, options ...ProductServiceOption) *ProductService {
	service := &ProductService{repo: repo}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}
```

- [ ] **Step 4: Replace CreateProduct with transaction-backed implementation**

Replace the whole `CreateProduct` method with:

```go
func (s *ProductService) CreateProduct(ctx context.Context, input CreateProductInput) (*Product, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("product transaction runner missing")
	}

	name := strings.TrimSpace(input.Name)
	status := input.Status
	if status == "" {
		status = string(repository.ProductStatusAvailable)
	}

	var row repository.Product
	err := s.txRunner.Run(ctx, func(repo productRepository) error {
		if _, err := repo.GetProductByNameCI(ctx, name); err == nil {
			return ErrProductNameAlreadyExists
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("check product name: %w", err)
		}

		created, err := repo.CreateProduct(ctx, repository.CreateProductParams{
			Name:        name,
			Description: optionalText(input.Description),
			Price:       input.Price,
			Category:    repository.ProductCategory(input.Category),
			Status:      repository.ProductStatus(status),
			ImageUrl:    pgtype.Text{String: input.ImageURL, Valid: input.ImageURL != ""},
			Attributes:  input.Attributes,
			Stock:       0,
		})
		if err != nil {
			if isUniqueViolation(err) {
				return ErrProductNameAlreadyExists
			}
			return fmt.Errorf("create product: %w", err)
		}

		row = created
		return nil
	})
	if err != nil {
		return nil, err
	}

	product := productFromRow(row)
	if s.cache != nil {
		_ = s.cache.InvalidateProductLists(ctx)
	}

	return &product, nil
}
```

- [ ] **Step 5: Run service tests and verify they pass**

Run:

```powershell
& 'C:\sdk\go1.26.2\bin\go.exe' test ./internal/service
```

Expected: PASS.

- [ ] **Step 6: Commit service transaction hooks**

Run:

```powershell
git add internal/service/product.go internal/service/product_test.go
git commit -m "feat: wrap product create in transaction"
```

---

### Task 3: Add Redis Product Cache Invalidator

**Files:**
- Create: `internal/cache/product.go`

- [ ] **Step 1: Create Redis product cache invalidator**

Create `internal/cache/product.go` with:

```go
package cache

import (
	"context"

	"github.com/redis/go-redis/v9"
)

const productListCachePattern = "products:list:*"

type ProductCache struct {
	client *redis.Client
}

func NewProductCache(client *redis.Client) *ProductCache {
	return &ProductCache{client: client}
}

func (c *ProductCache) InvalidateProductLists(ctx context.Context) error {
	if c == nil || c.client == nil {
		return nil
	}

	iter := c.client.Scan(ctx, 0, productListCachePattern, 100).Iterator()
	keys := make([]string, 0, 100)
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
		if len(keys) == cap(keys) {
			if err := c.client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
			keys = keys[:0]
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(keys) == 0 {
		return nil
	}
	return c.client.Del(ctx, keys...).Err()
}
```

- [ ] **Step 2: Run cache package tests**

Run:

```powershell
& 'C:\sdk\go1.26.2\bin\go.exe' test ./internal/cache
```

Expected: PASS or `[no test files]`.

- [ ] **Step 3: Commit cache invalidator**

Run:

```powershell
git add internal/cache/product.go
git commit -m "feat: add product cache invalidator"
```

---

### Task 4: Wire Transaction and Cache in Router

**Files:**
- Modify: `internal/http/router/router.go`

- [ ] **Step 1: Add cache package import**

Add the cache import:

```go
import (
	"cafeTelkom/internal/cache"
	"cafeTelkom/internal/config"
	"cafeTelkom/internal/http/handler"
	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/integrations/supabase"
	"cafeTelkom/internal/repository"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)
```

- [ ] **Step 2: Replace product service construction**

Replace:

```go
productService := service.NewProductService(repo)
```

with:

```go
productServiceOptions := []service.ProductServiceOption{}
if dbPool != nil {
	productServiceOptions = append(productServiceOptions, service.WithProductTransaction(dbPool, repo))
}
if redisClient != nil {
	productServiceOptions = append(productServiceOptions, service.WithProductCache(cache.NewProductCache(redisClient)))
}
productService := service.NewProductService(repo, productServiceOptions...)
```

- [ ] **Step 3: Run router package tests**

Run:

```powershell
& 'C:\sdk\go1.26.2\bin\go.exe' test ./internal/http/router
```

Expected: PASS or `[no test files]`.

- [ ] **Step 4: Commit router wiring**

Run:

```powershell
git add internal/http/router/router.go
git commit -m "feat: wire product transaction and cache"
```

---

### Task 5: Full Verification

**Files:**
- Verify: all modified files

- [ ] **Step 1: Run full test suite**

Run:

```powershell
& 'C:\sdk\go1.26.2\bin\go.exe' test ./...
```

Expected: PASS.

- [ ] **Step 2: Check git status**

Run:

```powershell
git status --short
```

Expected: clean working tree after task commits, or only intentional uncommitted files if the executor intentionally skipped commits.

- [ ] **Step 3: Confirm final behavior**

Confirm these facts from code and tests:

- `CreateProduct` returns an error when no transaction runner is configured.
- successful create calls duplicate-name check and insert through the transaction repository.
- cache invalidation runs only after the transaction runner returns success.
- cache invalidation errors are ignored.
- router wires transaction only when `dbPool != nil`.
- router wires product cache only when `redisClient != nil`.

---

## Self-Review Notes

- Spec coverage: Tasks 1 and 2 cover transaction-backed create and same-transaction duplicate check/insert. Tasks 1 and 3 cover product list cache invalidation. Task 1 covers Redis failure not blocking the create result. Task 4 preserves HTTP behavior by only changing service construction.
- Placeholder scan: no deferred implementation wording or unspecified edge handling remains.
- Type consistency: `ProductServiceOption`, `productTxRunner`, `ProductCacheInvalidator`, `WithProductTxRunner`, `WithProductTransaction`, and `WithProductCache` are introduced before later tasks use them.
