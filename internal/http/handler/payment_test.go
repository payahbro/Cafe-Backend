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

func TestPaymentHandlerInitiateReturnsSnapRedirectURL(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakePaymentService{
		initiation: &service.PaymentInitiation{
			PaymentID:       "77777777-7777-4777-8777-777777777777",
			OrderID:         "44444444-4444-4444-8444-444444444444",
			SnapRedirectURL: "https://app.sandbox.midtrans.com/snap/v2/vtweb/new",
			ExpiresAt:       timePtrForHandlerPayment(time.Date(2026, 6, 16, 12, 15, 0, 0, time.UTC)),
		},
	}
	router := gin.New()
	paymentHandler := NewPaymentHandler(fakeService)
	router.POST("/payments/initiate", authenticatedPaymentCustomerMiddleware(), paymentHandler.Initiate)

	req := httptest.NewRequest(http.MethodPost, "/payments/initiate", strings.NewReader(`{"order_id":"44444444-4444-4444-8444-444444444444"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.initiateCalled {
		t.Fatalf("expected initiate service call")
	}
	if fakeService.initiateInput.FullName != "Budi" || fakeService.initiateInput.Email != "budi@example.test" {
		t.Fatalf("user details = %+v", fakeService.initiateInput)
	}
	for _, want := range []string{
		`"payment_id":"77777777-7777-4777-8777-777777777777"`,
		`"snap_redirect_url":"https://app.sandbox.midtrans.com/snap/v2/vtweb/new"`,
		`"message":"Payment berhasil dibuat, silakan lanjutkan ke halaman pembayaran"`,
	} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("response body missing %s: %s", want, resp.Body.String())
		}
	}
}

func TestPaymentHandlerWebhookAlwaysReturnsOK(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakePaymentService{webhookResult: &service.WebhookResult{Processed: true, Status: "SUCCESS"}}
	router := gin.New()
	paymentHandler := NewPaymentHandler(fakeService)
	router.POST("/payments/webhook", paymentHandler.Webhook)

	req := httptest.NewRequest(http.MethodPost, "/payments/webhook", strings.NewReader(`{"order_id":"PAY-77777777-7777-4777-8777-777777777777-1781611200","status_code":"200","gross_amount":"68000.00","signature_key":"signature","transaction_status":"settlement","fraud_status":"accept","transaction_id":"midtrans-tx-1","payment_type":"qris"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.webhookCalled {
		t.Fatalf("expected webhook service call")
	}
	if fakeService.webhookInput.PaymentType != "qris" {
		t.Fatalf("payment type = %q", fakeService.webhookInput.PaymentType)
	}
	if !strings.Contains(resp.Body.String(), `"message":"OK"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestPaymentHandlerGetByOrderReturnsCustomerSinglePayment(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	fakeService := &fakePaymentService{
		payments: &service.PaymentsByOrder{
			OrderID: "44444444-4444-4444-8444-444444444444",
			Payments: []service.Payment{
				{
					PaymentID:   "77777777-7777-4777-8777-777777777777",
					OrderID:     "44444444-4444-4444-8444-444444444444",
					OrderNumber: "ORD-20260616-001",
					Status:      "SUCCESS",
					Amount:      68000,
					CreatedAt:   now,
					UpdatedAt:   now,
				},
			},
		},
	}
	router := gin.New()
	paymentHandler := NewPaymentHandler(fakeService)
	router.GET("/payments/order/:order_id", authenticatedPaymentCustomerMiddleware(), paymentHandler.GetByOrder)

	req := httptest.NewRequest(http.MethodGet, "/payments/order/44444444-4444-4444-8444-444444444444", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.getCalled {
		t.Fatalf("expected get by order service call")
	}
	if !strings.Contains(resp.Body.String(), `"payment_id":"77777777-7777-4777-8777-777777777777"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
	if strings.Contains(resp.Body.String(), `"data":[`) {
		t.Fatalf("customer response should be single object: %s", resp.Body.String())
	}
}

func TestPaymentHandlerListMeReturnsCustomerPayments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	fakeService := &fakePaymentService{
		list: &service.PaymentList{
			Items: []service.Payment{
				{
					PaymentID:   "77777777-7777-4777-8777-777777777777",
					OrderID:     "44444444-4444-4444-8444-444444444444",
					OrderNumber: "ORD-20260616-001",
					Status:      "SUCCESS",
					Amount:      68000,
					CreatedAt:   now,
					UpdatedAt:   now,
				},
			},
			Limit:   10,
			HasNext: false,
			HasPrev: false,
		},
	}
	router := gin.New()
	paymentHandler := NewPaymentHandler(fakeService)
	router.GET("/payments/me", authenticatedPaymentCustomerMiddleware(), paymentHandler.ListMe)

	req := httptest.NewRequest(http.MethodGet, "/payments/me?limit=10&status=SUCCESS", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.listCalled {
		t.Fatalf("expected list service call")
	}
	if fakeService.listInput.UserID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("user filter = %q", fakeService.listInput.UserID)
	}
	body := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"payment_id":"77777777-7777-4777-8777-777777777777"`,
		`"order_number":"ORD-20260616-001"`,
		`"pagination":`,
		`"limit":10`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %s: %s", want, body)
		}
	}
}

func TestPaymentHandlerListAllReturnsAdminPayments(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 6, 16, 12, 0, 0, 0, time.UTC)
	fakeService := &fakePaymentService{
		list: &service.PaymentList{
			Items: []service.Payment{
				{
					PaymentID:   "77777777-7777-4777-8777-777777777777",
					OrderID:     "44444444-4444-4444-8444-444444444444",
					OrderNumber: "ORD-20260616-001",
					Status:      "PENDING_PAYMENT",
					Amount:      68000,
					CreatedAt:   now,
					UpdatedAt:   now,
				},
			},
			Limit:   20,
			HasNext: true,
			HasPrev: false,
		},
	}
	router := gin.New()
	paymentHandler := NewPaymentHandler(fakeService)
	router.GET("/payments", authenticatedPaymentAdminMiddleware(), paymentHandler.ListAll)

	req := httptest.NewRequest(http.MethodGet, "/payments?limit=20&user_id=11111111-1111-4111-8111-111111111111&status=PENDING_PAYMENT", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.listCalled {
		t.Fatalf("expected list service call")
	}
	if fakeService.listInput.ActorRole != string(repository.UserRoleADMIN) {
		t.Fatalf("actor role = %q", fakeService.listInput.ActorRole)
	}
	if fakeService.listInput.UserID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("user filter = %q", fakeService.listInput.UserID)
	}
	if !strings.Contains(resp.Body.String(), `"has_next":true`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

type fakePaymentService struct {
	initiation     *service.PaymentInitiation
	webhookResult  *service.WebhookResult
	payments       *service.PaymentsByOrder
	list           *service.PaymentList
	err            error
	initiateInput  service.InitiatePaymentInput
	webhookInput   service.WebhookInput
	getInput       service.GetPaymentsByOrderInput
	listInput      service.ListPaymentsInput
	initiateCalled bool
	webhookCalled  bool
	getCalled      bool
	listCalled     bool
}

func (f *fakePaymentService) InitiatePayment(_ context.Context, input service.InitiatePaymentInput) (*service.PaymentInitiation, error) {
	f.initiateCalled = true
	f.initiateInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.initiation, nil
}

func (f *fakePaymentService) HandleWebhook(_ context.Context, input service.WebhookInput) (*service.WebhookResult, error) {
	f.webhookCalled = true
	f.webhookInput = input
	if f.err != nil {
		return f.webhookResult, f.err
	}
	return f.webhookResult, nil
}

func (f *fakePaymentService) GetPaymentsByOrder(_ context.Context, input service.GetPaymentsByOrderInput) (*service.PaymentsByOrder, error) {
	f.getCalled = true
	f.getInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.payments, nil
}

func (f *fakePaymentService) ListPayments(_ context.Context, input service.ListPaymentsInput) (*service.PaymentList, error) {
	f.listCalled = true
	f.listInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

func authenticatedPaymentCustomerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var userID pgtype.UUID
		_ = userID.Scan("11111111-1111-4111-8111-111111111111")
		c.Set("authenticated_user", middleware.AuthenticatedUser{
			ID:          "11111111-1111-4111-8111-111111111111",
			UUID:        userID,
			Email:       "budi@example.test",
			FullName:    "Budi",
			Role:        repository.UserRoleCUSTOMER,
			IsVerified:  true,
			IsActive:    true,
			PhoneNumber: "+628123456789",
		})
		c.Next()
	}
}

func authenticatedPaymentAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var userID pgtype.UUID
		_ = userID.Scan("99999999-9999-4999-8999-999999999999")
		c.Set("authenticated_user", middleware.AuthenticatedUser{
			ID:         "99999999-9999-4999-8999-999999999999",
			UUID:       userID,
			Email:      "admin@example.test",
			FullName:   "Admin",
			Role:       repository.UserRoleADMIN,
			IsVerified: true,
			IsActive:   true,
		})
		c.Next()
	}
}

func timePtrForHandlerPayment(value time.Time) *time.Time {
	return &value
}
