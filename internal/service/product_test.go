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
	repo := &fakeProductRepo{
		products: []repository.Product{
			productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
			productRow(t, "22222222-2222-4222-8222-222222222222", "Cafe Latte", createdAt),
		},
	}

	service := NewProductService(repo, nil, nil)
	list, err := service.ListProducts(context.Background(), ListProductsInput{Limit: 1})
	if err != nil {
		t.Fatalf("list products: %v", err)
	}

	if repo.listArg.Limit != 2 {
		t.Fatalf("repo limit = %d", repo.listArg.Limit)
	}
	if len(list.Items) != 1 {
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
	detailProductID         string
	err                     error
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
	products     []repository.Product
	product      repository.Product
	nameProduct  repository.Product
	err          error
	getByNameErr error
	listArg      repository.ListProductsParams
	createArg    repository.CreateProductParams
	updateArg    repository.UpdateProductParams
	getCalled    bool
	createCalled bool
	updateCalled bool
}

func (f *fakeProductRepo) ListProducts(ctx context.Context, arg repository.ListProductsParams) ([]repository.Product, error) {
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
