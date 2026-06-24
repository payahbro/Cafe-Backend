package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	cafedb "cafeTelkom/internal/db"
	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidCartUserID      = errors.New("invalid cart user id")
	ErrInvalidCartProductID   = errors.New("invalid cart product id")
	ErrInvalidCartItemID      = errors.New("invalid cart item id")
	ErrInvalidCartQuantity    = errors.New("invalid cart quantity")
	ErrCartProductNotFound    = errors.New("cart product not found")
	ErrCartItemNotFound       = errors.New("cart item not found")
	ErrCartProductUnavailable = errors.New("cart product unavailable")
	ErrCartProductOutOfStock  = errors.New("cart product out of stock")
	ErrInvalidCartAttributes  = errors.New("invalid cart attributes")
)

type cartRepository interface {
	GetCartByUserID(ctx context.Context, userID pgtype.UUID) (repository.Cart, error)
	CreateCart(ctx context.Context, userID pgtype.UUID) (repository.Cart, error)
	GetProductByIDIncludingDeleted(ctx context.Context, id pgtype.UUID) (repository.Product, error)
	AddOrIncrementCartItem(ctx context.Context, arg repository.AddOrIncrementCartItemParams) (repository.CartItem, error)
	UpdateCartItemQuantityForUser(ctx context.Context, arg repository.UpdateCartItemQuantityForUserParams) (repository.CartItem, error)
	DeleteCartItemForUser(ctx context.Context, arg repository.DeleteCartItemForUserParams) (pgtype.UUID, error)
	DeleteCartItemsByCartID(ctx context.Context, cartID pgtype.UUID) error
	DeleteCartItemsByIDsReturningCartIDs(ctx context.Context, itemIDs []pgtype.UUID) ([]pgtype.UUID, error)
	TouchCart(ctx context.Context, id pgtype.UUID) error
	ListCartItemsByCartID(ctx context.Context, cartID pgtype.UUID) ([]repository.ListCartItemsByCartIDRow, error)
}

type cartTxRunner interface {
	Run(ctx context.Context, fn func(cartRepository) error) error
}

type CartTxRunner struct {
	db   cafedb.TxBeginner
	repo *repository.Queries
}

type CartService struct {
	repo     cartRepository
	txRunner cartTxRunner
}

type Cart struct {
	ID         *string
	UserID     string
	Items      []CartItem
	GrandTotal int32
	UpdatedAt  *time.Time
}

type CartItem struct {
	ItemID             string
	ProductID          string
	Name               string
	ImageURL           *string
	Price              int32
	Quantity           int32
	Subtotal           int32
	IsAvailable        bool
	SelectedAttributes []byte
}

type AddCartItemInput struct {
	ProductID  string
	Quantity   int32
	Attributes map[string]string
}

type UpdateCartItemInput struct {
	Quantity int32
}

func NewCartService(repo cartRepository, txRunner cartTxRunner) *CartService {
	return &CartService{repo: repo, txRunner: txRunner}
}

func NewCartTxRunner(db cafedb.TxBeginner, repo *repository.Queries) *CartTxRunner {
	if db == nil || repo == nil {
		return nil
	}
	return &CartTxRunner{db: db, repo: repo}
}

func (r *CartTxRunner) Run(ctx context.Context, fn func(cartRepository) error) error {
	if r == nil || r.db == nil || r.repo == nil {
		return errors.New("cart transaction runner missing")
	}

	return cafedb.WithTx(ctx, r.db, func(ctx context.Context, tx pgx.Tx) error {
		return fn(r.repo.WithTx(tx))
	})
}

func (s *CartService) GetCart(ctx context.Context, userID string) (*Cart, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	userUUID, err := parseRequiredUUID(userID)
	if err != nil {
		return nil, ErrInvalidCartUserID
	}

	cart, err := s.repo.GetCartByUserID(ctx, userUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &Cart{
				UserID: userUUID.String(),
				Items:  []CartItem{},
			}, nil
		}
		return nil, fmt.Errorf("get cart: %w", err)
	}

	items, err := s.repo.ListCartItemsByCartID(ctx, cart.ID)
	if err != nil {
		return nil, fmt.Errorf("list cart items: %w", err)
	}

	return cartFromRows(cart, items), nil
}

func (s *CartService) AddItem(ctx context.Context, userID string, input AddCartItemInput) (*Cart, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("cart transaction runner missing")
	}
	if input.Quantity < 1 {
		return nil, ErrInvalidCartQuantity
	}

	userUUID, err := parseRequiredUUID(userID)
	if err != nil {
		return nil, ErrInvalidCartUserID
	}
	productUUID, err := parseRequiredUUID(input.ProductID)
	if err != nil {
		return nil, ErrInvalidCartProductID
	}

	err = s.txRunner.Run(ctx, func(repo cartRepository) error {
		product, err := repo.GetProductByIDIncludingDeleted(ctx, productUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCartProductNotFound
			}
			return fmt.Errorf("get product: %w", err)
		}
		if product.DeletedAt.Valid {
			return ErrCartProductNotFound
		}
		switch product.Status {
		case repository.ProductStatusUnavailable:
			return ErrCartProductUnavailable
		case repository.ProductStatusOutOfStock:
			return ErrCartProductOutOfStock
		}

		selectedAttributes, attributesKey, err := prepareCartItemAttributes(product, input.Attributes)
		if err != nil {
			return err
		}

		cart, err := repo.GetCartByUserID(ctx, userUUID)
		if err != nil {
			if !errors.Is(err, pgx.ErrNoRows) {
				return fmt.Errorf("get cart: %w", err)
			}
			cart, err = repo.CreateCart(ctx, userUUID)
			if err != nil {
				if isUniqueViolation(err) {
					cart, err = repo.GetCartByUserID(ctx, userUUID)
				}
				if err != nil {
					return fmt.Errorf("create cart: %w", err)
				}
			}
		}

		if _, err := repo.AddOrIncrementCartItem(ctx, repository.AddOrIncrementCartItemParams{
			CartID:             cart.ID,
			ProductID:          productUUID,
			Quantity:           input.Quantity,
			SelectedAttributes: selectedAttributes,
			AttributesKey:      attributesKey,
		}); err != nil {
			return fmt.Errorf("add cart item: %w", err)
		}
		if err := repo.TouchCart(ctx, cart.ID); err != nil {
			return fmt.Errorf("touch cart: %w", err)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetCart(ctx, userUUID.String())
}

func (s *CartService) UpdateItemQuantity(ctx context.Context, userID string, itemID string, input UpdateCartItemInput) (*Cart, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("cart transaction runner missing")
	}
	if input.Quantity < 1 {
		return nil, ErrInvalidCartQuantity
	}

	userUUID, err := parseRequiredUUID(userID)
	if err != nil {
		return nil, ErrInvalidCartUserID
	}
	itemUUID, err := parseRequiredUUID(itemID)
	if err != nil {
		return nil, ErrInvalidCartItemID
	}

	err = s.txRunner.Run(ctx, func(repo cartRepository) error {
		updated, err := repo.UpdateCartItemQuantityForUser(ctx, repository.UpdateCartItemQuantityForUserParams{
			ID:       itemUUID,
			Quantity: input.Quantity,
			UserID:   userUUID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCartItemNotFound
			}
			return fmt.Errorf("update cart item quantity: %w", err)
		}
		if err := repo.TouchCart(ctx, updated.CartID); err != nil {
			return fmt.Errorf("touch cart: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.GetCart(ctx, userUUID.String())
}

func (s *CartService) DeleteItem(ctx context.Context, userID string, itemID string) error {
	if s.repo == nil {
		return errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return errors.New("cart transaction runner missing")
	}

	userUUID, err := parseRequiredUUID(userID)
	if err != nil {
		return ErrInvalidCartUserID
	}
	itemUUID, err := parseRequiredUUID(itemID)
	if err != nil {
		return ErrInvalidCartItemID
	}

	return s.txRunner.Run(ctx, func(repo cartRepository) error {
		cartID, err := repo.DeleteCartItemForUser(ctx, repository.DeleteCartItemForUserParams{
			ID:     itemUUID,
			UserID: userUUID,
		})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrCartItemNotFound
			}
			return fmt.Errorf("delete cart item: %w", err)
		}
		if err := repo.TouchCart(ctx, cartID); err != nil {
			return fmt.Errorf("touch cart: %w", err)
		}
		return nil
	})
}

func (s *CartService) ClearItems(ctx context.Context, userID string) error {
	if s.repo == nil {
		return errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return errors.New("cart transaction runner missing")
	}

	userUUID, err := parseRequiredUUID(userID)
	if err != nil {
		return ErrInvalidCartUserID
	}

	return s.txRunner.Run(ctx, func(repo cartRepository) error {
		cart, err := repo.GetCartByUserID(ctx, userUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return fmt.Errorf("get cart: %w", err)
		}
		if err := repo.DeleteCartItemsByCartID(ctx, cart.ID); err != nil {
			return fmt.Errorf("delete cart items: %w", err)
		}
		if err := repo.TouchCart(ctx, cart.ID); err != nil {
			return fmt.Errorf("touch cart: %w", err)
		}
		return nil
	})
}

func (s *CartService) ClearItemsByIDs(ctx context.Context, itemIDs []string) error {
	if s.repo == nil {
		return errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return errors.New("cart transaction runner missing")
	}
	if len(itemIDs) == 0 {
		return ErrInvalidCartItemID
	}

	parsedItemIDs := make([]pgtype.UUID, 0, len(itemIDs))
	for _, itemID := range itemIDs {
		itemUUID, err := parseRequiredUUID(itemID)
		if err != nil {
			return ErrInvalidCartItemID
		}
		parsedItemIDs = append(parsedItemIDs, itemUUID)
	}

	return s.txRunner.Run(ctx, func(repo cartRepository) error {
		cartIDs, err := repo.DeleteCartItemsByIDsReturningCartIDs(ctx, parsedItemIDs)
		if err != nil {
			return fmt.Errorf("delete cart items by ids: %w", err)
		}

		touched := make(map[string]struct{}, len(cartIDs))
		for _, cartID := range cartIDs {
			key := cartID.String()
			if _, ok := touched[key]; ok {
				continue
			}
			touched[key] = struct{}{}
			if err := repo.TouchCart(ctx, cartID); err != nil {
				return fmt.Errorf("touch cart: %w", err)
			}
		}

		return nil
	})
}

func cartFromRows(cart repository.Cart, rows []repository.ListCartItemsByCartIDRow) *Cart {
	cartID := cart.ID.String()
	updatedAt := timestamptzPtr(cart.UpdatedAt)
	result := &Cart{
		ID:        &cartID,
		UserID:    cart.UserID.String(),
		Items:     make([]CartItem, 0, len(rows)),
		UpdatedAt: updatedAt,
	}

	for _, row := range rows {
		isAvailable := row.Status == repository.ProductStatusAvailable && !row.DeletedAt.Valid
		subtotal := row.Price * row.Quantity
		if isAvailable {
			result.GrandTotal += subtotal
		}
		result.Items = append(result.Items, CartItem{
			ItemID:             row.ItemID.String(),
			ProductID:          row.ProductID.String(),
			Name:               row.Name,
			ImageURL:           textPtr(row.ImageUrl),
			Price:              row.Price,
			Quantity:           row.Quantity,
			Subtotal:           subtotal,
			IsAvailable:        isAvailable,
			SelectedAttributes: row.SelectedAttributes,
		})
	}

	return result
}

func prepareCartItemAttributes(product repository.Product, input map[string]string) ([]byte, string, error) {
	raw, err := selectedAttributes(product.Category, product.Attributes, input)
	if err != nil {
		return nil, "", ErrInvalidCartAttributes
	}

	sum := sha256.Sum256(raw)
	return raw, hex.EncodeToString(sum[:]), nil
}

func parseRequiredUUID(value string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		return pgtype.UUID{}, err
	}
	return id, nil
}
