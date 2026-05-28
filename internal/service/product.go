package service

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

var (
	ErrInvalidProductID         = errors.New("invalid product id")
	ErrProductNotFound          = errors.New("product not found")
	ErrProductNameAlreadyExists = errors.New("product name already exists")
)

type productRepository interface {
	ListProducts(ctx context.Context, arg repository.ListProductsParams) ([]repository.Product, error)
	GetProductByID(ctx context.Context, id pgtype.UUID) (repository.Product, error)
	GetProductByNameCI(ctx context.Context, lower string) (repository.Product, error)
	CreateProduct(ctx context.Context, arg repository.CreateProductParams) (repository.Product, error)
}

type productTxRunner interface {
	Run(ctx context.Context, fn func(productRepository) error) error
}

type ProductCacheInvalidator interface {
	InvalidateProductLists(ctx context.Context) error
}

type ProductTxRunner struct {
	db   cafedb.TxBeginner
	repo *repository.Queries
}

type ProductService struct {
	repo     productRepository
	txRunner productTxRunner
	cache    ProductCacheInvalidator
}

type ListProductsInput struct {
	Limit  int32
	Offset int32
}

type ProductList struct {
	Items   []Product
	Limit   int32
	HasNext bool
	HasPrev bool
}

type Product struct {
	ID          string
	Name        string
	Description *string
	Price       int32
	Category    string
	Status      string
	ImageURL    *string
	Rating      float64
	TotalSold   int32
	Attributes  []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type CreateProductInput struct {
	Name        string
	Description *string
	Price       int32
	Category    string
	Status      string
	ImageURL    string
	Attributes  []byte
}

func NewProductService(repo productRepository, txRunner productTxRunner, cache ProductCacheInvalidator) *ProductService {
	return &ProductService{
		repo:     repo,
		txRunner: txRunner,
		cache:    cache,
	}
}

func NewProductTxRunner(db cafedb.TxBeginner, repo *repository.Queries) *ProductTxRunner {
	if db == nil || repo == nil {
		return nil
	}
	return &ProductTxRunner{db: db, repo: repo}
}

func (r *ProductTxRunner) Run(ctx context.Context, fn func(productRepository) error) error {
	if r == nil || r.db == nil || r.repo == nil {
		return errors.New("product transaction runner missing")
	}

	return cafedb.WithTx(ctx, r.db, func(ctx context.Context, tx pgx.Tx) error {
		return fn(r.repo.WithTx(tx))
	})
}

func (s *ProductService) ListProducts(ctx context.Context, input ListProductsInput) (*ProductList, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	limit := normalizeProductLimit(input.Limit)
	offset := input.Offset
	if offset < 0 {
		offset = 0
	}

	rows, err := s.repo.ListProducts(ctx, repository.ListProductsParams{
		Limit:  limit + 1,
		Offset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}

	hasNext := len(rows) > int(limit)
	if hasNext {
		rows = rows[:limit]
	}

	items := make([]Product, 0, len(rows))
	for _, row := range rows {
		items = append(items, productFromRow(row))
	}

	return &ProductList{
		Items:   items,
		Limit:   limit,
		HasNext: hasNext,
		HasPrev: offset > 0,
	}, nil
}

func (s *ProductService) GetProduct(ctx context.Context, productID string) (*Product, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	var id pgtype.UUID
	if err := id.Scan(productID); err != nil {
		return nil, ErrInvalidProductID
	}

	row, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, fmt.Errorf("get product: %w", err)
	}

	product := productFromRow(row)
	return &product, nil
}

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

func normalizeProductLimit(limit int32) int32 {
	if limit <= 0 {
		return 10
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func productFromRow(row repository.Product) Product {
	return Product{
		ID:          row.ID.String(),
		Name:        row.Name,
		Description: textPtr(row.Description),
		Price:       row.Price,
		Category:    string(row.Category),
		Status:      string(row.Status),
		ImageURL:    textPtr(row.ImageUrl),
		Rating:      numericFloat64(row.Rating),
		TotalSold:   row.TotalSold,
		Attributes:  row.Attributes,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
}

func numericFloat64(value pgtype.Numeric) float64 {
	floatValue, err := value.Float64Value()
	if err != nil || !floatValue.Valid {
		return 0
	}
	return floatValue.Float64
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
