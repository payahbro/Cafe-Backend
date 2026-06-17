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

func TestOrderServiceCheckoutCreatesPendingOrderWithSnapshotsAndDefaults(t *testing.T) {
	now := time.Date(2026, 6, 14, 3, 0, 0, 0, time.UTC)
	userID := "11111111-1111-4111-8111-111111111111"
	cartItemID := "22222222-2222-4222-8222-222222222222"
	productID := "33333333-3333-4333-8333-333333333333"
	orderID := "44444444-4444-4444-8444-444444444444"
	orderItemID := "55555555-5555-4555-8555-555555555555"
	txRepo := &fakeOrderRepo{
		checkoutRows: []repository.ListCheckoutCartItemsForUserRow{
			checkoutCartItemRow(t, cartItemID, productID, "Americano", "coffee", "available", 25000, 2, 10, false, `{"temperature":["hot","iced"],"sizes":["small","medium"],"sugar_levels":["normal","less"],"ice_levels":["normal","less"]}`),
		},
		lockedProducts: map[string]repository.Product{
			productID: orderProductRow(t, productID, "Americano", "coffee", "available", 25000, 10, false, `{"temperature":["hot","iced"],"sizes":["small","medium"],"sugar_levels":["normal","less"],"ice_levels":["normal","less"]}`),
		},
		createdOrder: repository.Order{
			ID:          mustUUIDForOrder(t, orderID),
			OrderNumber: "ORD-20260614-001",
			UserID:      mustUUIDForOrder(t, userID),
			Status:      repository.OrderStatusPENDING,
			Notes:       pgtype.Text{String: "Tolong bungkus rapi", Valid: true},
			TotalAmount: 50000,
			ExpiresAt:   pgtype.Timestamptz{Time: now.Add(15 * time.Minute), Valid: true},
			CreatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
			UpdatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
		},
		createdItems: []repository.OrderItem{
			{
				ID:                 mustUUIDForOrder(t, orderItemID),
				OrderID:            mustUUIDForOrder(t, orderID),
				ProductID:          mustUUIDForOrder(t, productID),
				CartItemID:         mustUUIDForOrder(t, cartItemID),
				ProductName:        "Americano",
				PriceAtCheckout:    25000,
				Quantity:           2,
				Subtotal:           50000,
				SelectedAttributes: []byte(`{"ice_levels":"normal","sizes":"medium","sugar_levels":"normal","temperature":"iced"}`),
				CreatedAt:          pgtype.Timestamptz{Time: now, Valid: true},
			},
		},
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, func() time.Time { return now })

	order, err := service.Checkout(context.Background(), CheckoutInput{
		UserID:      userID,
		IsVerified:  true,
		PhoneNumber: "+628123456789",
		Notes:       stringPtrForOrder("Tolong bungkus rapi"),
		Items: []CheckoutItemInput{
			{
				CartItemID: cartItemID,
				Attributes: map[string]string{
					"temperature": "iced",
					"sizes":       "medium",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	if order.OrderNumber != "ORD-20260614-001" {
		t.Fatalf("order number = %q", order.OrderNumber)
	}
	if order.TotalAmount != 50000 {
		t.Fatalf("total amount = %d", order.TotalAmount)
	}
	if len(order.Items) != 1 {
		t.Fatalf("items len = %d", len(order.Items))
	}
	if !txRepo.decrementCalled {
		t.Fatalf("expected stock to be decremented")
	}
	if txRepo.decrementArg.Quantity != 2 {
		t.Fatalf("decrement quantity = %d", txRepo.decrementArg.Quantity)
	}
	wantAttributes := `{"ice_levels":"normal","sizes":"medium","sugar_levels":"normal","temperature":"iced"}`
	if string(txRepo.createItemArgs[0].SelectedAttributes) != wantAttributes {
		t.Fatalf("selected attributes = %s", txRepo.createItemArgs[0].SelectedAttributes)
	}
}

func TestOrderServiceCheckoutRejectsUnverifiedCustomer(t *testing.T) {
	service := NewOrderService(&fakeOrderRepo{}, &fakeOrderTxRunner{repo: &fakeOrderRepo{}}, time.Now)

	_, err := service.Checkout(context.Background(), CheckoutInput{
		UserID:      "11111111-1111-4111-8111-111111111111",
		IsVerified:  false,
		PhoneNumber: "+628123456789",
		Items: []CheckoutItemInput{{
			CartItemID: "22222222-2222-4222-8222-222222222222",
		}},
	})
	if !errors.Is(err, ErrOrderEmailUnverified) {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderServiceCheckoutRejectsMissingRequiredCoffeeAttribute(t *testing.T) {
	userID := "11111111-1111-4111-8111-111111111111"
	cartItemID := "22222222-2222-4222-8222-222222222222"
	productID := "33333333-3333-4333-8333-333333333333"
	txRepo := &fakeOrderRepo{
		checkoutRows: []repository.ListCheckoutCartItemsForUserRow{
			checkoutCartItemRow(t, cartItemID, productID, "Americano", "coffee", "available", 25000, 1, 10, false, `{"temperature":["hot","iced"],"sizes":["small","medium"],"sugar_levels":["normal"],"ice_levels":["normal"]}`),
		},
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, time.Now)

	_, err := service.Checkout(context.Background(), CheckoutInput{
		UserID:      userID,
		IsVerified:  true,
		PhoneNumber: "+628123456789",
		Items: []CheckoutItemInput{{
			CartItemID: cartItemID,
			Attributes: map[string]string{
				"temperature": "iced",
			},
		}},
	})
	if !errors.Is(err, ErrOrderValidation) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.createOrderCalled {
		t.Fatalf("order should not be created for invalid attributes")
	}
}

func TestOrderServiceCheckoutReturnsCartItemNotFoundWhenAnyItemMissing(t *testing.T) {
	service := NewOrderService(&fakeOrderRepo{}, &fakeOrderTxRunner{repo: &fakeOrderRepo{}}, time.Now)

	_, err := service.Checkout(context.Background(), CheckoutInput{
		UserID:      "11111111-1111-4111-8111-111111111111",
		IsVerified:  true,
		PhoneNumber: "+628123456789",
		Items: []CheckoutItemInput{{
			CartItemID: "22222222-2222-4222-8222-222222222222",
		}},
	})
	if !errors.Is(err, ErrOrderCartItemNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderServiceCheckoutRejectsCartItemAlreadyInPendingOrder(t *testing.T) {
	userID := "11111111-1111-4111-8111-111111111111"
	cartItemID := "22222222-2222-4222-8222-222222222222"
	productID := "33333333-3333-4333-8333-333333333333"
	txRepo := &fakeOrderRepo{
		checkoutRows: []repository.ListCheckoutCartItemsForUserRow{
			checkoutCartItemRow(t, cartItemID, productID, "Americano", "coffee", "available", 25000, 1, 10, false, `{"temperature":["hot","iced"],"sizes":["small","medium"],"sugar_levels":["normal"],"ice_levels":["normal"]}`),
		},
		pendingCartItemCount: 1,
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, time.Now)

	_, err := service.Checkout(context.Background(), CheckoutInput{
		UserID:      userID,
		IsVerified:  true,
		PhoneNumber: "+628123456789",
		Items: []CheckoutItemInput{{
			CartItemID: cartItemID,
			Attributes: map[string]string{
				"temperature": "iced",
				"sizes":       "medium",
			},
		}},
	})
	if !errors.Is(err, ErrOrderCartItemAlreadyPending) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.createOrderCalled {
		t.Fatalf("order should not be created for cart item already pending")
	}
}

func TestOrderServiceCheckoutCancelsExpiredPendingOrderBeforePendingCheck(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	userID := "11111111-1111-4111-8111-111111111111"
	cartItemID := "22222222-2222-4222-8222-222222222222"
	productID := "33333333-3333-4333-8333-333333333333"
	expiredOrderID := "44444444-4444-4444-8444-444444444444"
	newOrderID := "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	expiredOrder := orderRow(t, expiredOrderID, userID, "ORD-20260617-001", "PENDING", 25000, now.Add(-time.Minute))
	expiredOrder.ExpiresAt = pgtype.Timestamptz{Time: now.Add(-time.Minute), Valid: true}
	txRepo := &fakeOrderRepo{
		checkoutRows: []repository.ListCheckoutCartItemsForUserRow{
			checkoutCartItemRow(t, cartItemID, productID, "Americano", "coffee", "available", 25000, 1, 10, false, `{"temperature":["iced"],"sizes":["medium"],"sugar_levels":["normal"],"ice_levels":["normal"]}`),
		},
		expiredOrders: []repository.Order{
			expiredOrder,
		},
		order: expiredOrder,
		orderItems: []repository.OrderItem{
			orderItemRow(t, "55555555-5555-4555-8555-555555555555", expiredOrderID, productID, cartItemID, 1, 25000, now.Add(-16*time.Minute)),
		},
		lockedProducts: map[string]repository.Product{
			productID: orderProductRow(t, productID, "Americano", "coffee", "available", 25000, 10, false, `{"temperature":["iced"],"sizes":["medium"],"sugar_levels":["normal"],"ice_levels":["normal"]}`),
		},
		createdOrder: repository.Order{
			ID:          mustUUIDForOrder(t, newOrderID),
			OrderNumber: "ORD-20260617-002",
			UserID:      mustUUIDForOrder(t, userID),
			Status:      repository.OrderStatusPENDING,
			TotalAmount: 25000,
			ExpiresAt:   pgtype.Timestamptz{Time: now.Add(15 * time.Minute), Valid: true},
			CreatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
			UpdatedAt:   pgtype.Timestamptz{Time: now, Valid: true},
		},
		createdItems: []repository.OrderItem{
			orderItemRow(t, "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", newOrderID, productID, cartItemID, 1, 25000, now),
		},
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, func() time.Time { return now })

	order, err := service.Checkout(context.Background(), CheckoutInput{
		UserID:      userID,
		IsVerified:  true,
		PhoneNumber: "+628123456789",
		Items: []CheckoutItemInput{{
			CartItemID: cartItemID,
			Attributes: map[string]string{
				"temperature": "iced",
				"sizes":       "medium",
			},
		}},
	})
	if err != nil {
		t.Fatalf("checkout: %v", err)
	}

	if order.ID != newOrderID {
		t.Fatalf("new order id = %q", order.ID)
	}
	if !txRepo.incrementStockCalled {
		t.Fatalf("expected expired order stock to be restored")
	}
	if !txRepo.expiredCancelCalled {
		t.Fatalf("expected expired order to be cancelled")
	}
	if !txRepo.createOrderCalled {
		t.Fatalf("expected new order to be created after lazy cancel")
	}
}

func TestOrderServiceListOrdersRestrictsCustomerToOwnOrders(t *testing.T) {
	userID := "11111111-1111-4111-8111-111111111111"
	repo := &fakeOrderRepo{
		listRows: []repository.ListOrdersRow{
			orderListRow(t, "44444444-4444-4444-8444-444444444444", userID, "ORD-20260614-001", "PENDING", 50000, time.Date(2026, 6, 14, 3, 0, 0, 0, time.UTC)),
		},
	}
	service := NewOrderService(repo, nil, time.Now)

	list, err := service.ListOrders(context.Background(), ListOrdersInput{
		ActorUserID: userID,
		ActorRole:   string(repository.UserRoleCUSTOMER),
		Limit:       10,
		UserID:      "99999999-9999-4999-8999-999999999999",
	})
	if err != nil {
		t.Fatalf("list orders: %v", err)
	}

	if repo.listArg.UserID.String() != userID {
		t.Fatalf("user filter = %q", repo.listArg.UserID.String())
	}
	if len(list.Items) != 1 || list.Items[0].OrderNumber != "ORD-20260614-001" {
		t.Fatalf("list = %+v", list)
	}
}

func TestOrderServiceGetOrderReturnsNotFoundForOtherCustomersOrder(t *testing.T) {
	orderID := "44444444-4444-4444-8444-444444444444"
	repo := &fakeOrderRepo{
		order: repository.Order{
			ID:     mustUUIDForOrder(t, orderID),
			UserID: mustUUIDForOrder(t, "99999999-9999-4999-8999-999999999999"),
		},
	}
	service := NewOrderService(repo, nil, time.Now)

	_, err := service.GetOrder(context.Background(), GetOrderInput{
		OrderID:     orderID,
		ActorUserID: "11111111-1111-4111-8111-111111111111",
		ActorRole:   string(repository.UserRoleCUSTOMER),
	})
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderServiceCancelOrderRestoresStockAndCancelsPendingOrder(t *testing.T) {
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	orderID := "44444444-4444-4444-8444-444444444444"
	userID := "11111111-1111-4111-8111-111111111111"
	productID := "33333333-3333-4333-8333-333333333333"
	txRepo := &fakeOrderRepo{
		order: orderRow(t, orderID, userID, "ORD-20260615-001", "PENDING", 50000, now),
		orderItems: []repository.OrderItem{
			orderItemRow(t, "55555555-5555-4555-8555-555555555555", orderID, productID, "22222222-2222-4222-8222-222222222222", 2, 50000, now),
		},
		lockedProducts: map[string]repository.Product{
			productID: orderProductRow(t, productID, "Americano", "coffee", "available", 25000, 8, false, `{}`),
		},
		updatedOrder: orderRow(t, orderID, userID, "ORD-20260615-001", "CANCELLED", 50000, now.Add(time.Minute)),
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, time.Now)

	result, err := service.CancelOrder(context.Background(), CancelOrderInput{
		OrderID:     orderID,
		ActorUserID: userID,
		ActorRole:   string(repository.UserRoleCUSTOMER),
	})
	if err != nil {
		t.Fatalf("cancel order: %v", err)
	}

	if result.Status != "CANCELLED" {
		t.Fatalf("status = %q", result.Status)
	}
	if !txRepo.incrementStockCalled {
		t.Fatalf("expected stock restore")
	}
	if txRepo.incrementStockArg.Quantity != 2 {
		t.Fatalf("restore quantity = %d", txRepo.incrementStockArg.Quantity)
	}
	if txRepo.updateStatusArg.Status != repository.OrderStatusCANCELLED {
		t.Fatalf("updated status = %q", txRepo.updateStatusArg.Status)
	}
}

func TestOrderServiceCancelOrderReturnsNotFoundForOtherCustomersOrder(t *testing.T) {
	orderID := "44444444-4444-4444-8444-444444444444"
	txRepo := &fakeOrderRepo{
		order: orderRow(t, orderID, "99999999-9999-4999-8999-999999999999", "ORD-20260615-001", "PENDING", 50000, time.Now()),
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, time.Now)

	_, err := service.CancelOrder(context.Background(), CancelOrderInput{
		OrderID:     orderID,
		ActorUserID: "11111111-1111-4111-8111-111111111111",
		ActorRole:   string(repository.UserRoleCUSTOMER),
	})
	if !errors.Is(err, ErrOrderNotFound) {
		t.Fatalf("err = %v", err)
	}
	if txRepo.updateStatusCalled {
		t.Fatalf("order should not be cancelled")
	}
}

func TestOrderServiceCancelOrderRejectsConfirmedOrder(t *testing.T) {
	orderID := "44444444-4444-4444-8444-444444444444"
	userID := "11111111-1111-4111-8111-111111111111"
	txRepo := &fakeOrderRepo{
		order: orderRow(t, orderID, userID, "ORD-20260615-001", "CONFIRMED", 50000, time.Now()),
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, time.Now)

	_, err := service.CancelOrder(context.Background(), CancelOrderInput{
		OrderID:     orderID,
		ActorUserID: userID,
		ActorRole:   string(repository.UserRoleCUSTOMER),
	})
	if !errors.Is(err, ErrOrderNotCancellable) {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderServiceUpdateStatusCompletesConfirmedOrderAndUpdatesTotalSold(t *testing.T) {
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	orderID := "44444444-4444-4444-8444-444444444444"
	userID := "11111111-1111-4111-8111-111111111111"
	productID := "33333333-3333-4333-8333-333333333333"
	txRepo := &fakeOrderRepo{
		order: orderRow(t, orderID, userID, "ORD-20260615-001", "CONFIRMED", 50000, now),
		orderItems: []repository.OrderItem{
			orderItemRow(t, "55555555-5555-4555-8555-555555555555", orderID, productID, "22222222-2222-4222-8222-222222222222", 2, 50000, now),
		},
		updatedOrder: orderRow(t, orderID, userID, "ORD-20260615-001", "COMPLETED", 50000, now.Add(time.Minute)),
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, time.Now)

	result, err := service.UpdateOrderStatus(context.Background(), UpdateOrderStatusInput{
		OrderID:   orderID,
		ActorRole: string(repository.UserRolePEGAWAI),
		Status:    "COMPLETED",
	})
	if err != nil {
		t.Fatalf("update order status: %v", err)
	}

	if result.Status != "COMPLETED" {
		t.Fatalf("status = %q", result.Status)
	}
	if !txRepo.incrementTotalSoldCalled {
		t.Fatalf("expected total_sold update")
	}
	if txRepo.incrementTotalSoldArg.Quantity != 2 {
		t.Fatalf("total_sold quantity = %d", txRepo.incrementTotalSoldArg.Quantity)
	}
}

func TestOrderServiceUpdateStatusRejectsAdminPendingToConfirmedForMVP(t *testing.T) {
	orderID := "44444444-4444-4444-8444-444444444444"
	txRepo := &fakeOrderRepo{
		order: orderRow(t, orderID, "11111111-1111-4111-8111-111111111111", "ORD-20260615-001", "PENDING", 50000, time.Now()),
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, time.Now)

	_, err := service.UpdateOrderStatus(context.Background(), UpdateOrderStatusInput{
		OrderID:   orderID,
		ActorRole: string(repository.UserRoleADMIN),
		Status:    "CONFIRMED",
	})
	if !errors.Is(err, ErrOrderInvalidStatusTransition) {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderServiceConfirmOrderFromPaymentConfirmsPendingAndReturnsCartItemIDs(t *testing.T) {
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	orderID := "44444444-4444-4444-8444-444444444444"
	userID := "11111111-1111-4111-8111-111111111111"
	productID := "33333333-3333-4333-8333-333333333333"
	cartItemID := "22222222-2222-4222-8222-222222222222"
	txRepo := &fakeOrderRepo{
		order: orderRow(t, orderID, userID, "ORD-20260615-001", "PENDING", 50000, now),
		orderItems: []repository.OrderItem{
			orderItemRow(t, "55555555-5555-4555-8555-555555555555", orderID, productID, cartItemID, 2, 50000, now),
		},
		updatedOrder: orderRow(t, orderID, userID, "ORD-20260615-001", "CONFIRMED", 50000, now.Add(time.Minute)),
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, time.Now)

	result, err := service.ConfirmOrderFromPayment(context.Background(), InternalConfirmOrderInput{OrderID: orderID})
	if err != nil {
		t.Fatalf("confirm order from payment: %v", err)
	}

	if result.Status != "CONFIRMED" {
		t.Fatalf("status = %q", result.Status)
	}
	if len(result.CartItemIDs) != 1 || result.CartItemIDs[0] != cartItemID {
		t.Fatalf("cart item ids = %v", result.CartItemIDs)
	}
}

func TestOrderServiceConfirmOrderFromPaymentIsIdempotentForConfirmedOrder(t *testing.T) {
	now := time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC)
	orderID := "44444444-4444-4444-8444-444444444444"
	cartItemID := "22222222-2222-4222-8222-222222222222"
	txRepo := &fakeOrderRepo{
		order: orderRow(t, orderID, "11111111-1111-4111-8111-111111111111", "ORD-20260615-001", "CONFIRMED", 50000, now),
		orderItems: []repository.OrderItem{
			orderItemRow(t, "55555555-5555-4555-8555-555555555555", orderID, "33333333-3333-4333-8333-333333333333", cartItemID, 2, 50000, now),
		},
	}
	service := NewOrderService(txRepo, &fakeOrderTxRunner{repo: txRepo}, time.Now)

	result, err := service.ConfirmOrderFromPayment(context.Background(), InternalConfirmOrderInput{OrderID: orderID})
	if err != nil {
		t.Fatalf("confirm order from payment: %v", err)
	}

	if result.Status != "CONFIRMED" {
		t.Fatalf("status = %q", result.Status)
	}
	if txRepo.updateStatusCalled {
		t.Fatalf("confirmed order should not be updated again")
	}
	if len(result.CartItemIDs) != 1 || result.CartItemIDs[0] != cartItemID {
		t.Fatalf("cart item ids = %v", result.CartItemIDs)
	}
}

type fakeOrderTxRunner struct {
	repo   orderRepository
	called bool
	err    error
}

func (f *fakeOrderTxRunner) Run(ctx context.Context, fn func(orderRepository) error) error {
	f.called = true
	if f.err != nil {
		return f.err
	}
	return fn(f.repo)
}

type fakeOrderRepo struct {
	checkoutRows             []repository.ListCheckoutCartItemsForUserRow
	lockedProducts           map[string]repository.Product
	createdOrder             repository.Order
	createdItems             []repository.OrderItem
	listRows                 []repository.ListOrdersRow
	order                    repository.Order
	updatedOrder             repository.Order
	orderItems               []repository.OrderItem
	expiredOrders            []repository.Order
	pendingCartItemCount     int64
	listArg                  repository.ListOrdersParams
	decrementArg             repository.DecrementProductStockParams
	incrementStockArg        repository.IncrementProductStockParams
	incrementTotalSoldArg    repository.IncrementProductTotalSoldParams
	updateStatusArg          repository.UpdateOrderStatusParams
	createOrderArg           repository.CreateOrderParams
	createItemArgs           []repository.CreateOrderItemParams
	getCheckoutErr           error
	lockProductErr           error
	decrementErr             error
	createOrderErr           error
	createItemErr            error
	listErr                  error
	getOrderErr              error
	listItemsErr             error
	createOrderCalled        bool
	decrementCalled          bool
	incrementStockCalled     bool
	incrementTotalSoldCalled bool
	updateStatusCalled       bool
	expiredCancelCalled      bool
}

func (f *fakeOrderRepo) ListCheckoutCartItemsForUser(ctx context.Context, arg repository.ListCheckoutCartItemsForUserParams) ([]repository.ListCheckoutCartItemsForUserRow, error) {
	if f.getCheckoutErr != nil {
		return nil, f.getCheckoutErr
	}
	return f.checkoutRows, nil
}

func (f *fakeOrderRepo) AcquireOrderNumberDateLock(ctx context.Context, dateKey string) error {
	return nil
}

func (f *fakeOrderRepo) CountOrdersByOrderNumberPrefix(ctx context.Context, prefix string) (int64, error) {
	return 0, nil
}

func (f *fakeOrderRepo) CountPendingOrderItemsByCartItemIDs(ctx context.Context, arg repository.CountPendingOrderItemsByCartItemIDsParams) (int64, error) {
	return f.pendingCartItemCount, nil
}

func (f *fakeOrderRepo) ListExpiredPendingOrdersByCartItemIDs(ctx context.Context, arg repository.ListExpiredPendingOrdersByCartItemIDsParams) ([]repository.Order, error) {
	return f.expiredOrders, nil
}

func (f *fakeOrderRepo) LockProductByIDForUpdate(ctx context.Context, id pgtype.UUID) (repository.Product, error) {
	if f.lockProductErr != nil {
		return repository.Product{}, f.lockProductErr
	}
	if product, ok := f.lockedProducts[id.String()]; ok {
		return product, nil
	}
	return repository.Product{}, pgx.ErrNoRows
}

func (f *fakeOrderRepo) LockOrderByIDForUpdate(ctx context.Context, id pgtype.UUID) (repository.Order, error) {
	if f.getOrderErr != nil {
		return repository.Order{}, f.getOrderErr
	}
	return f.order, nil
}

func (f *fakeOrderRepo) DecrementProductStock(ctx context.Context, arg repository.DecrementProductStockParams) (repository.Product, error) {
	f.decrementCalled = true
	f.decrementArg = arg
	if f.decrementErr != nil {
		return repository.Product{}, f.decrementErr
	}
	return repository.Product{ID: arg.ID, Stock: 8}, nil
}

func (f *fakeOrderRepo) IncrementProductStock(ctx context.Context, arg repository.IncrementProductStockParams) (repository.Product, error) {
	f.incrementStockCalled = true
	f.incrementStockArg = arg
	return repository.Product{ID: arg.ID, Stock: 10}, nil
}

func (f *fakeOrderRepo) IncrementProductTotalSold(ctx context.Context, arg repository.IncrementProductTotalSoldParams) (repository.Product, error) {
	f.incrementTotalSoldCalled = true
	f.incrementTotalSoldArg = arg
	return repository.Product{ID: arg.ID, TotalSold: arg.Quantity}, nil
}

func (f *fakeOrderRepo) CreateOrder(ctx context.Context, arg repository.CreateOrderParams) (repository.Order, error) {
	f.createOrderCalled = true
	f.createOrderArg = arg
	if f.createOrderErr != nil {
		return repository.Order{}, f.createOrderErr
	}
	return f.createdOrder, nil
}

func (f *fakeOrderRepo) CreateOrderItem(ctx context.Context, arg repository.CreateOrderItemParams) (repository.OrderItem, error) {
	f.createItemArgs = append(f.createItemArgs, arg)
	if f.createItemErr != nil {
		return repository.OrderItem{}, f.createItemErr
	}
	if len(f.createdItems) >= len(f.createItemArgs) {
		return f.createdItems[len(f.createItemArgs)-1], nil
	}
	return repository.OrderItem{}, nil
}

func (f *fakeOrderRepo) ListOrders(ctx context.Context, arg repository.ListOrdersParams) ([]repository.ListOrdersRow, error) {
	f.listArg = arg
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.listRows, nil
}

func (f *fakeOrderRepo) GetOrderByID(ctx context.Context, id pgtype.UUID) (repository.Order, error) {
	if f.getOrderErr != nil {
		return repository.Order{}, f.getOrderErr
	}
	return f.order, nil
}

func (f *fakeOrderRepo) UpdateOrderStatus(ctx context.Context, arg repository.UpdateOrderStatusParams) (repository.Order, error) {
	f.updateStatusCalled = true
	f.updateStatusArg = arg
	if arg.Status == repository.OrderStatusCANCELLED {
		f.expiredCancelCalled = true
	}
	return f.updatedOrder, nil
}

func (f *fakeOrderRepo) ListOrderItemsByOrderID(ctx context.Context, orderID pgtype.UUID) ([]repository.OrderItem, error) {
	if f.listItemsErr != nil {
		return nil, f.listItemsErr
	}
	return f.orderItems, nil
}

func orderRow(t *testing.T, orderID, userID, orderNumber, status string, totalAmount int32, updatedAt time.Time) repository.Order {
	t.Helper()

	return repository.Order{
		ID:          mustUUIDForOrder(t, orderID),
		OrderNumber: orderNumber,
		UserID:      mustUUIDForOrder(t, userID),
		Status:      repository.OrderStatus(status),
		TotalAmount: totalAmount,
		CreatedAt:   pgtype.Timestamptz{Time: updatedAt.Add(-time.Minute), Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: updatedAt, Valid: true},
	}
}

func orderItemRow(t *testing.T, orderItemID, orderID, productID, cartItemID string, quantity, subtotal int32, createdAt time.Time) repository.OrderItem {
	t.Helper()

	return repository.OrderItem{
		ID:              mustUUIDForOrder(t, orderItemID),
		OrderID:         mustUUIDForOrder(t, orderID),
		ProductID:       mustUUIDForOrder(t, productID),
		CartItemID:      mustUUIDForOrder(t, cartItemID),
		ProductName:     "Americano",
		PriceAtCheckout: 25000,
		Quantity:        quantity,
		Subtotal:        subtotal,
		CreatedAt:       pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
}

func checkoutCartItemRow(t *testing.T, cartItemID, productID, productName, category, status string, price, quantity, stock int32, deleted bool, attributes string) repository.ListCheckoutCartItemsForUserRow {
	t.Helper()

	row := repository.ListCheckoutCartItemsForUserRow{
		CartItemID:  mustUUIDForOrder(t, cartItemID),
		ProductID:   mustUUIDForOrder(t, productID),
		ProductName: productName,
		Price:       price,
		Category:    repository.ProductCategory(category),
		Status:      repository.ProductStatus(status),
		Attributes:  []byte(attributes),
		Stock:       stock,
		Quantity:    quantity,
	}
	if deleted {
		row.DeletedAt = pgtype.Timestamptz{Time: time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC), Valid: true}
	}
	return row
}

func orderProductRow(t *testing.T, productID, productName, category, status string, price, stock int32, deleted bool, attributes string) repository.Product {
	t.Helper()
	product := repository.Product{
		ID:         mustUUIDForOrder(t, productID),
		Name:       productName,
		Price:      price,
		Category:   repository.ProductCategory(category),
		Status:     repository.ProductStatus(status),
		Attributes: []byte(attributes),
		Stock:      stock,
	}
	if deleted {
		product.DeletedAt = pgtype.Timestamptz{Time: time.Date(2026, 6, 14, 4, 0, 0, 0, time.UTC), Valid: true}
	}
	return product
}

func orderListRow(t *testing.T, orderID, userID, orderNumber, status string, totalAmount int32, createdAt time.Time) repository.ListOrdersRow {
	t.Helper()
	return repository.ListOrdersRow{
		ID:          mustUUIDForOrder(t, orderID),
		OrderNumber: orderNumber,
		UserID:      mustUUIDForOrder(t, userID),
		Status:      repository.OrderStatus(status),
		TotalAmount: totalAmount,
		CreatedAt:   pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
}

func mustUUIDForOrder(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}

func stringPtrForOrder(value string) *string {
	return &value
}
