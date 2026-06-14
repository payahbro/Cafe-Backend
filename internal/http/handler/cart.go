package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"cafeTelkom/internal/http/dto"
	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
)

type CartHandler struct {
	cartService    cartManager
	internalAPIKey string
}

type cartManager interface {
	GetCart(ctx context.Context, userID string) (*service.Cart, error)
	AddItem(ctx context.Context, userID string, input service.AddCartItemInput) (*service.Cart, error)
	UpdateItemQuantity(ctx context.Context, userID string, itemID string, input service.UpdateCartItemInput) (*service.Cart, error)
	DeleteItem(ctx context.Context, userID string, itemID string) error
	ClearItems(ctx context.Context, userID string) error
	ClearItemsByIDs(ctx context.Context, itemIDs []string) error
}

type AddCartItemRequest struct {
	ProductID string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type UpdateCartItemRequest struct {
	Quantity int32 `json:"quantity"`
}

type ClearInternalCartItemsRequest struct {
	ItemIDs []string `json:"item_ids"`
}

type CartResponse struct {
	CartID     *string            `json:"cart_id"`
	UserID     string             `json:"user_id"`
	Items      []CartItemResponse `json:"items"`
	GrandTotal int32              `json:"grand_total"`
	UpdatedAt  *time.Time         `json:"updated_at"`
}

type CartItemResponse struct {
	ItemID      string  `json:"item_id"`
	ProductID   string  `json:"product_id"`
	Name        string  `json:"name"`
	ImageURL    *string `json:"image_url"`
	Price       int32   `json:"price"`
	Quantity    int32   `json:"quantity"`
	Subtotal    int32   `json:"subtotal"`
	IsAvailable bool    `json:"is_available"`
}

func NewCartHandler(cartService cartManager, internalAPIKey ...string) *CartHandler {
	handler := &CartHandler{cartService: cartService}
	if len(internalAPIKey) > 0 {
		handler.internalAPIKey = internalAPIKey[0]
	}
	return handler
}

func (h *CartHandler) GetCart(c *gin.Context) {
	if h.cartService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Cart service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	cart, err := h.cartService.GetCart(c.Request.Context(), user.ID)
	if err != nil {
		writeCartServiceError(c, err)
		return
	}

	dto.WriteSuccess(c, http.StatusOK, cartResponse(cart), "Cart berhasil diambil")
}

func (h *CartHandler) AddItem(c *gin.Context) {
	if h.cartService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Cart service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	var req AddCartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}

	req.ProductID = strings.TrimSpace(req.ProductID)
	if validationErrors := validateAddCartItemRequest(req); len(validationErrors) > 0 {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", validationErrors)
		return
	}

	cart, err := h.cartService.AddItem(c.Request.Context(), user.ID, service.AddCartItemInput{
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
	})
	if err != nil {
		writeCartServiceError(c, err)
		return
	}

	dto.WriteSuccess(c, http.StatusOK, cartResponse(cart), "Item berhasil ditambahkan ke cart")
}

func (h *CartHandler) UpdateItem(c *gin.Context) {
	if h.cartService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Cart service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	itemID := strings.TrimSpace(c.Param("item_id"))
	if !isValidUUID(itemID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"item_id": "ID item cart harus berupa UUID",
		})
		return
	}

	var req UpdateCartItemRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}
	if req.Quantity < 1 {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"quantity": "Quantity minimal 1",
		})
		return
	}

	cart, err := h.cartService.UpdateItemQuantity(c.Request.Context(), user.ID, itemID, service.UpdateCartItemInput{
		Quantity: req.Quantity,
	})
	if err != nil {
		writeCartServiceError(c, err)
		return
	}

	dto.WriteSuccess(c, http.StatusOK, cartResponse(cart), "Quantity item cart berhasil diperbarui")
}

func (h *CartHandler) DeleteItem(c *gin.Context) {
	if h.cartService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Cart service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	itemID := strings.TrimSpace(c.Param("item_id"))
	if !isValidUUID(itemID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"item_id": "ID item cart harus berupa UUID",
		})
		return
	}

	if err := h.cartService.DeleteItem(c.Request.Context(), user.ID, itemID); err != nil {
		writeCartServiceError(c, err)
		return
	}

	dto.WriteSuccess(c, http.StatusOK, nil, "Item berhasil dihapus dari cart")
}

func (h *CartHandler) ClearItems(c *gin.Context) {
	if h.cartService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Cart service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	if err := h.cartService.ClearItems(c.Request.Context(), user.ID); err != nil {
		writeCartServiceError(c, err)
		return
	}

	dto.WriteSuccess(c, http.StatusOK, nil, "Item berhasil dihapus dari cart")
}

func (h *CartHandler) ClearInternalItems(c *gin.Context) {
	if h.cartService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Cart service unavailable", nil)
		return
	}

	if h.internalAPIKey == "" || c.GetHeader("X-Internal-Api-Key") != h.internalAPIKey {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	var req ClearInternalCartItemsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}

	if validationErrors := validateClearInternalCartItemsRequest(req); len(validationErrors) > 0 {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", validationErrors)
		return
	}

	if err := h.cartService.ClearItemsByIDs(c.Request.Context(), req.ItemIDs); err != nil {
		writeCartServiceError(c, err)
		return
	}

	dto.WriteSuccess(c, http.StatusOK, nil, "Cart items cleared")
}

func validateAddCartItemRequest(req AddCartItemRequest) map[string]string {
	validationErrors := make(map[string]string)
	if !isValidUUID(req.ProductID) {
		validationErrors["product_id"] = "ID produk harus berupa UUID"
	}
	if req.Quantity < 1 {
		validationErrors["quantity"] = "Quantity minimal 1"
	}
	return validationErrors
}

func validateClearInternalCartItemsRequest(req ClearInternalCartItemsRequest) map[string]string {
	validationErrors := make(map[string]string)
	if len(req.ItemIDs) == 0 {
		validationErrors["item_ids"] = "Item IDs wajib diisi"
		return validationErrors
	}
	for _, itemID := range req.ItemIDs {
		if !isValidUUID(strings.TrimSpace(itemID)) {
			validationErrors["item_ids"] = "Semua item_id harus berupa UUID"
			return validationErrors
		}
	}
	return validationErrors
}

func writeCartServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidCartProductID), errors.Is(err, service.ErrInvalidCartItemID), errors.Is(err, service.ErrInvalidCartQuantity), errors.Is(err, service.ErrInvalidCartUserID):
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", nil)
	case errors.Is(err, service.ErrCartProductNotFound):
		dto.WriteError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Produk tidak ditemukan", nil)
	case errors.Is(err, service.ErrCartItemNotFound):
		dto.WriteError(c, http.StatusNotFound, "CART_ITEM_NOT_FOUND", "Item cart tidak ditemukan", nil)
	case errors.Is(err, service.ErrCartProductUnavailable):
		dto.WriteError(c, http.StatusUnprocessableEntity, "PRODUCT_UNAVAILABLE", "Produk tidak tersedia", nil)
	case errors.Is(err, service.ErrCartProductOutOfStock):
		dto.WriteError(c, http.StatusUnprocessableEntity, "PRODUCT_OUT_OF_STOCK", "Produk habis", nil)
	default:
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
	}
}

func cartResponse(cart *service.Cart) CartResponse {
	if cart == nil {
		return CartResponse{Items: []CartItemResponse{}}
	}
	items := make([]CartItemResponse, 0, len(cart.Items))
	for _, item := range cart.Items {
		items = append(items, CartItemResponse{
			ItemID:      item.ItemID,
			ProductID:   item.ProductID,
			Name:        item.Name,
			ImageURL:    item.ImageURL,
			Price:       item.Price,
			Quantity:    item.Quantity,
			Subtotal:    item.Subtotal,
			IsAvailable: item.IsAvailable,
		})
	}
	return CartResponse{
		CartID:     cart.ID,
		UserID:     cart.UserID,
		Items:      items,
		GrandTotal: cart.GrandTotal,
		UpdatedAt:  cart.UpdatedAt,
	}
}
