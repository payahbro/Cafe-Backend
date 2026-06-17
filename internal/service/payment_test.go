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

func TestPaymentServiceInitiateReusesActivePayment(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	orderID := "44444444-4444-4444-8444-444444444444"
	userID := "11111111-1111-4111-8111-111111111111"
	paymentID := "77777777-7777-4777-8777-777777777777"
	repo := &fakePaymentRepo{
		order:         paymentOrderRow(t, orderID, userID, "ORD-20260616-001", "PENDING", 68000, now.Add(10*time.Minute)),
		activePayment: paymentRow(t, paymentID, orderID, "PENDING_PAYMENT", 68000, "PAY-"+paymentID+"-1781611200", "https://app.sandbox.midtrans.com/snap/v2/vtweb/reuse", now),
	}
	client := &fakeSnapClient{}
	service := NewPaymentService(repo, nil, client, nil, nil, PaymentServiceOptions{Now: func() time.Time { return now }})

	result, err := service.InitiatePayment(context.Background(), InitiatePaymentInput{
		OrderID:     orderID,
		UserID:      userID,
		UserRole:    string(repository.UserRoleCUSTOMER),
		IsVerified:  true,
		FullName:    "Budi",
		Email:       "budi@example.test",
		PhoneNumber: "+628123456789",
	})
	if err != nil {
		t.Fatalf("initiate payment: %v", err)
	}

	if client.createCalled {
		t.Fatalf("midtrans should not be called when active payment exists")
	}
	if repo.createCalled {
		t.Fatalf("payment should not be created when active payment exists")
	}
	if result.PaymentID != paymentID {
		t.Fatalf("payment id = %q", result.PaymentID)
	}
	if result.SnapRedirectURL != "https://app.sandbox.midtrans.com/snap/v2/vtweb/reuse" {
		t.Fatalf("redirect url = %q", result.SnapRedirectURL)
	}
}

func TestPaymentServiceInitiateCreatesSnapTransaction(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	orderID := "44444444-4444-4444-8444-444444444444"
	userID := "11111111-1111-4111-8111-111111111111"
	repo := &fakePaymentRepo{
		order: paymentOrderRow(t, orderID, userID, "ORD-20260616-001", "PENDING", 68000, now.Add(10*time.Minute)),
		orderItems: []repository.OrderItem{
			paymentOrderItemRow(t, "55555555-5555-4555-8555-555555555555", orderID, "Americano", 34000, 2, 68000, now),
		},
		activePaymentErr: pgx.ErrNoRows,
		createdPayment:   paymentRow(t, "77777777-7777-4777-8777-777777777777", orderID, "PENDING_PAYMENT", 68000, "P-77777777-7777-4777-8777-777777777777-1781611200", "https://app.sandbox.midtrans.com/snap/v2/vtweb/new", now),
	}
	client := &fakeSnapClient{response: SnapTransaction{RedirectURL: "https://app.sandbox.midtrans.com/snap/v2/vtweb/new"}}
	service := NewPaymentService(repo, &fakePaymentTxRunner{repo: repo}, client, nil, nil, PaymentServiceOptions{
		Now:        func() time.Time { return now },
		NewUUID:    func() (string, error) { return "77777777-7777-4777-8777-777777777777", nil },
		WebhookURL: "https://example.ngrok-free.app/api/v1/payments/webhook",
	})

	result, err := service.InitiatePayment(context.Background(), InitiatePaymentInput{
		OrderID:     orderID,
		UserID:      userID,
		UserRole:    string(repository.UserRoleCUSTOMER),
		IsVerified:  true,
		FullName:    "Budi Santoso",
		Email:       "budi@example.test",
		PhoneNumber: "+628123456789",
	})
	if err != nil {
		t.Fatalf("initiate payment: %v", err)
	}

	if !client.createCalled {
		t.Fatalf("expected midtrans create transaction")
	}
	if client.request.OrderID != "P-77777777-7777-4777-8777-777777777777-1781611200" {
		t.Fatalf("midtrans order id = %q", client.request.OrderID)
	}
	if len(client.request.OrderID) > 50 {
		t.Fatalf("midtrans order id too long: len=%d value=%q", len(client.request.OrderID), client.request.OrderID)
	}
	if client.request.GrossAmount != 68000 {
		t.Fatalf("gross amount = %d", client.request.GrossAmount)
	}
	if client.request.NotificationURL != "https://example.ngrok-free.app/api/v1/payments/webhook" {
		t.Fatalf("notification url = %q", client.request.NotificationURL)
	}
	if len(client.request.Items) != 1 || client.request.Items[0].Name != "Americano" || client.request.Items[0].Quantity != 2 {
		t.Fatalf("items = %+v", client.request.Items)
	}
	if !repo.createCalled {
		t.Fatalf("expected payment creation")
	}
	if repo.createArg.ID.String() != "77777777-7777-4777-8777-777777777777" {
		t.Fatalf("created id = %s", repo.createArg.ID.String())
	}
	if result.SnapRedirectURL != "https://app.sandbox.midtrans.com/snap/v2/vtweb/new" {
		t.Fatalf("redirect url = %q", result.SnapRedirectURL)
	}
}

func TestPaymentServiceInitiateCancelsExpiredOrderAndRestoresStock(t *testing.T) {
	now := time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC)
	orderID := "44444444-4444-4444-8444-444444444444"
	userID := "11111111-1111-4111-8111-111111111111"
	productID := "33333333-3333-4333-8333-333333333333"
	cartItemID := "22222222-2222-4222-8222-222222222222"
	repo := &fakePaymentRepo{
		order: paymentOrderRow(t, orderID, userID, "ORD-20260617-001", "PENDING", 68000, now.Add(-time.Minute)),
		orderItems: []repository.OrderItem{
			paymentOrderItemWithProductRow(t, "55555555-5555-4555-8555-555555555555", orderID, productID, cartItemID, "Americano", 34000, 2, 68000, now.Add(-16*time.Minute)),
		},
		lockedProducts: map[string]repository.Product{
			productID: {ID: mustUUIDForPayment(t, productID), Stock: 5},
		},
		updatedOrder: paymentOrderRow(t, orderID, userID, "ORD-20260617-001", "CANCELLED", 68000, now.Add(-time.Minute)),
	}
	service := NewPaymentService(repo, &fakePaymentTxRunner{repo: repo}, &fakeSnapClient{}, nil, nil, PaymentServiceOptions{
		Now: func() time.Time { return now },
	})

	_, err := service.InitiatePayment(context.Background(), InitiatePaymentInput{
		OrderID:     orderID,
		UserID:      userID,
		UserRole:    string(repository.UserRoleCUSTOMER),
		IsVerified:  true,
		FullName:    "Budi",
		Email:       "budi@example.test",
		PhoneNumber: "+628123456789",
	})

	if !errors.Is(err, ErrPaymentOrderExpired) {
		t.Fatalf("err = %v", err)
	}
	if !repo.incrementStockCalled {
		t.Fatalf("expected expired order stock to be restored")
	}
	if !repo.updateOrderStatusCalled || repo.updateOrderStatusArg.Status != repository.OrderStatusCANCELLED {
		t.Fatalf("expected order to be cancelled, arg=%+v", repo.updateOrderStatusArg)
	}
}

func TestPaymentServiceWebhookSuccessUpdatesPaymentAndConfirmsOrder(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	orderID := "44444444-4444-4444-8444-444444444444"
	paymentID := "77777777-7777-4777-8777-777777777777"
	repo := &fakePaymentRepo{
		payment:        paymentRow(t, paymentID, orderID, "PENDING_PAYMENT", 68000, "PAY-"+paymentID+"-1781611200", "https://app.sandbox.midtrans.com/snap/v2/vtweb/new", now),
		updatedPayment: paymentRow(t, paymentID, orderID, "SUCCESS", 68000, "PAY-"+paymentID+"-1781611200", "", now.Add(time.Minute)),
	}
	orderConfirmer := &fakePaymentOrderConfirmer{
		result: &InternalConfirmOrderResult{
			OrderID:     orderID,
			Status:      "CONFIRMED",
			CartItemIDs: []string{"22222222-2222-4222-8222-222222222222"},
		},
	}
	cartClearer := &fakePaymentCartClearer{}
	service := NewPaymentService(repo, nil, nil, orderConfirmer, cartClearer, PaymentServiceOptions{
		ServerKey: "server-key",
		Now:       func() time.Time { return now },
	})
	payload := WebhookInput{
		OrderID:           "PAY-" + paymentID + "-1781611200",
		StatusCode:        "200",
		GrossAmount:       "68000.00",
		SignatureKey:      ComputeMidtransSignature("PAY-"+paymentID+"-1781611200", "200", "68000.00", "server-key"),
		TransactionStatus: "settlement",
		FraudStatus:       "accept",
		TransactionID:     "midtrans-tx-1",
		PaymentType:       "qris",
	}

	result, err := service.HandleWebhook(context.Background(), payload)
	if err != nil {
		t.Fatalf("handle webhook: %v", err)
	}

	if !result.Processed {
		t.Fatalf("expected webhook to be processed")
	}
	if repo.updateArg.Status != repository.PaymentStatusSUCCESS {
		t.Fatalf("updated status = %q", repo.updateArg.Status)
	}
	if repo.updateArg.PaymentMethod.String != "qris" {
		t.Fatalf("payment method = %q", repo.updateArg.PaymentMethod.String)
	}
	if !orderConfirmer.called {
		t.Fatalf("expected order confirmation")
	}
	if !cartClearer.called {
		t.Fatalf("expected cart clear")
	}
}

func TestPaymentServiceWebhookIgnoresInvalidSignature(t *testing.T) {
	repo := &fakePaymentRepo{}
	service := NewPaymentService(repo, nil, nil, nil, nil, PaymentServiceOptions{ServerKey: "server-key"})

	result, err := service.HandleWebhook(context.Background(), WebhookInput{
		OrderID:           "PAY-77777777-7777-4777-8777-777777777777-1781611200",
		StatusCode:        "200",
		GrossAmount:       "68000.00",
		SignatureKey:      "invalid",
		TransactionStatus: "settlement",
	})
	if err != nil {
		t.Fatalf("handle webhook: %v", err)
	}

	if result.Processed {
		t.Fatalf("invalid signature should be ignored")
	}
	if repo.getPaymentCalled || repo.updateCalled {
		t.Fatalf("repository should not be called for invalid signature")
	}
}

func TestPaymentServiceGetPaymentsByOrderRestrictsCustomerAndReturnsNewest(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	orderID := "44444444-4444-4444-8444-444444444444"
	userID := "11111111-1111-4111-8111-111111111111"
	repo := &fakePaymentRepo{
		order: paymentOrderRow(t, orderID, userID, "ORD-20260616-001", "PENDING", 68000, now.Add(10*time.Minute)),
		payments: []repository.Payment{
			paymentRow(t, "77777777-7777-4777-8777-777777777777", orderID, "SUCCESS", 68000, "PAY-77777777-7777-4777-8777-777777777777-1781611200", "", now),
			paymentRow(t, "88888888-8888-4888-8888-888888888888", orderID, "EXPIRED", 68000, "PAY-88888888-8888-4888-8888-888888888888-1781610000", "", now.Add(-time.Minute)),
		},
	}
	service := NewPaymentService(repo, nil, nil, nil, nil, PaymentServiceOptions{})

	result, err := service.GetPaymentsByOrder(context.Background(), GetPaymentsByOrderInput{
		OrderID:     orderID,
		ActorUserID: userID,
		ActorRole:   string(repository.UserRoleCUSTOMER),
	})
	if err != nil {
		t.Fatalf("get payments by order: %v", err)
	}

	if len(result.Payments) != 1 {
		t.Fatalf("payments len = %d", len(result.Payments))
	}
	if result.Payments[0].PaymentID != "77777777-7777-4777-8777-777777777777" {
		t.Fatalf("payment id = %q", result.Payments[0].PaymentID)
	}
}

func TestPaymentServiceListPaymentsForCustomerForcesOwnUserFilter(t *testing.T) {
	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	userID := "11111111-1111-4111-8111-111111111111"
	orderID := "44444444-4444-4444-8444-444444444444"
	repo := &fakePaymentRepo{
		paymentList: []repository.ListPaymentsRow{
			paymentListRow(t, "77777777-7777-4777-8777-777777777777", orderID, "ORD-20260616-001", userID, "SUCCESS", 68000, "qris", now),
		},
	}
	service := NewPaymentService(repo, nil, nil, nil, nil, PaymentServiceOptions{})

	result, err := service.ListPayments(context.Background(), ListPaymentsInput{
		ActorUserID: userID,
		ActorRole:   string(repository.UserRoleCUSTOMER),
		UserID:      "99999999-9999-4999-8999-999999999999",
		Status:      "SUCCESS",
		Limit:       10,
	})
	if err != nil {
		t.Fatalf("list payments: %v", err)
	}

	if !repo.listPaymentsCalled {
		t.Fatalf("expected repository list payments call")
	}
	if repo.listPaymentsArg.UserID.String() != userID {
		t.Fatalf("user filter = %s", repo.listPaymentsArg.UserID.String())
	}
	if repo.listPaymentsArg.Status != "SUCCESS" {
		t.Fatalf("status filter = %q", repo.listPaymentsArg.Status)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items len = %d", len(result.Items))
	}
	if result.Items[0].OrderNumber != "ORD-20260616-001" || result.Items[0].PaymentMethod == nil || *result.Items[0].PaymentMethod != "qris" {
		t.Fatalf("payment item = %+v", result.Items[0])
	}
}

type fakePaymentTxRunner struct {
	repo paymentRepository
}

func (f *fakePaymentTxRunner) Run(ctx context.Context, fn func(paymentRepository) error) error {
	return fn(f.repo)
}

type fakePaymentRepo struct {
	order                   repository.Order
	orderItems              []repository.OrderItem
	lockedProducts          map[string]repository.Product
	activePayment           repository.Payment
	payment                 repository.Payment
	updatedPayment          repository.Payment
	createdPayment          repository.Payment
	updatedOrder            repository.Order
	payments                []repository.Payment
	paymentList             []repository.ListPaymentsRow
	createArg               repository.CreatePaymentParams
	updateArg               repository.UpdatePaymentAfterWebhookParams
	updateOrderStatusArg    repository.UpdateOrderStatusParams
	listPaymentsArg         repository.ListPaymentsParams
	activePaymentErr        error
	orderErr                error
	paymentErr              error
	paymentsErr             error
	listPaymentsErr         error
	createCalled            bool
	updateCalled            bool
	getPaymentCalled        bool
	listPaymentsCalled      bool
	incrementStockCalled    bool
	updateOrderStatusCalled bool
}

func (f *fakePaymentRepo) GetOrderByID(ctx context.Context, id pgtype.UUID) (repository.Order, error) {
	if f.orderErr != nil {
		return repository.Order{}, f.orderErr
	}
	return f.order, nil
}

func (f *fakePaymentRepo) ListOrderItemsByOrderID(ctx context.Context, orderID pgtype.UUID) ([]repository.OrderItem, error) {
	return f.orderItems, nil
}

func (f *fakePaymentRepo) LockOrderByIDForUpdate(ctx context.Context, id pgtype.UUID) (repository.Order, error) {
	if f.orderErr != nil {
		return repository.Order{}, f.orderErr
	}
	return f.order, nil
}

func (f *fakePaymentRepo) LockProductByIDForUpdate(ctx context.Context, id pgtype.UUID) (repository.Product, error) {
	if product, ok := f.lockedProducts[id.String()]; ok {
		return product, nil
	}
	return repository.Product{}, nil
}

func (f *fakePaymentRepo) IncrementProductStock(ctx context.Context, arg repository.IncrementProductStockParams) (repository.Product, error) {
	f.incrementStockCalled = true
	return repository.Product{ID: arg.ID, Stock: arg.Quantity}, nil
}

func (f *fakePaymentRepo) UpdateOrderStatus(ctx context.Context, arg repository.UpdateOrderStatusParams) (repository.Order, error) {
	f.updateOrderStatusCalled = true
	f.updateOrderStatusArg = arg
	return f.updatedOrder, nil
}

func (f *fakePaymentRepo) GetActivePaymentByOrderID(ctx context.Context, orderID pgtype.UUID) (repository.Payment, error) {
	if f.activePaymentErr != nil {
		return repository.Payment{}, f.activePaymentErr
	}
	if !f.activePayment.ID.Valid {
		return repository.Payment{}, pgx.ErrNoRows
	}
	return f.activePayment, nil
}

func (f *fakePaymentRepo) CreatePayment(ctx context.Context, arg repository.CreatePaymentParams) (repository.Payment, error) {
	f.createCalled = true
	f.createArg = arg
	return f.createdPayment, nil
}

func (f *fakePaymentRepo) GetPaymentByID(ctx context.Context, id pgtype.UUID) (repository.Payment, error) {
	f.getPaymentCalled = true
	if f.paymentErr != nil {
		return repository.Payment{}, f.paymentErr
	}
	return f.payment, nil
}

func (f *fakePaymentRepo) UpdatePaymentAfterWebhook(ctx context.Context, arg repository.UpdatePaymentAfterWebhookParams) (repository.Payment, error) {
	f.updateCalled = true
	f.updateArg = arg
	return f.updatedPayment, nil
}

func (f *fakePaymentRepo) ListPaymentsByOrderID(ctx context.Context, orderID pgtype.UUID) ([]repository.Payment, error) {
	if f.paymentsErr != nil {
		return nil, f.paymentsErr
	}
	return f.payments, nil
}

func (f *fakePaymentRepo) ListPayments(ctx context.Context, arg repository.ListPaymentsParams) ([]repository.ListPaymentsRow, error) {
	f.listPaymentsCalled = true
	f.listPaymentsArg = arg
	if f.listPaymentsErr != nil {
		return nil, f.listPaymentsErr
	}
	return f.paymentList, nil
}

type fakeSnapClient struct {
	request      SnapTransactionRequest
	response     SnapTransaction
	err          error
	createCalled bool
}

func (f *fakeSnapClient) CreateTransaction(ctx context.Context, request SnapTransactionRequest) (SnapTransaction, error) {
	f.createCalled = true
	f.request = request
	if f.err != nil {
		return SnapTransaction{}, f.err
	}
	return f.response, nil
}

type fakePaymentOrderConfirmer struct {
	input  InternalConfirmOrderInput
	result *InternalConfirmOrderResult
	err    error
	called bool
}

func (f *fakePaymentOrderConfirmer) ConfirmOrderFromPayment(ctx context.Context, input InternalConfirmOrderInput) (*InternalConfirmOrderResult, error) {
	f.called = true
	f.input = input
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

type fakePaymentCartClearer struct {
	itemIDs []string
	err     error
	called  bool
}

func (f *fakePaymentCartClearer) ClearItemsByIDs(ctx context.Context, itemIDs []string) error {
	f.called = true
	f.itemIDs = itemIDs
	return f.err
}

func paymentOrderRow(t *testing.T, orderID, userID, orderNumber, status string, totalAmount int32, expiresAt time.Time) repository.Order {
	t.Helper()
	return repository.Order{
		ID:          mustUUIDForPayment(t, orderID),
		OrderNumber: orderNumber,
		UserID:      mustUUIDForPayment(t, userID),
		Status:      repository.OrderStatus(status),
		TotalAmount: totalAmount,
		ExpiresAt:   pgtype.Timestamptz{Time: expiresAt, Valid: true},
		CreatedAt:   pgtype.Timestamptz{Time: expiresAt.Add(-15 * time.Minute), Valid: true},
		UpdatedAt:   pgtype.Timestamptz{Time: expiresAt.Add(-15 * time.Minute), Valid: true},
	}
}

func paymentOrderItemRow(t *testing.T, itemID, orderID, productName string, price, quantity, subtotal int32, createdAt time.Time) repository.OrderItem {
	t.Helper()
	return repository.OrderItem{
		ID:              mustUUIDForPayment(t, itemID),
		OrderID:         mustUUIDForPayment(t, orderID),
		ProductName:     productName,
		PriceAtCheckout: price,
		Quantity:        quantity,
		Subtotal:        subtotal,
		CreatedAt:       pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
}

func paymentOrderItemWithProductRow(t *testing.T, itemID, orderID, productID, cartItemID, productName string, price, quantity, subtotal int32, createdAt time.Time) repository.OrderItem {
	t.Helper()
	row := paymentOrderItemRow(t, itemID, orderID, productName, price, quantity, subtotal, createdAt)
	row.ProductID = mustUUIDForPayment(t, productID)
	row.CartItemID = mustUUIDForPayment(t, cartItemID)
	return row
}

func paymentRow(t *testing.T, paymentID, orderID, status string, amount int32, midtransOrderID, snapURL string, createdAt time.Time) repository.Payment {
	t.Helper()
	return repository.Payment{
		ID:              mustUUIDForPayment(t, paymentID),
		OrderID:         mustUUIDForPayment(t, orderID),
		Status:          repository.PaymentStatus(status),
		Amount:          amount,
		MidtransOrderID: midtransOrderID,
		SnapRedirectUrl: pgtype.Text{String: snapURL, Valid: snapURL != ""},
		CreatedAt:       pgtype.Timestamptz{Time: createdAt, Valid: true},
		UpdatedAt:       pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
}

func paymentListRow(t *testing.T, paymentID, orderID, orderNumber, userID, status string, amount int32, method string, createdAt time.Time) repository.ListPaymentsRow {
	t.Helper()
	return repository.ListPaymentsRow{
		ID:                    mustUUIDForPayment(t, paymentID),
		OrderID:               mustUUIDForPayment(t, orderID),
		OrderNumber:           orderNumber,
		UserID:                mustUUIDForPayment(t, userID),
		Status:                repository.PaymentStatus(status),
		Amount:                amount,
		PaymentMethod:         pgtype.Text{String: method, Valid: method != ""},
		MidtransTransactionID: pgtype.Text{},
		SnapRedirectUrl:       pgtype.Text{},
		RefundAmount:          pgtype.Int4{},
		RefundReason:          pgtype.Text{},
		RefundedAt:            pgtype.Timestamptz{},
		CreatedAt:             pgtype.Timestamptz{Time: createdAt, Valid: true},
		UpdatedAt:             pgtype.Timestamptz{Time: createdAt, Valid: true},
	}
}

func mustUUIDForPayment(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}
