package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"cafeTelkom/internal/http/dto"
	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

type ProductHandler struct {
	productService productReader
	supabaseURL    string
}

type productReader interface {
	ListProducts(ctx context.Context, input service.ListProductsInput) (*service.ProductList, error)
	GetProduct(ctx context.Context, productID string) (*service.Product, error)
	CreateProduct(ctx context.Context, input service.CreateProductInput) (*service.Product, error)
	UpdateProduct(ctx context.Context, productID string, input service.UpdateProductInput) (*service.Product, error)
	UpdateProductStatus(ctx context.Context, productID string, input service.UpdateProductStatusInput) (*service.Product, error)
	DeleteProduct(ctx context.Context, productID string) error
	RestoreProduct(ctx context.Context, productID string) (*service.Product, error)
}

type ProductListResponse struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Price      int32           `json:"price"`
	Category   string          `json:"category"`
	Status     string          `json:"status"`
	ImageURL   *string         `json:"image_url"`
	Rating     float64         `json:"rating"`
	TotalSold  int32           `json:"total_sold"`
	Attributes json.RawMessage `json:"attributes"`
	CreatedAt  interface{}     `json:"created_at"`
	UpdatedAt  interface{}     `json:"updated_at"`
}

type ProductDetailResponse struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description *string         `json:"description"`
	Price       int32           `json:"price"`
	Category    string          `json:"category"`
	Status      string          `json:"status"`
	ImageURL    *string         `json:"image_url"`
	Rating      float64         `json:"rating"`
	TotalSold   int32           `json:"total_sold"`
	Attributes  json.RawMessage `json:"attributes"`
	CreatedAt   interface{}     `json:"created_at"`
	UpdatedAt   interface{}     `json:"updated_at"`
}

type productListEnvelope struct {
	Success    bool                  `json:"success"`
	Data       []ProductListResponse `json:"data"`
	Pagination productPagination     `json:"pagination"`
}

type productPagination struct {
	NextCursor *string `json:"next_cursor"`
	PrevCursor *string `json:"prev_cursor"`
	Limit      int32   `json:"limit"`
	HasNext    bool    `json:"has_next"`
	HasPrev    bool    `json:"has_prev"`
}

type CreateProductRequest struct {
	Name        string              `json:"name"`
	Description *string             `json:"description"`
	Price       int32               `json:"price"`
	Category    string              `json:"category"`
	Status      string              `json:"status"`
	ImageURL    string              `json:"image_url"`
	Attributes  map[string][]string `json:"attributes"`
}

type UpdateProductRequest struct {
	Name        *string              `json:"name"`
	Description *string              `json:"description"`
	Price       *int32               `json:"price"`
	Category    *string              `json:"category"`
	Status      *string              `json:"status"`
	ImageURL    *string              `json:"image_url"`
	Attributes  *map[string][]string `json:"attributes"`
}

type UpdateProductStatusRequest struct {
	Status string `json:"status"`
}

func NewProductHandler(productService productReader, supabaseURL ...string) *ProductHandler {
	handler := &ProductHandler{productService: productService}
	if len(supabaseURL) > 0 {
		handler.supabaseURL = supabaseURL[0]
	}
	return handler
}

func (h *ProductHandler) ListProducts(c *gin.Context) {
	if h.productService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Product service unavailable", nil)
		return
	}

	limit, ok := parseProductLimit(c.Query("limit"))
	if !ok {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"limit": "Limit harus berupa angka",
		})
		return
	}

	list, err := h.productService.ListProducts(c.Request.Context(), service.ListProductsInput{Limit: limit})
	if err != nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		return
	}
	if list == nil {
		list = &service.ProductList{Limit: limit}
	}

	c.JSON(http.StatusOK, productListEnvelope{
		Success: true,
		Data:    productListResponses(list.Items),
		Pagination: productPagination{
			Limit:   list.Limit,
			HasNext: list.HasNext,
			HasPrev: list.HasPrev,
		},
	})
}

func (h *ProductHandler) GetProduct(c *gin.Context) {
	if h.productService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Product service unavailable", nil)
		return
	}

	productID := c.Param("id")
	if !isValidUUID(productID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"id": "ID produk harus berupa UUID",
		})
		return
	}

	product, err := h.productService.GetProduct(c.Request.Context(), productID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProductNotFound):
			dto.WriteError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Produk tidak ditemukan", nil)
		case errors.Is(err, service.ErrInvalidProductID):
			dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
				"id": "ID produk harus berupa UUID",
			})
		default:
			dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		}
		return
	}

	dto.WriteSuccess(c, http.StatusOK, productDetailResponse(*product), "Produk berhasil diambil")
}

func (h *ProductHandler) CreateProduct(c *gin.Context) {
	if h.productService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Product service unavailable", nil)
		return
	}

	var req CreateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}

	req = normalizeCreateProductRequest(req)
	if validationErrors := validateCreateProductRequest(req, h.supabaseURL); len(validationErrors) > 0 {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", validationErrors)
		return
	}

	attributes, err := json.Marshal(req.Attributes)
	if err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"attributes": "Attributes tidak valid",
		})
		return
	}

	product, err := h.productService.CreateProduct(c.Request.Context(), service.CreateProductInput{
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		Category:    req.Category,
		Status:      req.Status,
		ImageURL:    req.ImageURL,
		Attributes:  attributes,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProductNameAlreadyExists):
			dto.WriteError(c, http.StatusConflict, "PRODUCT_NAME_ALREADY_EXISTS", "Nama produk sudah digunakan", nil)
		default:
			dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		}
		return
	}

	dto.WriteSuccess(c, http.StatusCreated, productDetailResponse(*product), "Produk berhasil dibuat")
}

func (h *ProductHandler) UpdateProduct(c *gin.Context) {
	if h.productService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Product service unavailable", nil)
		return
	}

	productID := c.Param("id")
	if !isValidUUID(productID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"id": "ID produk harus berupa UUID",
		})
		return
	}

	var req UpdateProductRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}

	req = normalizeUpdateProductRequest(req)
	if validationErrors := validateUpdateProductRequest(req, h.supabaseURL); len(validationErrors) > 0 {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", validationErrors)
		return
	}

	var attributes *[]byte
	if req.Attributes != nil {
		rawAttributes, err := json.Marshal(*req.Attributes)
		if err != nil {
			dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
				"attributes": "Attributes tidak valid",
			})
			return
		}
		attributes = &rawAttributes
	}

	product, err := h.productService.UpdateProduct(c.Request.Context(), productID, service.UpdateProductInput{
		Name:        req.Name,
		Description: req.Description,
		Price:       req.Price,
		Category:    req.Category,
		Status:      req.Status,
		ImageURL:    req.ImageURL,
		Attributes:  attributes,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProductNotFound):
			dto.WriteError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Produk tidak ditemukan", nil)
		case errors.Is(err, service.ErrProductNameAlreadyExists):
			dto.WriteError(c, http.StatusConflict, "PRODUCT_NAME_ALREADY_EXISTS", "Nama produk sudah digunakan", nil)
		case errors.Is(err, service.ErrInvalidProductID):
			dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
				"id": "ID produk harus berupa UUID",
			})
		default:
			dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		}
		return
	}

	dto.WriteSuccess(c, http.StatusOK, productDetailResponse(*product), "Produk berhasil diperbarui")
}

func (h *ProductHandler) UpdateProductStatus(c *gin.Context) {
	if h.productService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Product service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	productID := c.Param("id")
	if !isValidUUID(productID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"id": "ID produk harus berupa UUID",
		})
		return
	}

	var req UpdateProductStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}

	req.Status = strings.TrimSpace(req.Status)
	if !isAllowedValue(req.Status, []string{"available", "out_of_stock", "unavailable"}) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"status": "Status harus available, out_of_stock, atau unavailable",
		})
		return
	}

	product, err := h.productService.UpdateProductStatus(c.Request.Context(), productID, service.UpdateProductStatusInput{
		Status:    req.Status,
		ActorRole: string(user.Role),
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProductNotFound):
			dto.WriteError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Produk tidak ditemukan", nil)
		case errors.Is(err, service.ErrProductStatusForbidden):
			dto.WriteError(c, http.StatusForbidden, "FORBIDDEN", "Role tidak diizinkan", nil)
		case errors.Is(err, service.ErrProductAlreadyDeleted):
			dto.WriteError(c, http.StatusConflict, "PRODUCT_ALREADY_DELETED", "Produk sudah dihapus", nil)
		case errors.Is(err, service.ErrInvalidProductID):
			dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
				"id": "ID produk harus berupa UUID",
			})
		default:
			dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		}
		return
	}

	dto.WriteSuccess(c, http.StatusOK, productDetailResponse(*product), "Status produk berhasil diperbarui")
}

func (h *ProductHandler) DeleteProduct(c *gin.Context) {
	if h.productService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Product service unavailable", nil)
		return
	}

	productID := c.Param("id")
	if !isValidUUID(productID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"id": "ID produk harus berupa UUID",
		})
		return
	}

	if err := h.productService.DeleteProduct(c.Request.Context(), productID); err != nil {
		switch {
		case errors.Is(err, service.ErrProductNotFound):
			dto.WriteError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Produk tidak ditemukan", nil)
		case errors.Is(err, service.ErrProductAlreadyDeleted):
			dto.WriteError(c, http.StatusConflict, "PRODUCT_ALREADY_DELETED", "Produk sudah dihapus", nil)
		case errors.Is(err, service.ErrInvalidProductID):
			dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
				"id": "ID produk harus berupa UUID",
			})
		default:
			dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		}
		return
	}

	dto.WriteSuccess(c, http.StatusOK, nil, "Produk berhasil dihapus")
}

func (h *ProductHandler) RestoreProduct(c *gin.Context) {
	if h.productService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Product service unavailable", nil)
		return
	}

	productID := c.Param("id")
	if !isValidUUID(productID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"id": "ID produk harus berupa UUID",
		})
		return
	}

	product, err := h.productService.RestoreProduct(c.Request.Context(), productID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrProductNotFound):
			dto.WriteError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Produk tidak ditemukan", nil)
		case errors.Is(err, service.ErrProductNotDeleted):
			dto.WriteError(c, http.StatusConflict, "PRODUCT_NOT_DELETED", "Produk belum dihapus", nil)
		case errors.Is(err, service.ErrInvalidProductID):
			dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
				"id": "ID produk harus berupa UUID",
			})
		default:
			dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		}
		return
	}

	dto.WriteSuccess(c, http.StatusOK, productDetailResponse(*product), "Produk berhasil dipulihkan")
}

func parseProductLimit(raw string) (int32, bool) {
	if raw == "" {
		return 10, true
	}

	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	if value <= 0 {
		return 10, true
	}
	if value > 50 {
		return 50, true
	}
	return int32(value), true
}

func productListResponses(products []service.Product) []ProductListResponse {
	responses := make([]ProductListResponse, 0, len(products))
	for _, product := range products {
		responses = append(responses, ProductListResponse{
			ID:         product.ID,
			Name:       product.Name,
			Price:      product.Price,
			Category:   product.Category,
			Status:     product.Status,
			ImageURL:   product.ImageURL,
			Rating:     product.Rating,
			TotalSold:  product.TotalSold,
			Attributes: jsonAttributes(product.Attributes),
			CreatedAt:  product.CreatedAt,
			UpdatedAt:  product.UpdatedAt,
		})
	}
	return responses
}

func productDetailResponse(product service.Product) ProductDetailResponse {
	return ProductDetailResponse{
		ID:          product.ID,
		Name:        product.Name,
		Description: product.Description,
		Price:       product.Price,
		Category:    product.Category,
		Status:      product.Status,
		ImageURL:    product.ImageURL,
		Rating:      product.Rating,
		TotalSold:   product.TotalSold,
		Attributes:  jsonAttributes(product.Attributes),
		CreatedAt:   product.CreatedAt,
		UpdatedAt:   product.UpdatedAt,
	}
}

func jsonAttributes(attributes []byte) json.RawMessage {
	if len(attributes) == 0 {
		return json.RawMessage(`{}`)
	}
	return json.RawMessage(attributes)
}

func isValidUUID(value string) bool {
	var id pgtype.UUID
	return id.Scan(value) == nil
}

func normalizeCreateProductRequest(req CreateProductRequest) CreateProductRequest {
	req.Name = strings.TrimSpace(req.Name)
	req.Category = strings.TrimSpace(req.Category)
	req.Status = strings.TrimSpace(req.Status)
	req.ImageURL = strings.TrimSpace(req.ImageURL)
	if req.Status == "" {
		req.Status = string("available")
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		req.Description = &description
	}
	return req
}

func normalizeUpdateProductRequest(req UpdateProductRequest) UpdateProductRequest {
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		req.Name = &name
	}
	if req.Description != nil {
		description := strings.TrimSpace(*req.Description)
		req.Description = &description
	}
	if req.Category != nil {
		category := strings.TrimSpace(*req.Category)
		req.Category = &category
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		req.Status = &status
	}
	if req.ImageURL != nil {
		imageURL := strings.TrimSpace(*req.ImageURL)
		req.ImageURL = &imageURL
	}
	return req
}

func validateCreateProductRequest(req CreateProductRequest, supabaseURL string) map[string]string {
	errors := make(map[string]string)

	if req.Name == "" {
		errors["name"] = "Nama produk wajib diisi"
	} else if len(req.Name) < 3 || len(req.Name) > 100 {
		errors["name"] = "Nama produk harus 3-100 karakter"
	}

	if req.Description != nil && len(*req.Description) > 500 {
		errors["description"] = "Deskripsi maksimal 500 karakter"
	}

	if req.Price < 0 || req.Price > 99999999 {
		errors["price"] = "Harga harus 0-99999999"
	}

	if !isAllowedValue(req.Category, []string{"coffee", "food", "snack"}) {
		errors["category"] = "Kategori harus coffee, food, atau snack"
	}

	if !isAllowedValue(req.Status, []string{"available", "out_of_stock", "unavailable"}) {
		errors["status"] = "Status harus available, out_of_stock, atau unavailable"
	}

	if !isSupabaseProductStorageURL(req.ImageURL, supabaseURL) {
		errors["image_url"] = "URL gambar harus berasal dari Supabase Storage project ini"
	}

	for field, message := range validateProductAttributes(req.Category, req.Attributes) {
		errors[field] = message
	}

	return errors
}

func validateUpdateProductRequest(req UpdateProductRequest, supabaseURL string) map[string]string {
	errors := make(map[string]string)

	if req.Name != nil {
		if *req.Name == "" {
			errors["name"] = "Nama produk wajib diisi"
		} else if len(*req.Name) < 3 || len(*req.Name) > 100 {
			errors["name"] = "Nama produk harus 3-100 karakter"
		}
	}

	if req.Description != nil && len(*req.Description) > 500 {
		errors["description"] = "Deskripsi maksimal 500 karakter"
	}

	if req.Price != nil && (*req.Price < 0 || *req.Price > 99999999) {
		errors["price"] = "Harga harus 0-99999999"
	}

	if req.Category != nil && !isAllowedValue(*req.Category, []string{"coffee", "food", "snack"}) {
		errors["category"] = "Kategori harus coffee, food, atau snack"
	}

	if req.Status != nil && !isAllowedValue(*req.Status, []string{"available", "out_of_stock", "unavailable"}) {
		errors["status"] = "Status harus available, out_of_stock, atau unavailable"
	}

	if req.ImageURL != nil && !isSupabaseProductStorageURL(*req.ImageURL, supabaseURL) {
		errors["image_url"] = "URL gambar harus berasal dari Supabase Storage project ini"
	}

	if req.Category != nil && req.Attributes == nil {
		errors["attributes"] = "Attributes wajib dikirim saat category diubah"
	}
	if req.Attributes != nil {
		if req.Category == nil {
			errors["category"] = "Kategori wajib dikirim saat attributes diubah"
		} else {
			for field, message := range validateProductAttributes(*req.Category, *req.Attributes) {
				errors[field] = message
			}
		}
	}

	return errors
}

func validateProductAttributes(category string, attributes map[string][]string) map[string]string {
	errors := make(map[string]string)
	if attributes == nil {
		errors["attributes"] = "Attributes wajib diisi"
		return errors
	}

	switch category {
	case "coffee":
		validateRequiredValues(errors, attributes, "temperature", []string{"hot", "iced"})
		validateRequiredValues(errors, attributes, "sugar_levels", []string{"normal", "less", "no_sugar"})
		validateRequiredValues(errors, attributes, "ice_levels", []string{"normal", "less", "no_ice"})
		validateRequiredValues(errors, attributes, "sizes", []string{"small", "medium", "large"})
	case "food", "snack":
		validateRequiredValues(errors, attributes, "portions", []string{"regular", "large"})
		if values, ok := attributes["spicy_levels"]; ok {
			validateValues(errors, "spicy_levels", values, []string{"no_spicy", "mild", "medium", "hot"}, false)
		}
	}

	return errors
}

func validateRequiredValues(errors map[string]string, attributes map[string][]string, field string, allowed []string) {
	values, ok := attributes[field]
	if !ok || len(values) == 0 {
		errors["attributes."+field] = "Wajib diisi"
		return
	}
	validateValues(errors, field, values, allowed, true)
}

func validateValues(errors map[string]string, field string, values []string, allowed []string, required bool) {
	if !required && len(values) == 0 {
		errors["attributes."+field] = "Minimal 1 value jika dikirim"
		return
	}

	for _, value := range values {
		if !isAllowedValue(value, allowed) {
			errors["attributes."+field] = "Memuat value yang tidak valid"
			return
		}
	}
}

func isAllowedValue(value string, allowed []string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}

func isSupabaseProductStorageURL(rawImageURL, rawSupabaseURL string) bool {
	imageURL, err := url.Parse(rawImageURL)
	if err != nil || imageURL.Scheme != "https" || imageURL.Host == "" {
		return false
	}

	if !strings.HasPrefix(imageURL.Path, "/storage/v1/object/public/products/") {
		return false
	}

	if rawSupabaseURL == "" {
		return strings.HasSuffix(strings.ToLower(imageURL.Host), ".supabase.co")
	}

	supabaseURL, err := url.Parse(rawSupabaseURL)
	if err != nil || supabaseURL.Host == "" {
		return false
	}

	return strings.EqualFold(imageURL.Host, supabaseURL.Host)
}
