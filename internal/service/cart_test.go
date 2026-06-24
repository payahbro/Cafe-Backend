package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestPrepareCartItemAttributesCanonicalizesSelectedOptions(t *testing.T) {
	product := productRow(
		t,
		"44444444-4444-4444-8444-444444444444",
		"Miso",
		time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
	)
	product.Category = repository.ProductCategoryFood
	product.Attributes = []byte(`{"portions":["regular","large"],"spicy_levels":["no_spicy","hot"]}`)

	raw, key, err := prepareCartItemAttributes(product, map[string]string{
		"spicy_levels": "hot",
		"portions":     "large",
	})
	if err != nil {
		t.Fatalf("prepare attributes: %v", err)
	}

	if got, want := string(raw), `{"portions":"large","spicy_levels":"hot"}`; got != want {
		t.Fatalf("attributes = %s, want %s", got, want)
	}
	if key == "" {
		t.Fatal("attributes key must not be empty")
	}
}

func TestPrepareCartItemAttributesUsesDifferentKeysForDifferentOptions(t *testing.T) {
	product := productRow(
		t,
		"44444444-4444-4444-8444-444444444444",
		"Miso",
		time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC),
	)
	product.Category = repository.ProductCategoryFood
	product.Attributes = []byte(`{"portions":["regular","large"],"spicy_levels":["no_spicy","hot"]}`)

	_, regularKey, err := prepareCartItemAttributes(product, map[string]string{
		"portions":     "regular",
		"spicy_levels": "no_spicy",
	})
	if err != nil {
		t.Fatalf("prepare regular attributes: %v", err)
	}
	_, largeKey, err := prepareCartItemAttributes(product, map[string]string{
		"portions":     "large",
		"spicy_levels": "hot",
	})
	if err != nil {
		t.Fatalf("prepare large attributes: %v", err)
	}

	if regularKey == largeKey {
		t.Fatalf("different options produced the same key %q", regularKey)
	}
}

func TestCartServiceGetCartMapsItemsAndTotalsOnlyAvailableProducts(t *testing.T) {
	updatedAt := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	userID := "11111111-1111-4111-8111-111111111111"
	cartID := "22222222-2222-4222-8222-222222222222"
	repo := &fakeCartRepo{
		cart: cartRow(t, cartID, userID, updatedAt),
		items: []repository.ListCartItemsByCartIDRow{
			cartItemRow(t, "33333333-3333-4333-8333-333333333333", "44444444-4444-4444-8444-444444444444", "Americano", "available", 25000, 2, false),
			cartItemRow(t, "55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666", "Croissant", "out_of_stock", 18000, 1, false),
		},
	}
	service := NewCartService(repo, nil)

	cart, err := service.GetCart(context.Background(), userID)
	if err != nil {
		t.Fatalf("get cart: %v", err)
	}

	if cart.ID == nil || *cart.ID != cartID {
		t.Fatalf("cart id = %v", cart.ID)
	}
	if cart.GrandTotal != 50000 {
		t.Fatalf("grand total = %d", cart.GrandTotal)
	}
	if len(cart.Items) != 2 {
		t.Fatalf("items len = %d", len(cart.Items))
	}
	if !cart.Items[0].IsAvailable {
		t.Fatalf("expected first item available")
	}
	if cart.Items[0].Subtotal != 50000 {
		t.Fatalf("first subtotal = %d", cart.Items[0].Subtotal)
	}
	if cart.Items[1].IsAvailable {
		t.Fatalf("expected out of stock item unavailable")
	}
	if cart.Items[1].Subtotal != 18000 {
		t.Fatalf("second subtotal = %d", cart.Items[1].Subtotal)
	}
}

func TestCartServiceGetCartReturnsEmptyCartWhenNoCartExists(t *testing.T) {
	userID := "11111111-1111-4111-8111-111111111111"
	repo := &fakeCartRepo{getCartErr: pgx.ErrNoRows}
	service := NewCartService(repo, nil)

	cart, err := service.GetCart(context.Background(), userID)
	if err != nil {
		t.Fatalf("get cart: %v", err)
	}

	if cart.ID != nil {
		t.Fatalf("expected nil cart id, got %v", cart.ID)
	}
	if cart.UserID != userID {
		t.Fatalf("user id = %q", cart.UserID)
	}
	if len(cart.Items) != 0 {
		t.Fatalf("items len = %d", len(cart.Items))
	}
	if cart.GrandTotal != 0 {
		t.Fatalf("grand total = %d", cart.GrandTotal)
	}
	if repo.createCartCalled {
		t.Fatalf("GET cart should not create cart")
	}
}

func TestCartServiceAddItemCreatesCartAndReturnsUpdatedCart(t *testing.T) {
	updatedAt := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	userID := "11111111-1111-4111-8111-111111111111"
	cartID := "22222222-2222-4222-8222-222222222222"
	productID := "44444444-4444-4444-8444-444444444444"
	txRepo := &fakeCartRepo{
		getCartErr: pgx.ErrNoRows,
		cart:       cartRow(t, cartID, userID, updatedAt),
		product:    productRow(t, productID, "Americano", updatedAt),
	}
	txRepo.product.Attributes = []byte(`{"temperature":["hot","iced"],"sizes":["small","medium"],"sugar_levels":["normal","less"],"ice_levels":["normal","less"]}`)
	baseRepo := &fakeCartRepo{
		cart: cartRow(t, cartID, userID, updatedAt),
		items: []repository.ListCartItemsByCartIDRow{
			cartItemRow(t, "33333333-3333-4333-8333-333333333333", productID, "Americano", "available", 25000, 2, false),
		},
	}
	txRunner := &fakeCartTxRunner{repo: txRepo}
	service := NewCartService(baseRepo, txRunner)

	cart, err := service.AddItem(context.Background(), userID, AddCartItemInput{
		ProductID: productID,
		Quantity:  2,
		Attributes: map[string]string{
			"temperature": "iced",
			"sizes":       "medium",
		},
	})
	if err != nil {
		t.Fatalf("add item: %v", err)
	}

	if !txRunner.called {
		t.Fatalf("expected transaction runner to be called")
	}
	if !txRepo.createCartCalled {
		t.Fatalf("expected cart to be created when missing")
	}
	if !txRepo.addItemCalled {
		t.Fatalf("expected item to be added")
	}
	if txRepo.addItemArg.Quantity != 2 {
		t.Fatalf("added quantity = %d", txRepo.addItemArg.Quantity)
	}
	if txRepo.addItemArg.ProductID.String() != productID {
		t.Fatalf("added product id = %q", txRepo.addItemArg.ProductID.String())
	}
	if got, want := string(txRepo.addItemArg.SelectedAttributes), `{"ice_levels":"normal","sizes":"medium","sugar_levels":"normal","temperature":"iced"}`; got != want {
		t.Fatalf("selected attributes = %s, want %s", got, want)
	}
	if txRepo.addItemArg.AttributesKey == "" {
		t.Fatal("attributes key must not be empty")
	}
	if !txRepo.touchCartCalled {
		t.Fatalf("expected cart updated_at to be touched")
	}
	if len(cart.Items) != 1 || cart.GrandTotal != 50000 {
		t.Fatalf("cart = %+v", cart)
	}
}

func TestCartServiceAddItemRejectsOutOfStockProduct(t *testing.T) {
	updatedAt := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	productID := "44444444-4444-4444-8444-444444444444"
	product := productRow(t, productID, "Americano", updatedAt)
	product.Status = repository.ProductStatusOutOfStock
	txRepo := &fakeCartRepo{product: product}
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: txRepo})

	_, err := service.AddItem(context.Background(), "11111111-1111-4111-8111-111111111111", AddCartItemInput{
		ProductID: productID,
		Quantity:  1,
	})
	if !errors.Is(err, ErrCartProductOutOfStock) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.addItemCalled {
		t.Fatalf("item should not be added for out of stock product")
	}
}

func TestCartServiceAddItemRejectsUnavailableProduct(t *testing.T) {
	updatedAt := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	productID := "44444444-4444-4444-8444-444444444444"
	product := productRow(t, productID, "Americano", updatedAt)
	product.Status = repository.ProductStatusUnavailable
	txRepo := &fakeCartRepo{product: product}
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: txRepo})

	_, err := service.AddItem(context.Background(), "11111111-1111-4111-8111-111111111111", AddCartItemInput{
		ProductID: productID,
		Quantity:  1,
	})
	if !errors.Is(err, ErrCartProductUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestCartServiceUpdateItemQuantityUpdatesFinalQuantityInTransaction(t *testing.T) {
	updatedAt := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	userID := "11111111-1111-4111-8111-111111111111"
	cartID := "22222222-2222-4222-8222-222222222222"
	itemID := "33333333-3333-4333-8333-333333333333"
	productID := "44444444-4444-4444-8444-444444444444"
	txRepo := &fakeCartRepo{
		updatedItem: repository.CartItem{
			ID:     mustUUID(t, itemID),
			CartID: mustUUID(t, cartID),
		},
	}
	baseRepo := &fakeCartRepo{
		cart: cartRow(t, cartID, userID, updatedAt),
		items: []repository.ListCartItemsByCartIDRow{
			cartItemRow(t, itemID, productID, "Americano", "available", 25000, 3, false),
		},
	}
	txRunner := &fakeCartTxRunner{repo: txRepo}
	service := NewCartService(baseRepo, txRunner)

	cart, err := service.UpdateItemQuantity(context.Background(), userID, itemID, UpdateCartItemInput{Quantity: 3})
	if err != nil {
		t.Fatalf("update item quantity: %v", err)
	}

	if !txRunner.called {
		t.Fatalf("expected transaction runner to be called")
	}
	if !txRepo.updateItemCalled {
		t.Fatalf("expected item quantity to be updated")
	}
	if txRepo.updateItemArg.Quantity != 3 {
		t.Fatalf("quantity = %d", txRepo.updateItemArg.Quantity)
	}
	if txRepo.updateItemArg.UserID.String() != userID {
		t.Fatalf("user id = %q", txRepo.updateItemArg.UserID.String())
	}
	if !txRepo.touchCartCalled {
		t.Fatalf("expected cart to be touched")
	}
	if len(cart.Items) != 1 || cart.Items[0].Quantity != 3 {
		t.Fatalf("cart = %+v", cart)
	}
}

func TestCartServiceUpdateItemQuantityRejectsZero(t *testing.T) {
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: &fakeCartRepo{}})

	_, err := service.UpdateItemQuantity(context.Background(), "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333", UpdateCartItemInput{Quantity: 0})
	if !errors.Is(err, ErrInvalidCartQuantity) {
		t.Fatalf("err = %v", err)
	}
}

func TestCartServiceUpdateItemQuantityReturnsNotFoundForOtherUsersItem(t *testing.T) {
	txRepo := &fakeCartRepo{updateItemErr: pgx.ErrNoRows}
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: txRepo})

	_, err := service.UpdateItemQuantity(context.Background(), "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333", UpdateCartItemInput{Quantity: 3})
	if !errors.Is(err, ErrCartItemNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestCartServiceDeleteItemDeletesOwnedItemAndTouchesCart(t *testing.T) {
	userID := "11111111-1111-4111-8111-111111111111"
	cartID := "22222222-2222-4222-8222-222222222222"
	itemID := "33333333-3333-4333-8333-333333333333"
	txRepo := &fakeCartRepo{
		deletedCartID: mustUUID(t, cartID),
	}
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: txRepo})

	err := service.DeleteItem(context.Background(), userID, itemID)
	if err != nil {
		t.Fatalf("delete item: %v", err)
	}

	if !txRepo.deleteItemCalled {
		t.Fatalf("expected item to be deleted")
	}
	if txRepo.deleteItemArg.ID.String() != itemID {
		t.Fatalf("item id = %q", txRepo.deleteItemArg.ID.String())
	}
	if txRepo.deleteItemArg.UserID.String() != userID {
		t.Fatalf("user id = %q", txRepo.deleteItemArg.UserID.String())
	}
	if !txRepo.touchCartCalled {
		t.Fatalf("expected cart to be touched")
	}
}

func TestCartServiceDeleteItemReturnsNotFoundForOtherUsersItem(t *testing.T) {
	txRepo := &fakeCartRepo{deleteItemErr: pgx.ErrNoRows}
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: txRepo})

	err := service.DeleteItem(context.Background(), "11111111-1111-4111-8111-111111111111", "33333333-3333-4333-8333-333333333333")
	if !errors.Is(err, ErrCartItemNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestCartServiceClearItemsDeletesAllItemsWhenCartExists(t *testing.T) {
	userID := "11111111-1111-4111-8111-111111111111"
	cartID := "22222222-2222-4222-8222-222222222222"
	updatedAt := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	txRepo := &fakeCartRepo{
		cart: cartRow(t, cartID, userID, updatedAt),
	}
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: txRepo})

	err := service.ClearItems(context.Background(), userID)
	if err != nil {
		t.Fatalf("clear items: %v", err)
	}

	if !txRepo.clearItemsCalled {
		t.Fatalf("expected cart items to be cleared")
	}
	if txRepo.clearCartID.String() != cartID {
		t.Fatalf("cart id = %q", txRepo.clearCartID.String())
	}
	if !txRepo.touchCartCalled {
		t.Fatalf("expected cart to be touched")
	}
}

func TestCartServiceClearItemsIsIdempotentWhenCartDoesNotExist(t *testing.T) {
	txRepo := &fakeCartRepo{getCartErr: pgx.ErrNoRows}
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: txRepo})

	err := service.ClearItems(context.Background(), "11111111-1111-4111-8111-111111111111")
	if err != nil {
		t.Fatalf("clear items: %v", err)
	}
	if txRepo.clearItemsCalled {
		t.Fatalf("clear should not run when cart does not exist")
	}
}

func TestCartServiceClearItemsByIDsDeletesItemsAndTouchesAffectedCarts(t *testing.T) {
	itemIDs := []string{
		"33333333-3333-4333-8333-333333333333",
		"55555555-5555-4555-8555-555555555555",
	}
	firstCartID := mustUUID(t, "22222222-2222-4222-8222-222222222222")
	secondCartID := mustUUID(t, "66666666-6666-4666-8666-666666666666")
	txRepo := &fakeCartRepo{
		deletedCartIDs: []pgtype.UUID{firstCartID, firstCartID, secondCartID},
	}
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: txRepo})

	err := service.ClearItemsByIDs(context.Background(), itemIDs)
	if err != nil {
		t.Fatalf("clear items by ids: %v", err)
	}

	if !txRepo.deleteItemsByIDsCalled {
		t.Fatalf("expected items to be deleted by ids")
	}
	if len(txRepo.deleteItemsIDs) != 2 {
		t.Fatalf("delete ids len = %d", len(txRepo.deleteItemsIDs))
	}
	if len(txRepo.touchedCartIDs) != 2 {
		t.Fatalf("touched cart ids len = %d", len(txRepo.touchedCartIDs))
	}
	if txRepo.touchedCartIDs[0].String() != firstCartID.String() || txRepo.touchedCartIDs[1].String() != secondCartID.String() {
		t.Fatalf("touched cart ids = %v", txRepo.touchedCartIDs)
	}
}

func TestCartServiceClearItemsByIDsIsIdempotentWhenNoItemsFound(t *testing.T) {
	txRepo := &fakeCartRepo{}
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: txRepo})

	err := service.ClearItemsByIDs(context.Background(), []string{"33333333-3333-4333-8333-333333333333"})
	if err != nil {
		t.Fatalf("clear items by ids: %v", err)
	}
	if len(txRepo.touchedCartIDs) != 0 {
		t.Fatalf("expected no touched carts, got %d", len(txRepo.touchedCartIDs))
	}
}

func TestCartServiceClearItemsByIDsRejectsEmptyInput(t *testing.T) {
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: &fakeCartRepo{}})

	err := service.ClearItemsByIDs(context.Background(), nil)
	if !errors.Is(err, ErrInvalidCartItemID) {
		t.Fatalf("err = %v", err)
	}
}

func TestCartServiceClearItemsByIDsRejectsInvalidID(t *testing.T) {
	service := NewCartService(&fakeCartRepo{}, &fakeCartTxRunner{repo: &fakeCartRepo{}})

	err := service.ClearItemsByIDs(context.Background(), []string{"not-a-uuid"})
	if !errors.Is(err, ErrInvalidCartItemID) {
		t.Fatalf("err = %v", err)
	}
}

type fakeCartTxRunner struct {
	repo   cartRepository
	called bool
	err    error
}

func (f *fakeCartTxRunner) Run(ctx context.Context, fn func(cartRepository) error) error {
	f.called = true
	if f.err != nil {
		return f.err
	}
	return fn(f.repo)
}

type fakeCartRepo struct {
	cart                   repository.Cart
	product                repository.Product
	items                  []repository.ListCartItemsByCartIDRow
	getCartErr             error
	productErr             error
	listItemsErr           error
	updateItemErr          error
	deleteItemErr          error
	addItemArg             repository.AddOrIncrementCartItemParams
	updateItemArg          repository.UpdateCartItemQuantityForUserParams
	deleteItemArg          repository.DeleteCartItemForUserParams
	updatedItem            repository.CartItem
	deletedCartID          pgtype.UUID
	deletedCartIDs         []pgtype.UUID
	clearCartID            pgtype.UUID
	deleteItemsIDs         []pgtype.UUID
	touchedCartIDs         []pgtype.UUID
	createCartCalled       bool
	addItemCalled          bool
	touchCartCalled        bool
	updateItemCalled       bool
	deleteItemCalled       bool
	clearItemsCalled       bool
	deleteItemsByIDsCalled bool
}

func (f *fakeCartRepo) GetCartByUserID(ctx context.Context, userID pgtype.UUID) (repository.Cart, error) {
	if f.getCartErr != nil {
		return repository.Cart{}, f.getCartErr
	}
	return f.cart, nil
}

func (f *fakeCartRepo) CreateCart(ctx context.Context, userID pgtype.UUID) (repository.Cart, error) {
	f.createCartCalled = true
	if f.getCartErr != nil && !errors.Is(f.getCartErr, pgx.ErrNoRows) {
		return repository.Cart{}, f.getCartErr
	}
	return f.cart, nil
}

func (f *fakeCartRepo) GetProductByIDIncludingDeleted(ctx context.Context, id pgtype.UUID) (repository.Product, error) {
	if f.productErr != nil {
		return repository.Product{}, f.productErr
	}
	return f.product, nil
}

func (f *fakeCartRepo) AddOrIncrementCartItem(ctx context.Context, arg repository.AddOrIncrementCartItemParams) (repository.CartItem, error) {
	f.addItemCalled = true
	f.addItemArg = arg
	return repository.CartItem{}, nil
}

func (f *fakeCartRepo) TouchCart(ctx context.Context, id pgtype.UUID) error {
	f.touchCartCalled = true
	f.touchedCartIDs = append(f.touchedCartIDs, id)
	return nil
}

func (f *fakeCartRepo) ListCartItemsByCartID(ctx context.Context, cartID pgtype.UUID) ([]repository.ListCartItemsByCartIDRow, error) {
	if f.listItemsErr != nil {
		return nil, f.listItemsErr
	}
	return f.items, nil
}

func (f *fakeCartRepo) UpdateCartItemQuantityForUser(ctx context.Context, arg repository.UpdateCartItemQuantityForUserParams) (repository.CartItem, error) {
	f.updateItemCalled = true
	f.updateItemArg = arg
	if f.updateItemErr != nil {
		return repository.CartItem{}, f.updateItemErr
	}
	return f.updatedItem, nil
}

func (f *fakeCartRepo) DeleteCartItemForUser(ctx context.Context, arg repository.DeleteCartItemForUserParams) (pgtype.UUID, error) {
	f.deleteItemCalled = true
	f.deleteItemArg = arg
	if f.deleteItemErr != nil {
		return pgtype.UUID{}, f.deleteItemErr
	}
	return f.deletedCartID, nil
}

func (f *fakeCartRepo) DeleteCartItemsByCartID(ctx context.Context, cartID pgtype.UUID) error {
	f.clearItemsCalled = true
	f.clearCartID = cartID
	return nil
}

func (f *fakeCartRepo) DeleteCartItemsByIDsReturningCartIDs(ctx context.Context, itemIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	f.deleteItemsByIDsCalled = true
	f.deleteItemsIDs = itemIDs
	return f.deletedCartIDs, nil
}

func cartRow(t *testing.T, cartID, userID string, updatedAt time.Time) repository.Cart {
	t.Helper()

	var cartUUID pgtype.UUID
	if err := cartUUID.Scan(cartID); err != nil {
		t.Fatalf("scan cart uuid: %v", err)
	}
	var userUUID pgtype.UUID
	if err := userUUID.Scan(userID); err != nil {
		t.Fatalf("scan user uuid: %v", err)
	}

	return repository.Cart{
		ID:        cartUUID,
		UserID:    userUUID,
		CreatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true},
		UpdatedAt: pgtype.Timestamptz{Time: updatedAt, Valid: true},
	}
}

func cartItemRow(t *testing.T, itemID, productID, name, status string, price, quantity int32, deleted bool) repository.ListCartItemsByCartIDRow {
	t.Helper()

	var itemUUID pgtype.UUID
	if err := itemUUID.Scan(itemID); err != nil {
		t.Fatalf("scan item uuid: %v", err)
	}
	var productUUID pgtype.UUID
	if err := productUUID.Scan(productID); err != nil {
		t.Fatalf("scan product uuid: %v", err)
	}

	row := repository.ListCartItemsByCartIDRow{
		ItemID:             itemUUID,
		ProductID:          productUUID,
		SelectedAttributes: []byte(`{"sizes":"medium","temperature":"iced"}`),
		Name:               name,
		ImageUrl:           pgtype.Text{String: "https://example.supabase.co/storage/v1/object/public/products/" + name + ".png", Valid: true},
		Price:              price,
		Quantity:           quantity,
		Status:             repository.ProductStatus(status),
	}
	if deleted {
		row.DeletedAt = pgtype.Timestamptz{Time: time.Date(2026, 6, 13, 11, 0, 0, 0, time.UTC), Valid: true}
	}
	return row
}

func mustUUID(t *testing.T, value string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}
