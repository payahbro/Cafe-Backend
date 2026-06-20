package service

import (
	"context"
	"encoding/json"
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
	ErrProductCacheMiss         = errors.New("product cache miss")
	ErrProductStatusForbidden   = errors.New("product status forbidden")
	ErrProductAlreadyDeleted    = errors.New("product already deleted")
	ErrProductNotDeleted        = errors.New("product not deleted")
)

// Interface
type productRepository interface {
	ListProducts(ctx context.Context, arg repository.ListProductsParams) ([]repository.Product, error)
	GetProductByID(ctx context.Context, id pgtype.UUID) (repository.Product, error)
	GetProductByIDIncludingDeleted(ctx context.Context, id pgtype.UUID) (repository.Product, error)
	GetProductByNameCI(ctx context.Context, lower string) (repository.Product, error)
	CreateProduct(ctx context.Context, arg repository.CreateProductParams) (repository.Product, error)
	CreateProductWithDefaultStock(ctx context.Context, arg repository.CreateProductWithDefaultStockParams) (repository.Product, error)
	CreateOutboxEvent(ctx context.Context, arg repository.CreateOutboxEventParams) (repository.OutboxEvent, error)
	UpdateProduct(ctx context.Context, arg repository.UpdateProductParams) (repository.Product, error)
	UpdateProductStatus(ctx context.Context, arg repository.UpdateProductStatusParams) (repository.Product, error)
	SoftDeleteProduct(ctx context.Context, id pgtype.UUID) (repository.Product, error)
	RestoreProduct(ctx context.Context, id pgtype.UUID) (repository.Product, error)
}

type productTxRunner interface {
	Run(ctx context.Context, fn func(productRepository) error) error
}

type ProductCacheInvalidator interface {
	GetProductList(ctx context.Context, input ListProductsInput) (*ProductList, error)
	SetProductList(ctx context.Context, input ListProductsInput, list ProductList) error
	GetProductDetail(ctx context.Context, productID string) (*Product, error)
	SetProductDetail(ctx context.Context, product Product) error
	InvalidateProductLists(ctx context.Context) error
	InvalidateProductDetail(ctx context.Context, productID string) error
}

// Representative table
type Product struct {
	ID          string
	Name        string
	Description *string
	Price       int32
	Category    string
	Status      string
	ImageURL    *string
	Stock       int32
	Rating      float64
	TotalSold   int32
	Attributes  []byte
	CreatedAt   time.Time
	UpdatedAt   time.Time
	DeletedAt   *time.Time
}

// Implementation
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
	Limit          int32
	Offset         int32
	IncludeDeleted bool
}

type ProductList struct {
	Items   []Product
	Limit   int32
	HasNext bool
	HasPrev bool
}

type CreateProductInput struct {
	Name        string
	Description *string
	Price       int32
	Category    string
	Status      string
	Stock       *int32
	ImageURL    string
	Attributes  []byte
}

type UpdateProductInput struct {
	Name        *string
	Description *string
	Price       *int32
	Category    *string
	Status      *string
	ImageURL    *string
	Attributes  *[]byte
}

type UpdateProductStatusInput struct {
	Status    string
	ActorRole string
}

type ProductCreatedNotificationPayload struct {
	Type      string `json:"type"`
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	ImageURL  string `json:"image_url"`
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
	normalizedInput := ListProductsInput{Limit: limit, Offset: offset, IncludeDeleted: input.IncludeDeleted}

	if s.cache != nil {
		if list, err := s.cache.GetProductList(ctx, normalizedInput); err == nil && list != nil {
			return list, nil
		}
	}

	rows, err := s.repo.ListProducts(ctx, repository.ListProductsParams{
		Limit:          limit + 1,
		Offset:         offset,
		IncludeDeleted: input.IncludeDeleted,
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

	list := &ProductList{
		Items:   items,
		Limit:   limit,
		HasNext: hasNext,
		HasPrev: offset > 0,
	}
	if s.cache != nil {
		_ = s.cache.SetProductList(ctx, normalizedInput, *list)
	}

	return list, nil
}

func (s *ProductService) GetProduct(ctx context.Context, productID string) (*Product, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	var id pgtype.UUID
	if err := id.Scan(productID); err != nil {
		return nil, ErrInvalidProductID
	}
	normalizedProductID := id.String()

	if s.cache != nil {
		if product, err := s.cache.GetProductDetail(ctx, normalizedProductID); err == nil && product != nil {
			return product, nil
		}
	}

	row, err := s.repo.GetProductByID(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProductNotFound
		}
		return nil, fmt.Errorf("get product: %w", err)
	}

	product := productFromRow(row)
	if s.cache != nil {
		_ = s.cache.SetProductDetail(ctx, product)
	}

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

		createParams := repository.CreateProductWithDefaultStockParams{
			Name:        name,
			Description: optionalText(input.Description),
			Price:       input.Price,
			Category:    repository.ProductCategory(input.Category),
			Status:      repository.ProductStatus(status),
			ImageUrl:    pgtype.Text{String: input.ImageURL, Valid: input.ImageURL != ""},
			Attributes:  input.Attributes,
		}

		var created repository.Product
		var err error
		if input.Stock == nil {
			created, err = repo.CreateProductWithDefaultStock(ctx, createParams)
		} else {
			created, err = repo.CreateProduct(ctx, repository.CreateProductParams{
				Name:        createParams.Name,
				Description: createParams.Description,
				Price:       createParams.Price,
				Category:    createParams.Category,
				Status:      createParams.Status,
				ImageUrl:    createParams.ImageUrl,
				Attributes:  createParams.Attributes,
				Stock:       *input.Stock,
			})
		}
		if err != nil {
			if isUniqueViolation(err) {
				return ErrProductNameAlreadyExists
			}
			return fmt.Errorf("create product: %w", err)
		}

		if err := createProductCreatedOutboxEvent(ctx, repo, created); err != nil {
			return err
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

func createProductCreatedOutboxEvent(ctx context.Context, repo productRepository, product repository.Product) error {
	payload, err := json.Marshal(productCreatedNotificationPayload(product))
	if err != nil {
		return fmt.Errorf("marshal product created payload: %w", err)
	}

	if _, err := repo.CreateOutboxEvent(ctx, repository.CreateOutboxEventParams{
		AggregateType: "product",
		AggregateID:   product.ID,
		EventType:     "product.created",
		Payload:       payload,
	}); err != nil {
		return fmt.Errorf("create product created outbox event: %w", err)
	}

	return nil
}

func productCreatedNotificationPayload(product repository.Product) ProductCreatedNotificationPayload {
	imageURL := ""
	if product.ImageUrl.Valid {
		imageURL = product.ImageUrl.String
	}

	return ProductCreatedNotificationPayload{
		Type:      "product_created",
		ProductID: product.ID.String(),
		Name:      product.Name,
		Category:  string(product.Category),
		ImageURL:  imageURL,
	}
}

func (s *ProductService) UpdateProduct(ctx context.Context, productID string, input UpdateProductInput) (*Product, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("product transaction runner missing")
	}

	var id pgtype.UUID
	if err := id.Scan(productID); err != nil {
		return nil, ErrInvalidProductID
	}
	normalizedProductID := id.String()

	var row repository.Product
	err := s.txRunner.Run(ctx, func(repo productRepository) error {
		existing, err := repo.GetProductByID(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductNotFound
			}
			return fmt.Errorf("get product: %w", err)
		}

		name := existing.Name
		if input.Name != nil {
			name = strings.TrimSpace(*input.Name)
			if !strings.EqualFold(name, existing.Name) {
				found, err := repo.GetProductByNameCI(ctx, name)
				if err == nil && found.ID.String() != normalizedProductID {
					return ErrProductNameAlreadyExists
				}
				if err != nil && !errors.Is(err, pgx.ErrNoRows) {
					return fmt.Errorf("check product name: %w", err)
				}
			}
		}

		description := existing.Description
		if input.Description != nil {
			description = optionalText(input.Description)
		}

		price := existing.Price
		if input.Price != nil {
			price = *input.Price
		}

		category := existing.Category
		if input.Category != nil {
			category = repository.ProductCategory(*input.Category)
		}

		status := existing.Status
		if input.Status != nil {
			status = repository.ProductStatus(*input.Status)
		}

		imageURL := existing.ImageUrl
		if input.ImageURL != nil {
			imageURL = pgtype.Text{String: *input.ImageURL, Valid: *input.ImageURL != ""}
		}

		attributes := existing.Attributes
		if input.Attributes != nil {
			attributes = *input.Attributes
		}

		updated, err := repo.UpdateProduct(ctx, repository.UpdateProductParams{
			ID:          id,
			Name:        name,
			Description: description,
			Price:       price,
			Category:    category,
			Status:      status,
			ImageUrl:    imageURL,
			Attributes:  attributes,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductNotFound
			}
			if isUniqueViolation(err) {
				return ErrProductNameAlreadyExists
			}
			return fmt.Errorf("update product: %w", err)
		}

		row = updated
		return nil
	})
	if err != nil {
		return nil, err
	}

	product := productFromRow(row)
	if s.cache != nil {
		_ = s.cache.InvalidateProductLists(ctx)
		_ = s.cache.InvalidateProductDetail(ctx, normalizedProductID)
	}

	return &product, nil
}

func (s *ProductService) UpdateProductStatus(ctx context.Context, productID string, input UpdateProductStatusInput) (*Product, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("product transaction runner missing")
	}
	if input.ActorRole == string(repository.UserRolePEGAWAI) && input.Status == string(repository.ProductStatusUnavailable) {
		return nil, ErrProductStatusForbidden
	}

	var id pgtype.UUID
	if err := id.Scan(productID); err != nil {
		return nil, ErrInvalidProductID
	}
	normalizedProductID := id.String()

	var row repository.Product
	err := s.txRunner.Run(ctx, func(repo productRepository) error {
		existing, err := repo.GetProductByIDIncludingDeleted(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductNotFound
			}
			return fmt.Errorf("get product: %w", err)
		}
		if existing.DeletedAt.Valid {
			return ErrProductAlreadyDeleted
		}

		updated, err := repo.UpdateProductStatus(ctx, repository.UpdateProductStatusParams{
			ID:     id,
			Status: repository.ProductStatus(input.Status),
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductNotFound
			}
			return fmt.Errorf("update product status: %w", err)
		}

		row = updated
		return nil
	})
	if err != nil {
		return nil, err
	}

	product := productFromRow(row)
	if s.cache != nil {
		_ = s.cache.InvalidateProductLists(ctx)
		_ = s.cache.InvalidateProductDetail(ctx, normalizedProductID)
	}

	return &product, nil
}

func (s *ProductService) DeleteProduct(ctx context.Context, productID string) error {
	if s.repo == nil {
		return errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return errors.New("product transaction runner missing")
	}

	var id pgtype.UUID
	if err := id.Scan(productID); err != nil {
		return ErrInvalidProductID
	}
	normalizedProductID := id.String()

	err := s.txRunner.Run(ctx, func(repo productRepository) error {
		existing, err := repo.GetProductByIDIncludingDeleted(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductNotFound
			}
			return fmt.Errorf("get product: %w", err)
		}
		if existing.DeletedAt.Valid {
			return ErrProductAlreadyDeleted
		}

		if _, err := repo.SoftDeleteProduct(ctx, id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductAlreadyDeleted
			}
			return fmt.Errorf("soft delete product: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	if s.cache != nil {
		_ = s.cache.InvalidateProductLists(ctx)
		_ = s.cache.InvalidateProductDetail(ctx, normalizedProductID)
	}

	return nil
}

func (s *ProductService) RestoreProduct(ctx context.Context, productID string) (*Product, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("product transaction runner missing")
	}

	var id pgtype.UUID
	if err := id.Scan(productID); err != nil {
		return nil, ErrInvalidProductID
	}
	normalizedProductID := id.String()

	var row repository.Product
	err := s.txRunner.Run(ctx, func(repo productRepository) error {
		existing, err := repo.GetProductByIDIncludingDeleted(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductNotFound
			}
			return fmt.Errorf("get product: %w", err)
		}
		if !existing.DeletedAt.Valid {
			return ErrProductNotDeleted
		}

		restored, err := repo.RestoreProduct(ctx, id)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrProductNotDeleted
			}
			return fmt.Errorf("restore product: %w", err)
		}

		row = restored
		return nil
	})
	if err != nil {
		return nil, err
	}

	product := productFromRow(row)
	if s.cache != nil {
		_ = s.cache.InvalidateProductLists(ctx)
		_ = s.cache.InvalidateProductDetail(ctx, normalizedProductID)
	}

	return &product, nil
}

// helper
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
		Stock:       row.Stock,
		Rating:      numericFloat64(row.Rating),
		TotalSold:   row.TotalSold,
		Attributes:  row.Attributes,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
		DeletedAt:   timestamptzPtr(row.DeletedAt),
	}
}

func timestamptzPtr(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
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
