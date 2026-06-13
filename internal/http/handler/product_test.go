package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/repository"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
)

func TestProductHandlerListProductsReturnsProducts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{
		list: &service.ProductList{
			Items: []service.Product{
				{
					ID:         "11111111-1111-4111-8111-111111111111",
					Name:       "Americano",
					Price:      25000,
					Category:   "coffee",
					Status:     "available",
					ImageURL:   stringPtr("https://example.supabase.co/storage/v1/object/public/products/americano.png"),
					Rating:     4.5,
					TotalSold:  120,
					Attributes: []byte(`{"temperature":["hot","iced"]}`),
					CreatedAt:  time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
					UpdatedAt:  time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
				},
			},
			Limit:   10,
			HasNext: false,
			HasPrev: false,
		},
	}

	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.GET("/products", productHandler.ListProducts)

	req := httptest.NewRequest(http.MethodGet, "/products", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.listInput.Limit != 10 {
		t.Fatalf("limit = %d", fakeService.listInput.Limit)
	}

	body := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"name":"Americano"`,
		`"category":"coffee"`,
		`"status":"available"`,
		`"rating":4.5`,
		`"limit":10`,
		`"has_next":false`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %s: %s", want, body)
		}
	}
}

func TestProductHandlerListProductsClampsLimit(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{
		list: &service.ProductList{Limit: 50},
	}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.GET("/products", productHandler.ListProducts)

	req := httptest.NewRequest(http.MethodGet, "/products?limit=100", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.listInput.Limit != 50 {
		t.Fatalf("limit = %d", fakeService.listInput.Limit)
	}
}

func TestProductHandlerListProductsPassesIncludeDeleted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{
		list: &service.ProductList{Limit: 10},
	}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.GET("/products", productHandler.ListProducts)

	req := httptest.NewRequest(http.MethodGet, "/products?include_deleted=true", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.listInput.IncludeDeleted {
		t.Fatalf("expected include deleted to be passed to service")
	}
}

func TestProductHandlerListProductsReturnsDeletedAtForSoftDeletedProduct(t *testing.T) {
	gin.SetMode(gin.TestMode)

	deletedAt := time.Date(2026, 5, 26, 2, 0, 0, 0, time.UTC)
	fakeService := &fakeProductReader{
		list: &service.ProductList{
			Items: []service.Product{
				{
					ID:         "11111111-1111-4111-8111-111111111111",
					Name:       "Deleted Latte",
					Price:      25000,
					Category:   "coffee",
					Status:     "unavailable",
					ImageURL:   stringPtr("https://example.supabase.co/storage/v1/object/public/products/deleted-latte.png"),
					Rating:     4.5,
					TotalSold:  120,
					Attributes: []byte(`{"temperature":["hot","iced"]}`),
					CreatedAt:  time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
					UpdatedAt:  deletedAt,
					DeletedAt:  &deletedAt,
				},
			},
			Limit: 10,
		},
	}

	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.GET("/products", productHandler.ListProducts)

	req := httptest.NewRequest(http.MethodGet, "/products?include_deleted=true", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}

	body := resp.Body.String()
	if !strings.Contains(body, `"deleted_at":"2026-05-26T02:00:00Z"`) {
		t.Fatalf("response body missing deleted_at: %s", body)
	}
}

func TestProductHandlerGetProductReturnsProduct(t *testing.T) {
	gin.SetMode(gin.TestMode)

	description := "Espresso dengan air panas"
	fakeService := &fakeProductReader{
		product: &service.Product{
			ID:          "11111111-1111-4111-8111-111111111111",
			Name:        "Americano",
			Description: &description,
			Price:       25000,
			Category:    "coffee",
			Status:      "available",
			ImageURL:    stringPtr("https://example.supabase.co/storage/v1/object/public/products/americano.png"),
			Rating:      4.5,
			TotalSold:   120,
			Attributes:  []byte(`{"temperature":["hot","iced"]}`),
			CreatedAt:   time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
		},
	}

	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.GET("/products/:id", productHandler.GetProduct)

	req := httptest.NewRequest(http.MethodGet, "/products/11111111-1111-4111-8111-111111111111", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.productID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("product id = %q", fakeService.productID)
	}

	body := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"name":"Americano"`,
		`"description":"Espresso dengan air panas"`,
		`"message":"Produk berhasil diambil"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %s: %s", want, body)
		}
	}
}

func TestProductHandlerGetProductRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.GET("/products/:id", productHandler.GetProduct)

	req := httptest.NewRequest(http.MethodGet, "/products/not-a-uuid", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.productCalled {
		t.Fatalf("service should not be called for invalid uuid")
	}
	if !strings.Contains(resp.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerGetProductReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductNotFound}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.GET("/products/:id", productHandler.GetProduct)

	req := httptest.NewRequest(http.MethodGet, "/products/11111111-1111-4111-8111-111111111111", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_NOT_FOUND"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerCreateProductReturnsCreated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	description := "Espresso dengan air panas"
	fakeService := &fakeProductReader{
		product: &service.Product{
			ID:          "11111111-1111-4111-8111-111111111111",
			Name:        "Americano",
			Description: &description,
			Price:       25000,
			Category:    "coffee",
			Status:      "available",
			ImageURL:    stringPtr("https://example.supabase.co/storage/v1/object/public/products/americano.png"),
			Rating:      0,
			TotalSold:   0,
			Attributes:  []byte(`{"temperature":["hot","iced"],"sugar_levels":["normal"],"ice_levels":["normal"],"sizes":["medium"]}`),
			CreatedAt:   time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
		},
	}

	router := gin.New()
	productHandler := NewProductHandler(fakeService, "https://example.supabase.co")
	router.POST("/products", productHandler.CreateProduct)

	body := `{
		"name":"Americano",
		"description":"Espresso dengan air panas",
		"price":25000,
		"category":"coffee",
		"status":"available",
		"image_url":"https://example.supabase.co/storage/v1/object/public/products/americano.png",
		"attributes":{
			"temperature":["hot","iced"],
			"sugar_levels":["normal"],
			"ice_levels":["normal"],
			"sizes":["medium"]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.createInput.Name != "Americano" {
		t.Fatalf("create name = %q", fakeService.createInput.Name)
	}
	if fakeService.createInput.Status != "available" {
		t.Fatalf("create status = %q", fakeService.createInput.Status)
	}

	responseBody := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"name":"Americano"`,
		`"message":"Produk berhasil dibuat"`,
	} {
		if !strings.Contains(responseBody, want) {
			t.Fatalf("response body missing %s: %s", want, responseBody)
		}
	}
}

func TestProductHandlerCreateProductRejectsInvalidAttributes(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{}
	router := gin.New()
	productHandler := NewProductHandler(fakeService, "https://example.supabase.co")
	router.POST("/products", productHandler.CreateProduct)

	body := `{
		"name":"Americano",
		"price":25000,
		"category":"coffee",
		"image_url":"https://example.supabase.co/storage/v1/object/public/products/americano.png",
		"attributes":{
			"temperature":["iced"],
			"sugar_levels":["normal"],
			"sizes":["medium"]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.createCalled {
		t.Fatalf("service should not be called for invalid attributes")
	}
	if !strings.Contains(resp.Body.String(), `"attributes.ice_levels"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerCreateProductReturnsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductNameAlreadyExists}
	router := gin.New()
	productHandler := NewProductHandler(fakeService, "https://example.supabase.co")
	router.POST("/products", productHandler.CreateProduct)

	body := `{
		"name":"Americano",
		"price":25000,
		"category":"coffee",
		"image_url":"https://example.supabase.co/storage/v1/object/public/products/americano.png",
		"attributes":{
			"temperature":["hot"],
			"sugar_levels":["normal"],
			"ice_levels":["normal"],
			"sizes":["medium"]
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/products", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_NAME_ALREADY_EXISTS"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerUpdateProductReturnsUpdated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	description := "Espresso dengan air panas"
	fakeService := &fakeProductReader{
		product: &service.Product{
			ID:          "11111111-1111-4111-8111-111111111111",
			Name:        "Americano",
			Description: &description,
			Price:       28000,
			Category:    "coffee",
			Status:      "available",
			ImageURL:    stringPtr("https://example.supabase.co/storage/v1/object/public/products/americano.png"),
			Rating:      4.5,
			TotalSold:   120,
			Attributes:  []byte(`{"temperature":["hot","iced"],"sugar_levels":["normal"],"ice_levels":["normal"],"sizes":["medium"]}`),
			CreatedAt:   time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
			UpdatedAt:   time.Date(2026, 5, 26, 2, 0, 0, 0, time.UTC),
		},
	}

	router := gin.New()
	productHandler := NewProductHandler(fakeService, "https://example.supabase.co")
	router.PUT("/products/:id", productHandler.UpdateProduct)

	body := `{"price":28000}`
	req := httptest.NewRequest(http.MethodPut, "/products/11111111-1111-4111-8111-111111111111", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.updateProductID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("product id = %q", fakeService.updateProductID)
	}
	if fakeService.updateInput.Price == nil || *fakeService.updateInput.Price != 28000 {
		t.Fatalf("update price = %v", fakeService.updateInput.Price)
	}
	bodyText := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"price":28000`,
		`"message":"Produk berhasil diperbarui"`,
	} {
		if !strings.Contains(bodyText, want) {
			t.Fatalf("response body missing %s: %s", want, bodyText)
		}
	}
}

func TestProductHandlerUpdateProductRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PUT("/products/:id", productHandler.UpdateProduct)

	req := httptest.NewRequest(http.MethodPut, "/products/not-a-uuid", strings.NewReader(`{"price":28000}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.updateCalled {
		t.Fatalf("service should not be called for invalid uuid")
	}
	if !strings.Contains(resp.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerUpdateProductRejectsInvalidFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{}
	router := gin.New()
	productHandler := NewProductHandler(fakeService, "https://example.supabase.co")
	router.PUT("/products/:id", productHandler.UpdateProduct)

	body := `{"name":"A","price":-1}`
	req := httptest.NewRequest(http.MethodPut, "/products/11111111-1111-4111-8111-111111111111", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.updateCalled {
		t.Fatalf("service should not be called for invalid fields")
	}
	if !strings.Contains(resp.Body.String(), `"name"`) || !strings.Contains(resp.Body.String(), `"price"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerUpdateProductReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductNotFound}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PUT("/products/:id", productHandler.UpdateProduct)

	req := httptest.NewRequest(http.MethodPut, "/products/11111111-1111-4111-8111-111111111111", strings.NewReader(`{"price":28000}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_NOT_FOUND"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerUpdateProductReturnsConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductNameAlreadyExists}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PUT("/products/:id", productHandler.UpdateProduct)

	req := httptest.NewRequest(http.MethodPut, "/products/11111111-1111-4111-8111-111111111111", strings.NewReader(`{"name":"Cafe Latte"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_NAME_ALREADY_EXISTS"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerUpdateProductStatusReturnsUpdated(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{
		product: &service.Product{
			ID:        "11111111-1111-4111-8111-111111111111",
			Name:      "Americano",
			Price:     25000,
			Category:  "coffee",
			Status:    "out_of_stock",
			CreatedAt: time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 5, 26, 2, 0, 0, 0, time.UTC),
		},
	}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/status", func(c *gin.Context) {
		c.Set("authenticated_user", middleware.AuthenticatedUser{Role: repository.UserRoleADMIN})
		c.Next()
	}, productHandler.UpdateProductStatus)

	req := httptest.NewRequest(http.MethodPatch, "/products/11111111-1111-4111-8111-111111111111/status", strings.NewReader(`{"status":"out_of_stock"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.updateStatusProductID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("product id = %q", fakeService.updateStatusProductID)
	}
	if fakeService.updateStatusInput.Status != "out_of_stock" {
		t.Fatalf("status input = %q", fakeService.updateStatusInput.Status)
	}
	if fakeService.updateStatusInput.ActorRole != string(repository.UserRoleADMIN) {
		t.Fatalf("actor role = %q", fakeService.updateStatusInput.ActorRole)
	}
	if !strings.Contains(resp.Body.String(), `"message":"Status produk berhasil diperbarui"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerUpdateProductStatusRejectsMissingAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/status", productHandler.UpdateProductStatus)

	req := httptest.NewRequest(http.MethodPatch, "/products/11111111-1111-4111-8111-111111111111/status", strings.NewReader(`{"status":"available"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.updateStatusCalled {
		t.Fatalf("service should not be called")
	}
}

func TestProductHandlerUpdateProductStatusRejectsInvalidStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/status", func(c *gin.Context) {
		c.Set("authenticated_user", middleware.AuthenticatedUser{Role: repository.UserRoleADMIN})
		c.Next()
	}, productHandler.UpdateProductStatus)

	req := httptest.NewRequest(http.MethodPatch, "/products/11111111-1111-4111-8111-111111111111/status", strings.NewReader(`{"status":"invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.updateStatusCalled {
		t.Fatalf("service should not be called")
	}
	if !strings.Contains(resp.Body.String(), `"status"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerUpdateProductStatusReturnsForbiddenForPegawaiUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductStatusForbidden}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/status", func(c *gin.Context) {
		c.Set("authenticated_user", middleware.AuthenticatedUser{Role: repository.UserRolePEGAWAI})
		c.Next()
	}, productHandler.UpdateProductStatus)

	req := httptest.NewRequest(http.MethodPatch, "/products/11111111-1111-4111-8111-111111111111/status", strings.NewReader(`{"status":"unavailable"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusForbidden {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"FORBIDDEN"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerUpdateProductStatusReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductNotFound}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/status", func(c *gin.Context) {
		c.Set("authenticated_user", middleware.AuthenticatedUser{Role: repository.UserRoleADMIN})
		c.Next()
	}, productHandler.UpdateProductStatus)

	req := httptest.NewRequest(http.MethodPatch, "/products/11111111-1111-4111-8111-111111111111/status", strings.NewReader(`{"status":"available"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_NOT_FOUND"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerUpdateProductStatusReturnsAlreadyDeletedConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductAlreadyDeleted}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/status", func(c *gin.Context) {
		c.Set("authenticated_user", middleware.AuthenticatedUser{Role: repository.UserRoleADMIN})
		c.Next()
	}, productHandler.UpdateProductStatus)

	req := httptest.NewRequest(http.MethodPatch, "/products/11111111-1111-4111-8111-111111111111/status", strings.NewReader(`{"status":"available"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_ALREADY_DELETED"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerDeleteProductReturnsDeleted(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.DELETE("/products/:id", productHandler.DeleteProduct)

	req := httptest.NewRequest(http.MethodDelete, "/products/11111111-1111-4111-8111-111111111111", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.deleteProductID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("product id = %q", fakeService.deleteProductID)
	}
	if !strings.Contains(resp.Body.String(), `"message":"Produk berhasil dihapus"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerDeleteProductRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.DELETE("/products/:id", productHandler.DeleteProduct)

	req := httptest.NewRequest(http.MethodDelete, "/products/not-a-uuid", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.deleteCalled {
		t.Fatalf("service should not be called for invalid uuid")
	}
	if !strings.Contains(resp.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerDeleteProductReturnsAlreadyDeletedConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductAlreadyDeleted}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.DELETE("/products/:id", productHandler.DeleteProduct)

	req := httptest.NewRequest(http.MethodDelete, "/products/11111111-1111-4111-8111-111111111111", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_ALREADY_DELETED"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerDeleteProductReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductNotFound}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.DELETE("/products/:id", productHandler.DeleteProduct)

	req := httptest.NewRequest(http.MethodDelete, "/products/11111111-1111-4111-8111-111111111111", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_NOT_FOUND"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerRestoreProductReturnsRestored(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{
		product: &service.Product{
			ID:        "11111111-1111-4111-8111-111111111111",
			Name:      "Americano",
			Price:     25000,
			Category:  "coffee",
			Status:    "available",
			CreatedAt: time.Date(2026, 5, 26, 1, 0, 0, 0, time.UTC),
			UpdatedAt: time.Date(2026, 5, 26, 2, 0, 0, 0, time.UTC),
		},
	}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/restore", productHandler.RestoreProduct)

	req := httptest.NewRequest(http.MethodPatch, "/products/11111111-1111-4111-8111-111111111111/restore", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.restoreProductID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("product id = %q", fakeService.restoreProductID)
	}
	body := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"status":"available"`,
		`"message":"Produk berhasil dipulihkan"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %s: %s", want, body)
		}
	}
}

func TestProductHandlerRestoreProductRejectsInvalidID(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/restore", productHandler.RestoreProduct)

	req := httptest.NewRequest(http.MethodPatch, "/products/not-a-uuid/restore", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.restoreCalled {
		t.Fatalf("service should not be called for invalid uuid")
	}
	if !strings.Contains(resp.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerRestoreProductReturnsNotDeletedConflict(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductNotDeleted}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/restore", productHandler.RestoreProduct)

	req := httptest.NewRequest(http.MethodPatch, "/products/11111111-1111-4111-8111-111111111111/restore", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_NOT_DELETED"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestProductHandlerRestoreProductReturnsNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeProductReader{err: service.ErrProductNotFound}
	router := gin.New()
	productHandler := NewProductHandler(fakeService)
	router.PATCH("/products/:id/restore", productHandler.RestoreProduct)

	req := httptest.NewRequest(http.MethodPatch, "/products/11111111-1111-4111-8111-111111111111/restore", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"code":"PRODUCT_NOT_FOUND"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

type fakeProductReader struct {
	list                  *service.ProductList
	product               *service.Product
	err                   error
	listInput             service.ListProductsInput
	createInput           service.CreateProductInput
	updateInput           service.UpdateProductInput
	updateStatusInput     service.UpdateProductStatusInput
	productID             string
	updateProductID       string
	updateStatusProductID string
	deleteProductID       string
	restoreProductID      string
	productCalled         bool
	createCalled          bool
	updateCalled          bool
	updateStatusCalled    bool
	deleteCalled          bool
	restoreCalled         bool
}

func (f *fakeProductReader) ListProducts(_ context.Context, input service.ListProductsInput) (*service.ProductList, error) {
	f.listInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.list, nil
}

func (f *fakeProductReader) GetProduct(_ context.Context, productID string) (*service.Product, error) {
	f.productCalled = true
	f.productID = productID
	if f.err != nil {
		return nil, f.err
	}
	if f.product == nil {
		return nil, errors.New("missing product")
	}
	return f.product, nil
}

func (f *fakeProductReader) CreateProduct(_ context.Context, input service.CreateProductInput) (*service.Product, error) {
	f.createCalled = true
	f.createInput = input
	if f.err != nil {
		return nil, f.err
	}
	if f.product == nil {
		return nil, errors.New("missing product")
	}
	return f.product, nil
}

func (f *fakeProductReader) UpdateProduct(_ context.Context, productID string, input service.UpdateProductInput) (*service.Product, error) {
	f.updateCalled = true
	f.updateProductID = productID
	f.updateInput = input
	if f.err != nil {
		return nil, f.err
	}
	if f.product == nil {
		return nil, errors.New("missing product")
	}
	return f.product, nil
}

func (f *fakeProductReader) UpdateProductStatus(_ context.Context, productID string, input service.UpdateProductStatusInput) (*service.Product, error) {
	f.updateStatusCalled = true
	f.updateStatusProductID = productID
	f.updateStatusInput = input
	if f.err != nil {
		return nil, f.err
	}
	if f.product == nil {
		return nil, errors.New("missing product")
	}
	return f.product, nil
}

func (f *fakeProductReader) DeleteProduct(_ context.Context, productID string) error {
	f.deleteCalled = true
	f.deleteProductID = productID
	return f.err
}

func (f *fakeProductReader) RestoreProduct(_ context.Context, productID string) (*service.Product, error) {
	f.restoreCalled = true
	f.restoreProductID = productID
	if f.err != nil {
		return nil, f.err
	}
	if f.product == nil {
		return nil, errors.New("missing product")
	}
	return f.product, nil
}

func stringPtr(value string) *string {
	return &value
}
