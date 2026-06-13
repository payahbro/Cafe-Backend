package service

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestProductServiceListProductsMapsRows(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	deletedAt := time.Date(2026, 5, 26, 2, 0, 0, 0, time.UTC)
	deleted := productRow(t, "22222222-2222-4222-8222-222222222222", "Cafe Latte", createdAt)
	deleted.DeletedAt = pgtype.Timestamptz{Time: deletedAt, Valid: true}
	repo := &fakeProductRepo{
		products: []repository.Product{
			productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
			deleted,
			productRow(t, "33333333-3333-4333-8333-333333333333", "Mocha", createdAt),
		},
	}

	service := NewProductService(repo, nil, nil)
	list, err := service.ListProducts(context.Background(), ListProductsInput{Limit: 2})
	if err != nil {
		t.Fatalf("list products: %v", err)
	}

	if repo.listArg.Limit != 3 {
		t.Fatalf("repo limit = %d", repo.listArg.Limit)
	}
	if len(list.Items) != 2 {
		t.Fatalf("items len = %d", len(list.Items))
	}
	if !list.HasNext {
		t.Fatalf("expected has_next true when repository returns limit+1 rows")
	}
	if list.Items[0].ID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("id = %q", list.Items[0].ID)
	}
	if list.Items[0].Rating != 4.5 {
		t.Fatalf("rating = %v", list.Items[0].Rating)
	}
	if list.Items[1].DeletedAt == nil || !list.Items[1].DeletedAt.Equal(deletedAt) {
		t.Fatalf("deleted_at = %v", list.Items[1].DeletedAt)
	}
}

func TestProductServiceListProductsReturnsCacheHit(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	repo := &fakeProductRepo{
		products: []repository.Product{
			productRow(t, "22222222-2222-4222-8222-222222222222", "Cafe Latte", createdAt),
		},
	}
	cache := &fakeProductCacheInvalidator{
		list: &ProductList{
			Items: []Product{
				{
					ID:        "11111111-1111-4111-8111-111111111111",
					Name:      "Americano",
					Price:     25000,
					Category:  "coffee",
					Status:    "available",
					CreatedAt: createdAt,
					UpdatedAt: createdAt,
				},
			},
			Limit:   10,
			HasNext: false,
			HasPrev: false,
		},
	}
	service := NewProductService(repo, nil, cache)

	list, err := service.ListProducts(context.Background(), ListProductsInput{Limit: 10})
	if err != nil {
		t.Fatalf("list products: %v", err)
	}

	if repo.listCalled {
		t.Fatalf("repository should not be called on cache hit")
	}
	if list.Items[0].Name != "Americano" {
		t.Fatalf("product name = %q", list.Items[0].Name)
	}
	if cache.listInput.Limit != 10 {
		t.Fatalf("cache list limit = %d", cache.listInput.Limit)
	}
}

func TestProductServiceListProductsCachesMissResult(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	repo := &fakeProductRepo{
		products: []repository.Product{
			productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
			productRow(t, "22222222-2222-4222-8222-222222222222", "Cafe Latte", createdAt),
		},
	}
	cache := &fakeProductCacheInvalidator{listErr: ErrProductCacheMiss}
	service := NewProductService(repo, nil, cache)

	list, err := service.ListProducts(context.Background(), ListProductsInput{Limit: 1})
	if err != nil {
		t.Fatalf("list products: %v", err)
	}

	if !repo.listCalled {
		t.Fatalf("expected repository to be called on cache miss")
	}
	if !cache.setListCalled {
		t.Fatalf("expected list result to be cached")
	}
	if cache.setListInput.Limit != 1 {
		t.Fatalf("cached list input limit = %d", cache.setListInput.Limit)
	}
	if !list.HasNext {
		t.Fatalf("expected has_next true")
	}
}

func TestProductServiceListProductsPassesIncludeDeletedToRepositoryAndCache(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	repo := &fakeProductRepo{
		products: []repository.Product{
			productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
		},
	}
	cache := &fakeProductCacheInvalidator{listErr: ErrProductCacheMiss}
	service := NewProductService(repo, nil, cache)

	list, err := service.ListProducts(context.Background(), ListProductsInput{Limit: 10, IncludeDeleted: true})
	if err != nil {
		t.Fatalf("list products: %v", err)
	}

	if len(list.Items) != 1 {
		t.Fatalf("items len = %d", len(list.Items))
	}
	if !repo.listArg.IncludeDeleted {
		t.Fatalf("expected repository include deleted flag")
	}
	if !cache.listInput.IncludeDeleted {
		t.Fatalf("expected cache lookup include deleted flag")
	}
	if !cache.setListInput.IncludeDeleted {
		t.Fatalf("expected cached result include deleted flag")
	}
}

func TestProductServiceGetProductMapsRow(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	repo := &fakeProductRepo{
		product: productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}

	service := NewProductService(repo, nil, nil)
	product, err := service.GetProduct(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatalf("get product: %v", err)
	}

	if product.Name != "Americano" {
		t.Fatalf("name = %q", product.Name)
	}
	if product.Description == nil || *product.Description != "Description for Americano" {
		t.Fatalf("description = %v", product.Description)
	}
	if string(product.Attributes) != `{"temperature":["hot","iced"]}` {
		t.Fatalf("attributes = %s", product.Attributes)
	}
}

func TestProductServiceGetProductReturnsCacheHit(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	repo := &fakeProductRepo{
		product: productRow(t, "22222222-2222-4222-8222-222222222222", "Cafe Latte", createdAt),
	}
	cache := &fakeProductCacheInvalidator{
		product: &Product{
			ID:        "11111111-1111-4111-8111-111111111111",
			Name:      "Americano",
			Price:     25000,
			Category:  "coffee",
			Status:    "available",
			CreatedAt: createdAt,
			UpdatedAt: createdAt,
		},
	}
	service := NewProductService(repo, nil, cache)

	product, err := service.GetProduct(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatalf("get product: %v", err)
	}

	if repo.getCalled {
		t.Fatalf("repository should not be called on cache hit")
	}
	if product.Name != "Americano" {
		t.Fatalf("product name = %q", product.Name)
	}
	if cache.detailProductID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("cache detail product id = %q", cache.detailProductID)
	}
}

func TestProductServiceGetProductCachesMissResult(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	productID := "11111111-1111-4111-8111-111111111111"
	repo := &fakeProductRepo{
		product: productRow(t, productID, "Americano", createdAt),
	}
	cache := &fakeProductCacheInvalidator{detailErr: ErrProductCacheMiss}
	service := NewProductService(repo, nil, cache)

	product, err := service.GetProduct(context.Background(), productID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}

	if !repo.getCalled {
		t.Fatalf("expected repository to be called on cache miss")
	}
	if !cache.setDetailCalled {
		t.Fatalf("expected detail result to be cached")
	}
	if product.Name != "Americano" {
		t.Fatalf("product name = %q", product.Name)
	}
}

func TestProductServiceGetProductFallsBackWhenCacheFails(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	productID := "11111111-1111-4111-8111-111111111111"
	repo := &fakeProductRepo{
		product: productRow(t, productID, "Americano", createdAt),
	}
	cache := &fakeProductCacheInvalidator{detailErr: errors.New("redis unavailable")}
	service := NewProductService(repo, nil, cache)

	product, err := service.GetProduct(context.Background(), productID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}

	if !repo.getCalled {
		t.Fatalf("expected repository to be called when cache fails")
	}
	if product.Name != "Americano" {
		t.Fatalf("product name = %q", product.Name)
	}
}

func TestProductServiceGetProductRejectsInvalidID(t *testing.T) {
	repo := &fakeProductRepo{}
	service := NewProductService(repo, nil, nil)

	_, err := service.GetProduct(context.Background(), "not-a-uuid")
	if !errors.Is(err, ErrInvalidProductID) {
		t.Fatalf("err = %v", err)
	}
	if repo.getCalled {
		t.Fatalf("repository should not be called for invalid uuid")
	}
}

func TestProductServiceGetProductReturnsNotFound(t *testing.T) {
	repo := &fakeProductRepo{err: pgx.ErrNoRows}
	service := NewProductService(repo, nil, nil)

	_, err := service.GetProduct(context.Background(), "11111111-1111-4111-8111-111111111111")
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("err = %v", err)
	}
}

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
	service := NewProductService(baseRepo, txRunner, cache)

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

func TestProductServiceCreateProductRejectsDuplicateNameInTransaction(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	txRepo := &fakeProductRepo{
		product: productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}
	txRunner := &fakeProductTxRunner{repo: txRepo}
	cache := &fakeProductCacheInvalidator{}
	service := NewProductService(&fakeProductRepo{}, txRunner, cache)

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

func TestProductServiceCreateProductIgnoresCacheInvalidationError(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	txRepo := &fakeProductRepo{
		getByNameErr: pgx.ErrNoRows,
		product:      productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}
	txRunner := &fakeProductTxRunner{repo: txRepo}
	cache := &fakeProductCacheInvalidator{err: errors.New("redis unavailable")}
	service := NewProductService(&fakeProductRepo{}, txRunner, cache)

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

func TestProductServiceUpdateProductMergesPartialFieldsInTransactionAndInvalidatesCache(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	productID := "11111111-1111-4111-8111-111111111111"
	existing := productRow(t, productID, "Americano", createdAt)
	txRepo := &fakeProductRepo{
		product:      existing,
		getByNameErr: pgx.ErrNoRows,
	}
	txRunner := &fakeProductTxRunner{repo: txRepo}
	cache := &fakeProductCacheInvalidator{}
	cache.txRunner = txRunner
	service := NewProductService(&fakeProductRepo{}, txRunner, cache)
	price := int32(28000)

	product, err := service.UpdateProduct(context.Background(), productID, UpdateProductInput{
		Price: &price,
	})
	if err != nil {
		t.Fatalf("update product: %v", err)
	}

	if !txRunner.called {
		t.Fatalf("expected transaction runner to be called")
	}
	if !txRepo.updateCalled {
		t.Fatalf("expected update to be called")
	}
	if txRepo.updateArg.Name != "Americano" {
		t.Fatalf("update name = %q", txRepo.updateArg.Name)
	}
	if txRepo.updateArg.Price != 28000 {
		t.Fatalf("update price = %d", txRepo.updateArg.Price)
	}
	if txRepo.updateArg.Category != repository.ProductCategoryCoffee {
		t.Fatalf("update category = %q", txRepo.updateArg.Category)
	}
	if txRepo.updateArg.Status != repository.ProductStatusAvailable {
		t.Fatalf("update status = %q", txRepo.updateArg.Status)
	}
	if product.Price != 28000 {
		t.Fatalf("product price = %d", product.Price)
	}
	if !cache.invalidated {
		t.Fatalf("expected product list cache invalidation")
	}
	if cache.detailProductID != productID {
		t.Fatalf("detail cache product id = %q", cache.detailProductID)
	}
	if cache.invalidatedBeforeTxDone {
		t.Fatalf("cache invalidation should run after transaction runner finishes")
	}
}

func TestProductServiceUpdateProductRejectsInvalidID(t *testing.T) {
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: &fakeProductRepo{}}, nil)
	price := int32(28000)

	_, err := service.UpdateProduct(context.Background(), "not-a-uuid", UpdateProductInput{Price: &price})
	if !errors.Is(err, ErrInvalidProductID) {
		t.Fatalf("err = %v", err)
	}
}

func TestProductServiceUpdateProductReturnsNotFound(t *testing.T) {
	txRepo := &fakeProductRepo{err: pgx.ErrNoRows}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, nil)
	price := int32(28000)

	_, err := service.UpdateProduct(context.Background(), "11111111-1111-4111-8111-111111111111", UpdateProductInput{Price: &price})
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestProductServiceUpdateProductRejectsDuplicateName(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	txRepo := &fakeProductRepo{
		product:     productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
		nameProduct: productRow(t, "22222222-2222-4222-8222-222222222222", "Cafe Latte", createdAt),
	}
	cache := &fakeProductCacheInvalidator{}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, cache)
	name := "Cafe Latte"

	_, err := service.UpdateProduct(context.Background(), "11111111-1111-4111-8111-111111111111", UpdateProductInput{
		Name: &name,
	})
	if !errors.Is(err, ErrProductNameAlreadyExists) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.updateCalled {
		t.Fatalf("update should not be called for duplicate name")
	}
	if cache.invalidated {
		t.Fatalf("cache should not be invalidated when update fails")
	}
}

func TestProductServiceUpdateProductStatusAllowsAdminUnavailableAndInvalidatesCache(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	productID := "11111111-1111-4111-8111-111111111111"
	txRepo := &fakeProductRepo{
		product: productRow(t, productID, "Americano", createdAt),
	}
	txRunner := &fakeProductTxRunner{repo: txRepo}
	cache := &fakeProductCacheInvalidator{}
	cache.txRunner = txRunner
	service := NewProductService(&fakeProductRepo{}, txRunner, cache)

	product, err := service.UpdateProductStatus(context.Background(), productID, UpdateProductStatusInput{
		Status:    "unavailable",
		ActorRole: string(repository.UserRoleADMIN),
	})
	if err != nil {
		t.Fatalf("update product status: %v", err)
	}

	if !txRunner.called {
		t.Fatalf("expected transaction runner to be called")
	}
	if !txRepo.updateStatusCalled {
		t.Fatalf("expected update status to be called")
	}
	if txRepo.updateStatusArg.Status != repository.ProductStatusUnavailable {
		t.Fatalf("status = %q", txRepo.updateStatusArg.Status)
	}
	if product.Status != "unavailable" {
		t.Fatalf("product status = %q", product.Status)
	}
	if !cache.invalidated {
		t.Fatalf("expected product list cache invalidation")
	}
	if cache.detailProductID != productID {
		t.Fatalf("detail cache product id = %q", cache.detailProductID)
	}
	if cache.invalidatedBeforeTxDone {
		t.Fatalf("cache invalidation should run after transaction runner finishes")
	}
}

func TestProductServiceUpdateProductStatusRejectsPegawaiUnavailable(t *testing.T) {
	txRepo := &fakeProductRepo{}
	cache := &fakeProductCacheInvalidator{}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, cache)

	_, err := service.UpdateProductStatus(context.Background(), "11111111-1111-4111-8111-111111111111", UpdateProductStatusInput{
		Status:    "unavailable",
		ActorRole: string(repository.UserRolePEGAWAI),
	})
	if !errors.Is(err, ErrProductStatusForbidden) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.updateStatusCalled {
		t.Fatalf("update status should not be called")
	}
	if cache.invalidated {
		t.Fatalf("cache should not be invalidated")
	}
}

func TestProductServiceUpdateProductStatusAllowsPegawaiOutOfStock(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	txRepo := &fakeProductRepo{
		product: productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, nil)

	product, err := service.UpdateProductStatus(context.Background(), "11111111-1111-4111-8111-111111111111", UpdateProductStatusInput{
		Status:    "out_of_stock",
		ActorRole: string(repository.UserRolePEGAWAI),
	})
	if err != nil {
		t.Fatalf("update product status: %v", err)
	}
	if product.Status != "out_of_stock" {
		t.Fatalf("product status = %q", product.Status)
	}
}

func TestProductServiceUpdateProductStatusRejectsInvalidID(t *testing.T) {
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: &fakeProductRepo{}}, nil)

	_, err := service.UpdateProductStatus(context.Background(), "not-a-uuid", UpdateProductStatusInput{
		Status:    "available",
		ActorRole: string(repository.UserRoleADMIN),
	})
	if !errors.Is(err, ErrInvalidProductID) {
		t.Fatalf("err = %v", err)
	}
}

func TestProductServiceUpdateProductStatusReturnsNotFound(t *testing.T) {
	txRepo := &fakeProductRepo{err: pgx.ErrNoRows}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, nil)

	_, err := service.UpdateProductStatus(context.Background(), "11111111-1111-4111-8111-111111111111", UpdateProductStatusInput{
		Status:    "available",
		ActorRole: string(repository.UserRoleADMIN),
	})
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestProductServiceUpdateProductStatusRejectsAlreadyDeletedProduct(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	deleted := productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt)
	deleted.DeletedAt = pgtype.Timestamptz{Time: createdAt, Valid: true}
	txRepo := &fakeProductRepo{product: deleted}
	cache := &fakeProductCacheInvalidator{}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, cache)

	_, err := service.UpdateProductStatus(context.Background(), "11111111-1111-4111-8111-111111111111", UpdateProductStatusInput{
		Status:    "available",
		ActorRole: string(repository.UserRoleADMIN),
	})
	if !errors.Is(err, ErrProductAlreadyDeleted) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.updateStatusCalled {
		t.Fatalf("update status should not be called for already deleted product")
	}
	if cache.invalidated {
		t.Fatalf("cache should not be invalidated when update status fails")
	}
}

func TestProductServiceDeleteProductSoftDeletesInTransactionAndInvalidatesCache(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	productID := "11111111-1111-4111-8111-111111111111"
	txRepo := &fakeProductRepo{
		product: productRow(t, productID, "Americano", createdAt),
	}
	txRunner := &fakeProductTxRunner{repo: txRepo}
	cache := &fakeProductCacheInvalidator{}
	cache.txRunner = txRunner
	service := NewProductService(&fakeProductRepo{}, txRunner, cache)

	err := service.DeleteProduct(context.Background(), productID)
	if err != nil {
		t.Fatalf("delete product: %v", err)
	}

	if !txRunner.called {
		t.Fatalf("expected transaction runner to be called")
	}
	if !txRepo.getIncludingDeletedCalled {
		t.Fatalf("expected product lookup including deleted rows")
	}
	if !txRepo.deleteCalled {
		t.Fatalf("expected soft delete to be called")
	}
	if txRepo.deletedID.String() != productID {
		t.Fatalf("deleted id = %q", txRepo.deletedID.String())
	}
	if !cache.invalidated {
		t.Fatalf("expected product list cache invalidation")
	}
	if cache.detailProductID != productID {
		t.Fatalf("detail cache product id = %q", cache.detailProductID)
	}
	if cache.invalidatedBeforeTxDone {
		t.Fatalf("cache invalidation should run after transaction runner finishes")
	}
}

func TestProductServiceDeleteProductRejectsAlreadyDeletedProduct(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	deleted := productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt)
	deleted.DeletedAt = pgtype.Timestamptz{Time: createdAt, Valid: true}
	txRepo := &fakeProductRepo{product: deleted}
	cache := &fakeProductCacheInvalidator{}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, cache)

	err := service.DeleteProduct(context.Background(), "11111111-1111-4111-8111-111111111111")
	if !errors.Is(err, ErrProductAlreadyDeleted) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.deleteCalled {
		t.Fatalf("soft delete should not be called for already deleted product")
	}
	if cache.invalidated {
		t.Fatalf("cache should not be invalidated when delete fails")
	}
}

func TestProductServiceDeleteProductRejectsInvalidID(t *testing.T) {
	txRepo := &fakeProductRepo{}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, nil)

	err := service.DeleteProduct(context.Background(), "not-a-uuid")
	if !errors.Is(err, ErrInvalidProductID) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.deleteCalled {
		t.Fatalf("soft delete should not be called for invalid uuid")
	}
}

func TestProductServiceDeleteProductReturnsNotFound(t *testing.T) {
	txRepo := &fakeProductRepo{err: pgx.ErrNoRows}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, nil)

	err := service.DeleteProduct(context.Background(), "11111111-1111-4111-8111-111111111111")
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestProductServiceRestoreProductRestoresDeletedProductInTransactionAndInvalidatesCache(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	productID := "11111111-1111-4111-8111-111111111111"
	deleted := productRow(t, productID, "Americano", createdAt)
	deleted.Status = repository.ProductStatusUnavailable
	deleted.DeletedAt = pgtype.Timestamptz{Time: createdAt, Valid: true}
	txRepo := &fakeProductRepo{product: deleted}
	txRunner := &fakeProductTxRunner{repo: txRepo}
	cache := &fakeProductCacheInvalidator{}
	cache.txRunner = txRunner
	service := NewProductService(&fakeProductRepo{}, txRunner, cache)

	product, err := service.RestoreProduct(context.Background(), productID)
	if err != nil {
		t.Fatalf("restore product: %v", err)
	}

	if !txRunner.called {
		t.Fatalf("expected transaction runner to be called")
	}
	if !txRepo.getIncludingDeletedCalled {
		t.Fatalf("expected product lookup including deleted rows")
	}
	if !txRepo.restoreCalled {
		t.Fatalf("expected restore to be called")
	}
	if txRepo.restoredID.String() != productID {
		t.Fatalf("restored id = %q", txRepo.restoredID.String())
	}
	if product.Status != "available" {
		t.Fatalf("product status = %q", product.Status)
	}
	if !cache.invalidated {
		t.Fatalf("expected product list cache invalidation")
	}
	if cache.detailProductID != productID {
		t.Fatalf("detail cache product id = %q", cache.detailProductID)
	}
	if cache.invalidatedBeforeTxDone {
		t.Fatalf("cache invalidation should run after transaction runner finishes")
	}
}

func TestProductServiceRestoreProductRejectsActiveProduct(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	txRepo := &fakeProductRepo{
		product: productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}
	cache := &fakeProductCacheInvalidator{}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, cache)

	_, err := service.RestoreProduct(context.Background(), "11111111-1111-4111-8111-111111111111")
	if !errors.Is(err, ErrProductNotDeleted) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.restoreCalled {
		t.Fatalf("restore should not be called for active product")
	}
	if cache.invalidated {
		t.Fatalf("cache should not be invalidated when restore fails")
	}
}

func TestProductServiceRestoreProductRejectsInvalidID(t *testing.T) {
	txRepo := &fakeProductRepo{}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, nil)

	_, err := service.RestoreProduct(context.Background(), "not-a-uuid")
	if !errors.Is(err, ErrInvalidProductID) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.restoreCalled {
		t.Fatalf("restore should not be called for invalid uuid")
	}
}

func TestProductServiceRestoreProductReturnsNotFound(t *testing.T) {
	txRepo := &fakeProductRepo{err: pgx.ErrNoRows}
	service := NewProductService(&fakeProductRepo{}, &fakeProductTxRunner{repo: txRepo}, nil)

	_, err := service.RestoreProduct(context.Background(), "11111111-1111-4111-8111-111111111111")
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("err = %v", err)
	}
}

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
	list                    *ProductList
	product                 *Product
	listInput               ListProductsInput
	setListInput            ListProductsInput
	detailProductID         string
	listErr                 error
	detailErr               error
	setListCalled           bool
	setDetailCalled         bool
	err                     error
}

func (f *fakeProductCacheInvalidator) GetProductList(ctx context.Context, input ListProductsInput) (*ProductList, error) {
	f.listInput = input
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.list, nil
}

func (f *fakeProductCacheInvalidator) SetProductList(ctx context.Context, input ListProductsInput, list ProductList) error {
	f.setListCalled = true
	f.setListInput = input
	return f.err
}

func (f *fakeProductCacheInvalidator) GetProductDetail(ctx context.Context, productID string) (*Product, error) {
	f.detailProductID = productID
	if f.detailErr != nil {
		return nil, f.detailErr
	}
	return f.product, nil
}

func (f *fakeProductCacheInvalidator) SetProductDetail(ctx context.Context, product Product) error {
	f.setDetailCalled = true
	return f.err
}

func (f *fakeProductCacheInvalidator) InvalidateProductLists(ctx context.Context) error {
	f.invalidated = true
	if f.txRunner != nil && !f.txRunner.done {
		f.invalidatedBeforeTxDone = true
	}
	return f.err
}

func (f *fakeProductCacheInvalidator) InvalidateProductDetail(ctx context.Context, productID string) error {
	f.detailProductID = productID
	if f.txRunner != nil && !f.txRunner.done {
		f.invalidatedBeforeTxDone = true
	}
	return f.err
}

type fakeProductRepo struct {
	products                  []repository.Product
	product                   repository.Product
	nameProduct               repository.Product
	err                       error
	getByNameErr              error
	listArg                   repository.ListProductsParams
	createArg                 repository.CreateProductParams
	updateArg                 repository.UpdateProductParams
	updateStatusArg           repository.UpdateProductStatusParams
	deletedID                 pgtype.UUID
	restoredID                pgtype.UUID
	listCalled                bool
	getCalled                 bool
	getIncludingDeletedCalled bool
	createCalled              bool
	updateCalled              bool
	updateStatusCalled        bool
	deleteCalled              bool
	restoreCalled             bool
}

func (f *fakeProductRepo) ListProducts(ctx context.Context, arg repository.ListProductsParams) ([]repository.Product, error) {
	f.listCalled = true
	f.listArg = arg
	if f.err != nil {
		return nil, f.err
	}
	return f.products, nil
}

func (f *fakeProductRepo) GetProductByID(ctx context.Context, id pgtype.UUID) (repository.Product, error) {
	f.getCalled = true
	if f.err != nil {
		return repository.Product{}, f.err
	}
	return f.product, nil
}

func (f *fakeProductRepo) GetProductByIDIncludingDeleted(ctx context.Context, id pgtype.UUID) (repository.Product, error) {
	f.getIncludingDeletedCalled = true
	if f.err != nil {
		return repository.Product{}, f.err
	}
	return f.product, nil
}

func (f *fakeProductRepo) GetProductByNameCI(ctx context.Context, lower string) (repository.Product, error) {
	if f.getByNameErr != nil {
		return repository.Product{}, f.getByNameErr
	}
	if f.err != nil {
		return repository.Product{}, f.err
	}
	if f.nameProduct.ID.Valid {
		return f.nameProduct, nil
	}
	return f.product, nil
}

func (f *fakeProductRepo) CreateProduct(ctx context.Context, arg repository.CreateProductParams) (repository.Product, error) {
	f.createCalled = true
	f.createArg = arg
	if f.err != nil {
		return repository.Product{}, f.err
	}
	return f.product, nil
}

func (f *fakeProductRepo) UpdateProduct(ctx context.Context, arg repository.UpdateProductParams) (repository.Product, error) {
	f.updateCalled = true
	f.updateArg = arg
	if f.err != nil {
		return repository.Product{}, f.err
	}
	product := f.product
	product.Name = arg.Name
	product.Description = arg.Description
	product.Price = arg.Price
	product.Category = arg.Category
	product.Status = arg.Status
	product.ImageUrl = arg.ImageUrl
	product.Attributes = arg.Attributes
	return product, nil
}

func (f *fakeProductRepo) UpdateProductStatus(ctx context.Context, arg repository.UpdateProductStatusParams) (repository.Product, error) {
	f.updateStatusCalled = true
	f.updateStatusArg = arg
	if f.err != nil {
		return repository.Product{}, f.err
	}
	product := f.product
	product.Status = arg.Status
	return product, nil
}

func (f *fakeProductRepo) SoftDeleteProduct(ctx context.Context, id pgtype.UUID) (repository.Product, error) {
	f.deleteCalled = true
	f.deletedID = id
	if f.err != nil {
		return repository.Product{}, f.err
	}
	product := f.product
	product.Status = repository.ProductStatusUnavailable
	product.DeletedAt = pgtype.Timestamptz{Time: time.Now(), Valid: true}
	return product, nil
}

func (f *fakeProductRepo) RestoreProduct(ctx context.Context, id pgtype.UUID) (repository.Product, error) {
	f.restoreCalled = true
	f.restoredID = id
	if f.err != nil {
		return repository.Product{}, f.err
	}
	product := f.product
	product.Status = repository.ProductStatusAvailable
	product.DeletedAt = pgtype.Timestamptz{}
	return product, nil
}

func productRow(t *testing.T, id, name string, createdAt time.Time) repository.Product {
	t.Helper()

	var productID pgtype.UUID
	if err := productID.Scan(id); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}

	return repository.Product{
		ID:          productID,
		Name:        name,
		Description: pgtype.Text{String: "Description for " + name, Valid: true},
		Price:       25000,
		Category:    repository.ProductCategoryCoffee,
		Status:      repository.ProductStatusAvailable,
		ImageUrl:    pgtype.Text{String: "https://example.supabase.co/storage/v1/object/public/products/" + name + ".png", Valid: true},
		Attributes:  []byte(`{"temperature":["hot","iced"]}`),
		Stock:       10,
		Rating: pgtype.Numeric{
			Int:   big.NewInt(45),
			Exp:   -1,
			Valid: true,
		},
		TotalSold: 120,
		CreatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
}
