package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

type fakeProductReader struct {
	list          *service.ProductList
	product       *service.Product
	err           error
	listInput     service.ListProductsInput
	createInput   service.CreateProductInput
	productID     string
	productCalled bool
	createCalled  bool
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

func stringPtr(value string) *string {
	return &value
}
