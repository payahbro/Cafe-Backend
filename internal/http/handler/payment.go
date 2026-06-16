package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"cafeTelkom/internal/http/dto"
	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/logger"
	"cafeTelkom/internal/repository"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type PaymentHandler struct {
	paymentService paymentManager
	log            *zap.Logger
}

type paymentManager interface {
	InitiatePayment(ctx context.Context, input service.InitiatePaymentInput) (*service.PaymentInitiation, error)
	HandleWebhook(ctx context.Context, input service.WebhookInput) (*service.WebhookResult, error)
	GetPaymentsByOrder(ctx context.Context, input service.GetPaymentsByOrderInput) (*service.PaymentsByOrder, error)
}

type InitiatePaymentRequest struct {
	OrderID string `json:"order_id"`
}

type InitiatePaymentResponse struct {
	PaymentID       string     `json:"payment_id"`
	OrderID         string     `json:"order_id"`
	SnapRedirectURL string     `json:"snap_redirect_url"`
	ExpiresAt       *time.Time `json:"expires_at"`
}

type PaymentResponse struct {
	PaymentID             string     `json:"payment_id"`
	OrderID               string     `json:"order_id"`
	OrderNumber           string     `json:"order_number"`
	Status                string     `json:"status"`
	Amount                int32      `json:"amount"`
	PaymentMethod         *string    `json:"payment_method"`
	MidtransTransactionID *string    `json:"midtrans_transaction_id"`
	SnapRedirectURL       *string    `json:"snap_redirect_url"`
	RefundAmount          *int32     `json:"refund_amount"`
	RefundReason          *string    `json:"refund_reason"`
	RefundedAt            *time.Time `json:"refunded_at"`
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

type PaymentHandlerOption func(*PaymentHandler)

func WithPaymentLogger(log *zap.Logger) PaymentHandlerOption {
	return func(h *PaymentHandler) {
		h.log = log
	}
}

func NewPaymentHandler(paymentService paymentManager, options ...PaymentHandlerOption) *PaymentHandler {
	handler := &PaymentHandler{paymentService: paymentService}
	for _, option := range options {
		option(handler)
	}
	return handler
}

func (h *PaymentHandler) Initiate(c *gin.Context) {
	if h.paymentService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Payment service unavailable", nil)
		return
	}

	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	var req InitiatePaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}
	req.OrderID = strings.TrimSpace(req.OrderID)
	if !isValidUUID(req.OrderID) {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"order_id": "ID order harus berupa UUID",
		})
		return
	}

	result, err := h.paymentService.InitiatePayment(c.Request.Context(), service.InitiatePaymentInput{
		OrderID:     req.OrderID,
		UserID:      user.ID,
		UserRole:    string(user.Role),
		IsVerified:  user.IsVerified,
		FullName:    user.FullName,
		Email:       user.Email,
		PhoneNumber: user.PhoneNumber,
	})
	if err != nil {
		h.writePaymentServiceError(c, err)
		return
	}

	dto.WriteSuccess(c, http.StatusOK, initiatePaymentResponse(result), "Payment berhasil dibuat, silakan lanjutkan ke halaman pembayaran")
}

func (h *PaymentHandler) Webhook(c *gin.Context) {
	var payload service.WebhookInput
	if err := c.ShouldBindJSON(&payload); err == nil && h.paymentService != nil {
		result, handleErr := h.paymentService.HandleWebhook(c.Request.Context(), payload)
		h.logWebhookResult(payload, result, handleErr)
	}

	dto.WriteSuccess(c, http.StatusOK, nil, "OK")
}

func (h *PaymentHandler) GetByOrder(c *gin.Context) {
	if h.paymentService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Payment service unavailable", nil)
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

	result, err := h.paymentService.GetPaymentsByOrder(c.Request.Context(), service.GetPaymentsByOrderInput{
		OrderID:     orderID,
		ActorUserID: user.ID,
		ActorRole:   string(user.Role),
	})
	if err != nil {
		h.writePaymentServiceError(c, err)
		return
	}

	responses := paymentResponses(result.Payments)
	if user.Role == repository.UserRoleADMIN {
		dto.WriteSuccess(c, http.StatusOK, responses, "Payment berhasil diambil")
		return
	}
	if len(responses) == 0 {
		dto.WriteError(c, http.StatusNotFound, "PAYMENT_NOT_FOUND", "Payment tidak ditemukan", nil)
		return
	}
	dto.WriteSuccess(c, http.StatusOK, responses[0], "Payment berhasil diambil")
}

func (h *PaymentHandler) writePaymentServiceError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrInvalidPaymentOrderID), errors.Is(err, service.ErrInvalidPaymentID), errors.Is(err, service.ErrPaymentWebhookInvalid):
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", nil)
	case errors.Is(err, service.ErrPaymentForbidden):
		dto.WriteError(c, http.StatusForbidden, "FORBIDDEN", "Role tidak diizinkan", nil)
	case errors.Is(err, service.ErrPaymentEmailUnverified):
		dto.WriteError(c, http.StatusForbidden, "EMAIL_UNVERIFIED", "Email belum terverifikasi", nil)
	case errors.Is(err, service.ErrPaymentPhoneRequired):
		dto.WriteError(c, http.StatusForbidden, "PHONE_NUMBER_REQUIRED", "Nomor telepon wajib diisi", nil)
	case errors.Is(err, service.ErrPaymentOrderNotFound):
		dto.WriteError(c, http.StatusNotFound, "ORDER_NOT_FOUND", "Order tidak ditemukan", nil)
	case errors.Is(err, service.ErrPaymentNotFound):
		dto.WriteError(c, http.StatusNotFound, "PAYMENT_NOT_FOUND", "Payment tidak ditemukan", nil)
	case errors.Is(err, service.ErrPaymentOrderNotPayable):
		dto.WriteError(c, http.StatusUnprocessableEntity, "ORDER_NOT_PAYABLE", "Order tidak bisa dibayar", nil)
	case errors.Is(err, service.ErrPaymentOrderExpired):
		dto.WriteError(c, http.StatusUnprocessableEntity, "ORDER_EXPIRED", "Order sudah expired", nil)
	case errors.Is(err, service.ErrPaymentGateway):
		h.logPaymentError("payment gateway error", err)
		dto.WriteError(c, http.StatusBadGateway, "PAYMENT_GATEWAY_ERROR", "Payment gateway tidak tersedia", nil)
	default:
		h.logPaymentError("payment service error", err)
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
	}
}

func (h *PaymentHandler) logPaymentError(message string, err error) {
	if h != nil && h.log != nil {
		h.log.Error(message, logger.Error(err))
	}
}

func (h *PaymentHandler) logWebhookResult(payload service.WebhookInput, result *service.WebhookResult, err error) {
	if h == nil || h.log == nil {
		return
	}
	fields := []zap.Field{
		logger.String("midtrans_order_id", payload.OrderID),
		logger.String("transaction_status", payload.TransactionStatus),
		logger.String("fraud_status", payload.FraudStatus),
		logger.String("status_code", payload.StatusCode),
		logger.String("payment_type", payload.PaymentType),
	}
	if result != nil {
		fields = append(fields,
			zap.Bool("processed", result.Processed),
			logger.String("internal_status", result.Status),
		)
	}
	if err != nil {
		fields = append(fields, logger.Error(err))
		h.log.Error("payment webhook processed with error", fields...)
		return
	}
	h.log.Info("payment webhook processed", fields...)
}

func initiatePaymentResponse(result *service.PaymentInitiation) InitiatePaymentResponse {
	if result == nil {
		return InitiatePaymentResponse{}
	}
	return InitiatePaymentResponse{
		PaymentID:       result.PaymentID,
		OrderID:         result.OrderID,
		SnapRedirectURL: result.SnapRedirectURL,
		ExpiresAt:       result.ExpiresAt,
	}
}

func paymentResponses(payments []service.Payment) []PaymentResponse {
	responses := make([]PaymentResponse, 0, len(payments))
	for _, payment := range payments {
		responses = append(responses, PaymentResponse{
			PaymentID:             payment.PaymentID,
			OrderID:               payment.OrderID,
			OrderNumber:           payment.OrderNumber,
			Status:                payment.Status,
			Amount:                payment.Amount,
			PaymentMethod:         payment.PaymentMethod,
			MidtransTransactionID: payment.MidtransTransactionID,
			SnapRedirectURL:       payment.SnapRedirectURL,
			RefundAmount:          payment.RefundAmount,
			RefundReason:          payment.RefundReason,
			RefundedAt:            payment.RefundedAt,
			CreatedAt:             payment.CreatedAt,
			UpdatedAt:             payment.UpdatedAt,
		})
	}
	return responses
}
