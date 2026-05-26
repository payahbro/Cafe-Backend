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

	service := NewProductService(repo)
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

	service := NewProductService(repo)
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
	service := NewProductService(repo)

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
	service := NewProductService(repo)

	_, err := service.GetProduct(context.Background(), "11111111-1111-4111-8111-111111111111")
	if !errors.Is(err, ErrProductNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestProductServiceCreateProductCreatesRow(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	repo := &fakeProductRepo{
		getByNameErr: pgx.ErrNoRows,
		product:      productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}
	description := "Espresso dengan air panas"
	service := NewProductService(repo)

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

	if !repo.createCalled {
		t.Fatalf("expected create to be called")
	}
	if repo.createArg.Name != "Americano" {
		t.Fatalf("create name = %q", repo.createArg.Name)
	}
	if repo.createArg.Status != repository.ProductStatusAvailable {
		t.Fatalf("create status = %q", repo.createArg.Status)
	}
	if repo.createArg.Stock != 0 {
		t.Fatalf("create stock = %d", repo.createArg.Stock)
	}
	if product.Name != "Americano" {
		t.Fatalf("product name = %q", product.Name)
	}
}

func TestProductServiceCreateProductRejectsDuplicateName(t *testing.T) {
	createdAt := time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC)
	repo := &fakeProductRepo{
		product: productRow(t, "11111111-1111-4111-8111-111111111111", "Americano", createdAt),
	}
	service := NewProductService(repo)

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
	if repo.createCalled {
		t.Fatalf("create should not be called for duplicate name")
	}
}

type fakeProductRepo struct {
	products     []repository.Product
	product      repository.Product
	err          error
	getByNameErr error
	listArg      repository.ListProductsParams
	createArg    repository.CreateProductParams
	getCalled    bool
	createCalled bool
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
