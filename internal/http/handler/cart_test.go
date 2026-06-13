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

func TestCartHandlerGetCartReturnsAuthenticatedCustomerCart(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cartID := "22222222-2222-4222-8222-222222222222"
	updatedAt := time.Date(2026, 6, 13, 10, 0, 0, 0, time.UTC)
	fakeService := &fakeCartService{
		cart: &service.Cart{
			ID:        &cartID,
			UserID:    "11111111-1111-4111-8111-111111111111",
			UpdatedAt: &updatedAt,
			Items: []service.CartItem{
				{
					ItemID:      "33333333-3333-4333-8333-333333333333",
					ProductID:   "44444444-4444-4444-8444-444444444444",
					Name:        "Americano",
					ImageURL:    stringPtr("https://example.supabase.co/storage/v1/object/public/products/americano.png"),
					Price:       25000,
					Quantity:    2,
					Subtotal:    50000,
					IsAvailable: true,
				},
			},
			GrandTotal: 50000,
		},
	}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.GET("/cart", authenticatedCustomerMiddleware(), cartHandler.GetCart)

	req := httptest.NewRequest(http.MethodGet, "/cart", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.userID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("user id = %q", fakeService.userID)
	}
	body := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"cart_id":"22222222-2222-4222-8222-222222222222"`,
		`"name":"Americano"`,
		`"grand_total":50000`,
		`"message":"Cart berhasil diambil"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %s: %s", want, body)
		}
	}
}

func TestCartHandlerAddItemPassesDeltaQuantityToService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cartID := "22222222-2222-4222-8222-222222222222"
	fakeService := &fakeCartService{
		cart: &service.Cart{
			ID:         &cartID,
			UserID:     "11111111-1111-4111-8111-111111111111",
			Items:      []service.CartItem{},
			GrandTotal: 0,
		},
	}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.POST("/cart/items", authenticatedCustomerMiddleware(), cartHandler.AddItem)

	req := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader(`{"product_id":"44444444-4444-4444-8444-444444444444","quantity":2}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.addCalled {
		t.Fatalf("expected AddItem to be called")
	}
	if fakeService.addInput.ProductID != "44444444-4444-4444-8444-444444444444" {
		t.Fatalf("product id = %q", fakeService.addInput.ProductID)
	}
	if fakeService.addInput.Quantity != 2 {
		t.Fatalf("quantity = %d", fakeService.addInput.Quantity)
	}
	if !strings.Contains(resp.Body.String(), `"message":"Item berhasil ditambahkan ke cart"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestCartHandlerAddItemRejectsInvalidQuantity(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeCartService{}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.POST("/cart/items", authenticatedCustomerMiddleware(), cartHandler.AddItem)

	req := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader(`{"product_id":"44444444-4444-4444-8444-444444444444","quantity":0}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.addCalled {
		t.Fatalf("service should not be called for invalid quantity")
	}
	if !strings.Contains(resp.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestCartHandlerAddItemMapsOutOfStockError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeCartService{err: service.ErrCartProductOutOfStock}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.POST("/cart/items", authenticatedCustomerMiddleware(), cartHandler.AddItem)

	req := httptest.NewRequest(http.MethodPost, "/cart/items", strings.NewReader(`{"product_id":"44444444-4444-4444-8444-444444444444","quantity":1}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_OUT_OF_STOCK"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestCartHandlerUpdateItemPassesFinalQuantityToService(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cartID := "22222222-2222-4222-8222-222222222222"
	fakeService := &fakeCartService{
		cart: &service.Cart{
			ID:         &cartID,
			UserID:     "11111111-1111-4111-8111-111111111111",
			Items:      []service.CartItem{},
			GrandTotal: 0,
		},
	}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.PATCH("/cart/items/:item_id", authenticatedCustomerMiddleware(), cartHandler.UpdateItem)

	req := httptest.NewRequest(http.MethodPatch, "/cart/items/33333333-3333-4333-8333-333333333333", strings.NewReader(`{"quantity":3}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.updateCalled {
		t.Fatalf("expected UpdateItemQuantity to be called")
	}
	if fakeService.itemID != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("item id = %q", fakeService.itemID)
	}
	if fakeService.updateInput.Quantity != 3 {
		t.Fatalf("quantity = %d", fakeService.updateInput.Quantity)
	}
	if !strings.Contains(resp.Body.String(), `"message":"Quantity item cart berhasil diperbarui"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestCartHandlerUpdateItemRejectsInvalidItemID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeCartService{}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.PATCH("/cart/items/:item_id", authenticatedCustomerMiddleware(), cartHandler.UpdateItem)

	req := httptest.NewRequest(http.MethodPatch, "/cart/items/not-a-uuid", strings.NewReader(`{"quantity":3}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.updateCalled {
		t.Fatalf("service should not be called for invalid item id")
	}
	if !strings.Contains(resp.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestCartHandlerUpdateItemMapsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeCartService{err: service.ErrCartItemNotFound}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.PATCH("/cart/items/:item_id", authenticatedCustomerMiddleware(), cartHandler.UpdateItem)

	req := httptest.NewRequest(http.MethodPatch, "/cart/items/33333333-3333-4333-8333-333333333333", strings.NewReader(`{"quantity":3}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"CART_ITEM_NOT_FOUND"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestCartHandlerDeleteItemReturnsSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeCartService{}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.DELETE("/cart/items/:item_id", authenticatedCustomerMiddleware(), cartHandler.DeleteItem)

	req := httptest.NewRequest(http.MethodDelete, "/cart/items/33333333-3333-4333-8333-333333333333", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.deleteCalled {
		t.Fatalf("expected DeleteItem to be called")
	}
	if fakeService.itemID != "33333333-3333-4333-8333-333333333333" {
		t.Fatalf("item id = %q", fakeService.itemID)
	}
	if !strings.Contains(resp.Body.String(), `"message":"Item berhasil dihapus dari cart"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestCartHandlerDeleteItemRejectsInvalidItemID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeCartService{}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.DELETE("/cart/items/:item_id", authenticatedCustomerMiddleware(), cartHandler.DeleteItem)

	req := httptest.NewRequest(http.MethodDelete, "/cart/items/not-a-uuid", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.deleteCalled {
		t.Fatalf("service should not be called for invalid item id")
	}
}

func TestCartHandlerClearItemsReturnsSuccess(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeCartService{}
	router := gin.New()
	cartHandler := NewCartHandler(fakeService)
	router.DELETE("/cart/items", authenticatedCustomerMiddleware(), cartHandler.ClearItems)

	req := httptest.NewRequest(http.MethodDelete, "/cart/items", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.clearCalled {
		t.Fatalf("expected ClearItems to be called")
	}
	if !strings.Contains(resp.Body.String(), `"message":"Item berhasil dihapus dari cart"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

type fakeCartService struct {
	cart         *service.Cart
	err          error
	userID       string
	itemID       string
	addInput     service.AddCartItemInput
	updateInput  service.UpdateCartItemInput
	addCalled    bool
	updateCalled bool
	deleteCalled bool
	clearCalled  bool
}

func (f *fakeCartService) GetCart(_ context.Context, userID string) (*service.Cart, error) {
	f.userID = userID
	if f.err != nil {
		return nil, f.err
	}
	return f.cart, nil
}

func (f *fakeCartService) AddItem(_ context.Context, userID string, input service.AddCartItemInput) (*service.Cart, error) {
	f.addCalled = true
	f.userID = userID
	f.addInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.cart, nil
}

func (f *fakeCartService) UpdateItemQuantity(_ context.Context, userID string, itemID string, input service.UpdateCartItemInput) (*service.Cart, error) {
	f.updateCalled = true
	f.userID = userID
	f.itemID = itemID
	f.updateInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.cart, nil
}

func (f *fakeCartService) DeleteItem(_ context.Context, userID string, itemID string) error {
	f.deleteCalled = true
	f.userID = userID
	f.itemID = itemID
	return f.err
}

func (f *fakeCartService) ClearItems(_ context.Context, userID string) error {
	f.clearCalled = true
	f.userID = userID
	return f.err
}

func authenticatedCustomerMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var userID pgtype.UUID
		_ = userID.Scan("11111111-1111-4111-8111-111111111111")
		c.Set("authenticated_user", middleware.AuthenticatedUser{
			ID:       "11111111-1111-4111-8111-111111111111",
			UUID:     userID,
			Role:     repository.UserRoleCUSTOMER,
			IsActive: true,
		})
		c.Next()
	}
}
