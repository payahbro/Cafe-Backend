package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/repository"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestOrderHandlerCheckoutReturnsCreatedOrder(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 6, 14, 3, 0, 0, 0, time.UTC)
	fakeService := &fakeOrderService{
		order: &service.Order{
			ID:          "44444444-4444-4444-8444-444444444444",
			OrderNumber: "ORD-20260614-001",
			UserID:      "11111111-1111-4111-8111-111111111111",
			Status:      "PENDING",
			Notes:       stringPtrForHandlerOrder("Tolong bungkus rapi"),
			TotalAmount: 50000,
			ExpiresAt:   timePtrForHandlerOrder(now.Add(15 * time.Minute)),
			CreatedAt:   now,
			UpdatedAt:   now,
			Items: []service.OrderItem{
				{
					ID:                 "55555555-5555-4555-8555-555555555555",
					ProductID:          "33333333-3333-4333-8333-333333333333",
					ProductName:        "Americano",
					PriceAtCheckout:    25000,
					Quantity:           2,
					Subtotal:           50000,
					SelectedAttributes: []byte(`{"temperature":"iced","sizes":"medium","sugar_levels":"normal","ice_levels":"normal"}`),
				},
			},
		},
	}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService)
	router.POST("/orders/checkout", authenticatedOrderCustomerMiddleware(), orderHandler.Checkout)

	req := httptest.NewRequest(http.MethodPost, "/orders/checkout", strings.NewReader(`{"notes":"Tolong bungkus rapi","items":[{"cart_item_id":"22222222-2222-4222-8222-222222222222","attributes":{"temperature":"iced","sizes":"medium"}}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.checkoutCalled {
		t.Fatalf("expected Checkout to be called")
	}
	if fakeService.checkoutInput.UserID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("user id = %q", fakeService.checkoutInput.UserID)
	}
	if !fakeService.checkoutInput.IsVerified || fakeService.checkoutInput.PhoneNumber == "" {
		t.Fatalf("preconditions not passed: %+v", fakeService.checkoutInput)
	}
	body := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"order_number":"ORD-20260614-001"`,
		`"total_amount":50000`,
		`"temperature":"iced"`,
		`"message":"Order berhasil dibuat"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %s: %s", want, body)
		}
	}
}

func TestOrderHandlerCheckoutRejectsEmptyItems(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeOrderService{}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService)
	router.POST("/orders/checkout", authenticatedOrderCustomerMiddleware(), orderHandler.Checkout)

	req := httptest.NewRequest(http.MethodPost, "/orders/checkout", strings.NewReader(`{"items":[]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.checkoutCalled {
		t.Fatalf("service should not be called for empty items")
	}
	if !strings.Contains(resp.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestOrderHandlerCheckoutMapsPhoneRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeOrderService{err: service.ErrOrderPhoneNumberRequired}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService)
	router.POST("/orders/checkout", authenticatedOrderCustomerMiddleware(), orderHandler.Checkout)

	req := httptest.NewRequest(http.MethodPost, "/orders/checkout", strings.NewReader(`{"items":[{"cart_item_id":"22222222-2222-4222-8222-222222222222","attributes":{}}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PHONE_NUMBER_REQUIRED"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestOrderHandlerCheckoutMapsCartItemAlreadyPending(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeOrderService{err: service.ErrOrderCartItemAlreadyPending}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService)
	router.POST("/orders/checkout", authenticatedOrderCustomerMiddleware(), orderHandler.Checkout)

	req := httptest.NewRequest(http.MethodPost, "/orders/checkout", strings.NewReader(`{"items":[{"cart_item_id":"22222222-2222-4222-8222-222222222222","attributes":{}}]}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"CART_ITEM_ALREADY_PENDING"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestOrderHandlerListOrdersReturnsPaginationEnvelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeOrderService{
		list: &service.OrderList{
			Items: []service.OrderSummary{
				{
					ID:          "44444444-4444-4444-8444-444444444444",
					OrderNumber: "ORD-20260614-001",
					Status:      "PENDING",
					TotalAmount: 50000,
					CreatedAt:   time.Date(2026, 6, 14, 3, 0, 0, 0, time.UTC),
				},
			},
			Limit:   10,
			HasNext: false,
			HasPrev: false,
		},
	}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService)
	router.GET("/orders", authenticatedOrderCustomerMiddleware(), orderHandler.ListOrders)

	req := httptest.NewRequest(http.MethodGet, "/orders?limit=10&status=PENDING", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.listCalled {
		t.Fatalf("expected ListOrders to be called")
	}
	body := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"order_number":"ORD-20260614-001"`,
		`"pagination":`,
		`"limit":10`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %s: %s", want, body)
		}
	}
}

func TestOrderHandlerGetOrderReturnsDetail(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeOrderService{
		order: &service.Order{
			ID:          "44444444-4444-4444-8444-444444444444",
			OrderNumber: "ORD-20260614-001",
			UserID:      "11111111-1111-4111-8111-111111111111",
			Status:      "PENDING",
			TotalAmount: 50000,
			CreatedAt:   time.Date(2026, 6, 14, 3, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 6, 14, 3, 0, 0, 0, time.UTC),
			Items:       []service.OrderItem{},
		},
	}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService)
	router.GET("/orders/:order_id", authenticatedOrderCustomerMiddleware(), orderHandler.GetOrder)

	req := httptest.NewRequest(http.MethodGet, "/orders/44444444-4444-4444-8444-444444444444", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.getCalled {
		t.Fatalf("expected GetOrder to be called")
	}
	if fakeService.getInput.OrderID != "44444444-4444-4444-8444-444444444444" {
		t.Fatalf("order id = %q", fakeService.getInput.OrderID)
	}
	if !strings.Contains(resp.Body.String(), `"message":"Order berhasil diambil"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestOrderHandlerCancelOrderReturnsCancelledStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeOrderService{
		statusUpdate: &service.OrderStatusUpdate{
			OrderID:   "44444444-4444-4444-8444-444444444444",
			Status:    "CANCELLED",
			UpdatedAt: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
		},
	}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService)
	router.PATCH("/orders/:order_id/cancel", authenticatedOrderCustomerMiddleware(), orderHandler.CancelOrder)

	req := httptest.NewRequest(http.MethodPatch, "/orders/44444444-4444-4444-8444-444444444444/cancel", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.cancelCalled {
		t.Fatalf("expected CancelOrder to be called")
	}
	if !strings.Contains(resp.Body.String(), `"status":"CANCELLED"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestOrderHandlerUpdateStatusPassesCompletedStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeOrderService{
		statusUpdate: &service.OrderStatusUpdate{
			OrderID:   "44444444-4444-4444-8444-444444444444",
			Status:    "COMPLETED",
			UpdatedAt: time.Date(2026, 6, 15, 9, 0, 0, 0, time.UTC),
		},
	}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService)
	router.PATCH("/orders/:order_id/status", authenticatedOrderPegawaiMiddleware(), orderHandler.UpdateStatus)

	req := httptest.NewRequest(http.MethodPatch, "/orders/44444444-4444-4444-8444-444444444444/status", strings.NewReader(`{"status":"COMPLETED"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.updateStatusCalled {
		t.Fatalf("expected UpdateOrderStatus to be called")
	}
	if fakeService.updateStatusInput.Status != "COMPLETED" {
		t.Fatalf("status input = %q", fakeService.updateStatusInput.Status)
	}
	if !strings.Contains(resp.Body.String(), `"message":"Status order berhasil diperbarui"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestOrderHandlerInternalUpdateStatusConfirmsAndClearsCartItems(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeOrderService{
		internalResult: &service.InternalConfirmOrderResult{
			OrderID:     "44444444-4444-4444-8444-444444444444",
			Status:      "CONFIRMED",
			CartItemIDs: []string{"22222222-2222-4222-8222-222222222222"},
		},
	}
	fakeCart := &fakeOrderCartClearer{}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService, WithOrderCartClearer(fakeCart), WithOrderInternalAPIKey("secret"))
	router.PATCH("/internal/orders/:order_id/status", orderHandler.InternalUpdateStatus)

	req := httptest.NewRequest(http.MethodPatch, "/internal/orders/44444444-4444-4444-8444-444444444444/status", nil)
	req.Header.Set("X-Internal-Api-Key", "secret")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.internalConfirmCalled {
		t.Fatalf("expected ConfirmOrderFromPayment to be called")
	}
	if !fakeCart.clearCalled {
		t.Fatalf("expected cart clear to be called")
	}
	if len(fakeCart.itemIDs) != 1 || fakeCart.itemIDs[0] != "22222222-2222-4222-8222-222222222222" {
		t.Fatalf("cart item ids = %v", fakeCart.itemIDs)
	}
	if !strings.Contains(resp.Body.String(), `"message":"Order status updated to CONFIRMED"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestOrderHandlerInternalUpdateStatusRejectsMissingAPIKey(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeOrderService{}
	router := gin.New()
	orderHandler := NewOrderHandler(fakeService, WithOrderInternalAPIKey("secret"))
	router.PATCH("/internal/orders/:order_id/status", orderHandler.InternalUpdateStatus)

	req := httptest.NewRequest(http.MethodPatch, "/internal/orders/44444444-4444-4444-8444-444444444444/status", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.internalConfirmCalled {
		t.Fatalf("service should not be called without api key")
	}
}

type fakeOrderService struct {
	order         *service.Order
	list          *service.OrderList
	statusUpdate  *service.OrderStatusUpdate
	internalResult *service.InternalConfirmOrderResult
	err           error
	checkoutInput service.CheckoutInput
	listInput     service.ListOrdersInput
	getInput      service.GetOrderInput
	cancelInput   service.CancelOrderInput
	updateStatusInput service.UpdateOrderStatusInput
	internalInput service.InternalConfirmOrderInput
	checkoutCalled bool
	listCalled     bool
	getCalled      bool
	cancelCalled   bool
	updateStatusCalled bool
	internalConfirmCalled bool
}

func (f *fakeOrderService) Checkout(_ context.Context, input service.CheckoutInput) (*service.Order, error) {
	f.checkoutCalled = true
	f.checkoutInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.order, nil
}

func (f *fakeOrderService) ListOrders(_ context.Context, input service.ListOrdersInput) (*service.OrderList, error) {
	f.listCalled = true
	f.listInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

func (f *fakeOrderService) GetOrder(_ context.Context, input service.GetOrderInput) (*service.Order, error) {
	f.getCalled = true
	f.getInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.order, nil
}

func (f *fakeOrderService) CancelOrder(_ context.Context, input service.CancelOrderInput) (*service.OrderStatusUpdate, error) {
	f.cancelCalled = true
	f.cancelInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.statusUpdate, nil
}

func (f *fakeOrderService) UpdateOrderStatus(_ context.Context, input service.UpdateOrderStatusInput) (*service.OrderStatusUpdate, error) {
	f.updateStatusCalled = true
	f.updateStatusInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.statusUpdate, nil
}

func (f *fakeOrderService) ConfirmOrderFromPayment(_ context.Context, input service.InternalConfirmOrderInput) (*service.InternalConfirmOrderResult, error) {
	f.internalConfirmCalled = true
	f.internalInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.internalResult, nil
}

func authenticatedOrderCustomerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var userID pgtype.UUID
		_ = userID.Scan("11111111-1111-4111-8111-111111111111")
		c.Set("authenticated_user", middleware.AuthenticatedUser{
			ID:          "11111111-1111-4111-8111-111111111111",
			UUID:        userID,
			Role:        repository.UserRoleCUSTOMER,
			IsVerified:  true,
			IsActive:    true,
			PhoneNumber: "+628123456789",
		})
		c.Next()
	}
}

func authenticatedOrderPegawaiMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var userID pgtype.UUID
		_ = userID.Scan("66666666-6666-4666-8666-666666666666")
		c.Set("authenticated_user", middleware.AuthenticatedUser{
			ID:       "66666666-6666-4666-8666-666666666666",
			UUID:     userID,
			Role:     repository.UserRolePEGAWAI,
			IsActive: true,
		})
		c.Next()
	}
}

type fakeOrderCartClearer struct {
	itemIDs []string
	clearCalled bool
	err error
}

func (f *fakeOrderCartClearer) ClearItemsByIDs(_ context.Context, itemIDs []string) error {
	f.clearCalled = true
	f.itemIDs = itemIDs
	return f.err
}

func stringPtrForHandlerOrder(value string) *string {
	return &value
}

func timePtrForHandlerOrder(value time.Time) *time.Time {
	return &value
}
