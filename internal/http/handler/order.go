package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cafeTelkom/internal/http/dto"
	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
)

type OrderHandler struct {
	orderService orderManager
}

type orderManager interface {
	Checkout(ctx context.Context, input service.CheckoutInput) (*service.Order, error)
	ListOrders(ctx context.Context, input service.ListOrdersInput) (*service.OrderList, error)
	GetOrder(ctx context.Context, input service.GetOrderInput) (*service.Order, error)
}

type CheckoutOrderRequest struct {
	Notes *string                    `json:"notes"`
	Items []CheckoutOrderItemRequest `json:"items"`
}

type CheckoutOrderItemRequest struct {
	CartItemID string            `json:"cart_item_id"`
	Attributes map[string]string `json:"attributes"`
}

type OrderDetailResponse struct {
	OrderID     string              `json:"order_id"`
	OrderNumber string              `json:"order_number"`
	UserID      string              `json:"user_id"`
	Status      string              `json:"status"`
	Notes       *string             `json:"notes"`
	TotalAmount int32               `json:"total_amount"`
	ExpiresAt   *time.Time          `json:"expires_at"`
	Items       []OrderItemResponse `json:"items"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
}

type OrderItemResponse struct {
	OrderItemID        string          `json:"order_item_id"`
	ProductID          string          `json:"product_id"`
	ProductName        string          `json:"product_name"`
	PriceAtCheckout    int32           `json:"price_at_checkout"`
	Quantity           int32           `json:"quantity"`
	Subtotal           int32           `json:"subtotal"`
	SelectedAttributes json.RawMessage `json:"selected_attributes"`
}

type OrderSummaryResponse struct {
	OrderID     string    `json:"order_id"`
	OrderNumber string    `json:"order_number"`
	Status      string    `json:"status"`
	TotalAmount int32     `json:"total_amount"`
	CreatedAt   time.Time `json:"created_at"`
}

type orderListEnvelope struct {
	Success    bool                   `json:"success"`
	Data       []OrderSummaryResponse `json:"data"`
	Pagination orderPagination        `json:"pagination"`
}

type orderPagination struct {
	NextCursor *string `json:"next_cursor"`
	PrevCursor *string `json:"prev_cursor"`
	Limit      int32   `json:"limit"`
	HasNext    bool    `json:"has_next"`
	HasPrev    bool    `json:"has_prev"`
}

func NewOrderHandler(orderService orderManager) *OrderHandler {
	return &OrderHandler{orderService: orderService}
}

func (h *OrderHandler) Checkout(c *gin.Context) {
	if h.orderService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Order service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	var req CheckoutOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}
	req = normalizeCheckoutOrderRequest(req)
	if validationErrors := validateCheckoutOrderRequest(req); len(validationErrors) > 0 {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", validationErrors)
		return
	}

	items := make([]service.CheckoutItemInput, 0, len(req.Items))
	for _, item := range req.Items {
		items = append(items, service.CheckoutItemInput{
			CartItemID: item.CartItemID,
			Attributes: item.Attributes,
		})
	}

	order, err := h.orderService.Checkout(c.Request.Context(), service.CheckoutInput{
		UserID:      user.ID,
		IsVerified:  user.IsVerified,
		PhoneNumber: user.PhoneNumber,
		Notes:       req.Notes,
		Items:       items,
	})
	if err != nil {
		writeOrderServiceError(c, err)
		return
	}

	dto.WriteSuccess(c, http.StatusCreated, orderDetailResponse(order), "Order berhasil dibuat")
}

func (h *OrderHandler) ListOrders(c *gin.Context) {
	if h.orderService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Order service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	limit, ok := parseOrderLimit(c.Query("limit"))
	if !ok {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"limit": "Limit harus berupa angka",
		})
		return
	}
	direction := strings.TrimSpace(c.Query("direction"))
	if direction != "" && direction != "next" && direction != "prev" {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"direction": "Direction harus next atau prev",
		})
		return
	}
	status := strings.TrimSpace(c.Query("status"))
	if status != "" && !isAllowedValue(status, []string{"PENDING", "CONFIRMED", "COMPLETED", "CANCELLED"}) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"status": "Status tidak valid",
		})
		return
	}
	userID := strings.TrimSpace(c.Query("user_id"))
	if string(user.Role) == "ADMIN" && userID != "" && !isValidUUID(userID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"user_id": "ID user harus berupa UUID",
		})
		return
	}

	list, err := h.orderService.ListOrders(c.Request.Context(), service.ListOrdersInput{
		ActorUserID: user.ID,
		ActorRole:   string(user.Role),
		Cursor:      strings.TrimSpace(c.Query("cursor")),
		Direction:   direction,
		Limit:       limit,
		Status:      status,
		UserID:      userID,
	})
	if err != nil {
		writeOrderServiceError(c, err)
		return
	}
	if list == nil {
		list = &service.OrderList{Limit: limit}
	}

	c.JSON(http.StatusOK, orderListEnvelope{
		Success: true,
		Data:    orderSummaryResponses(list.Items),
		Pagination: orderPagination{
			NextCursor: list.NextCursor,
			PrevCursor: list.PrevCursor,
			Limit:      list.Limit,
			HasNext:    list.HasNext,
			HasPrev:    list.HasPrev,
		},
	})
}

func (h *OrderHandler) GetOrder(c *gin.Context) {
	if h.orderService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Order service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	orderID := strings.TrimSpace(c.Param("order_id"))
	if !isValidUUID(orderID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"order_id": "ID order harus berupa UUID",
		})
		return
	}

	order, err := h.orderService.GetOrder(c.Request.Context(), service.GetOrderInput{
		OrderID:     orderID,
		ActorUserID: user.ID,
		ActorRole:   string(user.Role),
	})
	if err != nil {
		writeOrderServiceError(c, err)
		return
	}

	dto.WriteSuccess(c, http.StatusOK, orderDetailResponse(order), "Order berhasil diambil")
}

func normalizeCheckoutOrderRequest(req CheckoutOrderRequest) CheckoutOrderRequest {
	if req.Notes != nil {
		notes := strings.TrimSpace(*req.Notes)
		req.Notes = &notes
	}
	for i := range req.Items {
		req.Items[i].CartItemID = strings.TrimSpace(req.Items[i].CartItemID)
		if req.Items[i].Attributes == nil {
			req.Items[i].Attributes = map[string]string{}
		}
		for key, value := range req.Items[i].Attributes {
			req.Items[i].Attributes[key] = strings.TrimSpace(value)
		}
	}
	return req
}

func validateCheckoutOrderRequest(req CheckoutOrderRequest) map[string]string {
	validationErrors := make(map[string]string)
	if req.Notes != nil && len(*req.Notes) > 255 {
		validationErrors["notes"] = "Notes maksimal 255 karakter"
	}
	if len(req.Items) == 0 {
		validationErrors["items"] = "Items wajib diisi"
		return validationErrors
	}
	for i, item := range req.Items {
		if !isValidUUID(item.CartItemID) {
			validationErrors["items."+strconv.Itoa(i)+".cart_item_id"] = "ID item cart harus berupa UUID"
		}
	}
	return validationErrors
}

func parseOrderLimit(raw string) (int32, bool) {
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

func writeOrderServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidOrderUserID), errors.Is(err, service.ErrInvalidOrderID), errors.Is(err, service.ErrInvalidOrderCartItemID), errors.Is(err, service.ErrOrderValidation), errors.Is(err, service.ErrOrderInvalidStatus):
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", nil)
	case errors.Is(err, service.ErrOrderInvalidCursor):
		dto.WriteError(c, http.StatusBadRequest, "INVALID_CURSOR", "Cursor tidak valid", nil)
	case errors.Is(err, service.ErrOrderForbidden):
		dto.WriteError(c, http.StatusForbidden, "FORBIDDEN", "Role tidak diizinkan", nil)
	case errors.Is(err, service.ErrOrderEmailUnverified):
		dto.WriteError(c, http.StatusForbidden, "EMAIL_UNVERIFIED", "Email belum terverifikasi", nil)
	case errors.Is(err, service.ErrOrderPhoneNumberRequired):
		dto.WriteError(c, http.StatusForbidden, "PHONE_NUMBER_REQUIRED", "Nomor telepon wajib diisi", nil)
	case errors.Is(err, service.ErrOrderCartItemNotFound):
		dto.WriteError(c, http.StatusNotFound, "CART_ITEM_NOT_FOUND", "Item cart tidak ditemukan", nil)
	case errors.Is(err, service.ErrOrderCartItemAlreadyPending):
		dto.WriteError(c, http.StatusUnprocessableEntity, "CART_ITEM_ALREADY_PENDING", "Item cart masih menunggu pembayaran", nil)
	case errors.Is(err, service.ErrOrderProductNotFound):
		dto.WriteError(c, http.StatusNotFound, "PRODUCT_NOT_FOUND", "Produk tidak ditemukan", nil)
	case errors.Is(err, service.ErrOrderNotFound):
		dto.WriteError(c, http.StatusNotFound, "ORDER_NOT_FOUND", "Order tidak ditemukan", nil)
	case errors.Is(err, service.ErrOrderProductUnavailable):
		dto.WriteError(c, http.StatusUnprocessableEntity, "PRODUCT_UNAVAILABLE", "Produk tidak tersedia", nil)
	case errors.Is(err, service.ErrOrderProductOutOfStock):
		dto.WriteError(c, http.StatusUnprocessableEntity, "PRODUCT_OUT_OF_STOCK", "Produk habis", nil)
	case errors.Is(err, service.ErrOrderInsufficientStock):
		dto.WriteError(c, http.StatusUnprocessableEntity, "INSUFFICIENT_STOCK", "Stok produk tidak mencukupi", nil)
	default:
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
	}
}

func orderDetailResponse(order *service.Order) OrderDetailResponse {
	if order == nil {
		return OrderDetailResponse{Items: []OrderItemResponse{}}
	}
	return OrderDetailResponse{
		OrderID:     order.ID,
		OrderNumber: order.OrderNumber,
		UserID:      order.UserID,
		Status:      order.Status,
		Notes:       order.Notes,
		TotalAmount: order.TotalAmount,
		ExpiresAt:   order.ExpiresAt,
		Items:       orderItemResponses(order.Items),
		CreatedAt:   order.CreatedAt,
		UpdatedAt:   order.UpdatedAt,
	}
}

func orderItemResponses(items []service.OrderItem) []OrderItemResponse {
	responses := make([]OrderItemResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, OrderItemResponse{
			OrderItemID:        item.ID,
			ProductID:          item.ProductID,
			ProductName:        item.ProductName,
			PriceAtCheckout:    item.PriceAtCheckout,
			Quantity:           item.Quantity,
			Subtotal:           item.Subtotal,
			SelectedAttributes: jsonAttributes(item.SelectedAttributes),
		})
	}
	return responses
}

func orderSummaryResponses(items []service.OrderSummary) []OrderSummaryResponse {
	responses := make([]OrderSummaryResponse, 0, len(items))
	for _, item := range items {
		responses = append(responses, OrderSummaryResponse{
			OrderID:     item.ID,
			OrderNumber: item.OrderNumber,
			Status:      item.Status,
			TotalAmount: item.TotalAmount,
			CreatedAt:   item.CreatedAt,
		})
	}
	return responses
}
