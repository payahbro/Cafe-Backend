package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	cafedb "cafeTelkom/internal/db"
	"cafeTelkom/internal/integrations/midtrans"
	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidPaymentOrderID  = errors.New("invalid payment order id")
	ErrInvalidPaymentID       = errors.New("invalid payment id")
	ErrPaymentForbidden       = errors.New("payment forbidden")
	ErrPaymentEmailUnverified = errors.New("payment email unverified")
	ErrPaymentPhoneRequired   = errors.New("payment phone number required")
	ErrPaymentOrderNotFound   = errors.New("payment order not found")
	ErrPaymentOrderNotPayable = errors.New("payment order not payable")
	ErrPaymentOrderExpired    = errors.New("payment order expired")
	ErrPaymentGateway         = errors.New("payment gateway error")
	ErrPaymentNotFound        = errors.New("payment not found")
	ErrPaymentWebhookInvalid  = errors.New("payment webhook invalid")
	ErrPaymentOrderSyncFailed = errors.New("payment order sync failed")
)

type paymentRepository interface {
	GetOrderByID(ctx context.Context, id pgtype.UUID) (repository.Order, error)
	ListOrderItemsByOrderID(ctx context.Context, orderID pgtype.UUID) ([]repository.OrderItem, error)
	GetActivePaymentByOrderID(ctx context.Context, orderID pgtype.UUID) (repository.Payment, error)
	CreatePayment(ctx context.Context, arg repository.CreatePaymentParams) (repository.Payment, error)
	GetPaymentByID(ctx context.Context, id pgtype.UUID) (repository.Payment, error)
	UpdatePaymentAfterWebhook(ctx context.Context, arg repository.UpdatePaymentAfterWebhookParams) (repository.Payment, error)
	ListPaymentsByOrderID(ctx context.Context, orderID pgtype.UUID) ([]repository.Payment, error)
}

type paymentTxRunner interface {
	Run(ctx context.Context, fn func(paymentRepository) error) error
}

type PaymentTxRunner struct {
	db   cafedb.TxBeginner
	repo *repository.Queries
}

type snapTransactionCreator interface {
	CreateTransaction(ctx context.Context, request SnapTransactionRequest) (SnapTransaction, error)
}

type paymentOrderConfirmer interface {
	ConfirmOrderFromPayment(ctx context.Context, input InternalConfirmOrderInput) (*InternalConfirmOrderResult, error)
}

type paymentCartClearer interface {
	ClearItemsByIDs(ctx context.Context, itemIDs []string) error
}

type PaymentService struct {
	repo           paymentRepository
	txRunner       paymentTxRunner
	snapClient     snapTransactionCreator
	orderConfirmer paymentOrderConfirmer
	cartClearer    paymentCartClearer
	serverKey      string
	webhookURL     string
	now            func() time.Time
	newUUID        func() (string, error)
}

type PaymentServiceOptions struct {
	ServerKey  string
	WebhookURL string
	Now        func() time.Time
	NewUUID    func() (string, error)
}

type InitiatePaymentInput struct {
	OrderID     string
	UserID      string
	UserRole    string
	IsVerified  bool
	FullName    string
	Email       string
	PhoneNumber string
}

type PaymentInitiation struct {
	PaymentID       string
	OrderID         string
	SnapRedirectURL string
	ExpiresAt       *time.Time
}

type WebhookInput struct {
	OrderID           string `json:"order_id"`
	StatusCode        string `json:"status_code"`
	GrossAmount       string `json:"gross_amount"`
	SignatureKey      string `json:"signature_key"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	TransactionID     string `json:"transaction_id"`
	PaymentType       string `json:"payment_type"`
}

type WebhookResult struct {
	Processed bool
	Status    string
}

type GetPaymentsByOrderInput struct {
	OrderID     string
	ActorUserID string
	ActorRole   string
}

type PaymentsByOrder struct {
	OrderID  string
	Payments []Payment
}

type Payment struct {
	PaymentID             string
	OrderID               string
	OrderNumber           string
	Status                string
	Amount                int32
	PaymentMethod         *string
	MidtransTransactionID *string
	SnapRedirectURL       *string
	RefundAmount          *int32
	RefundReason          *string
	RefundedAt            *time.Time
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type SnapTransactionRequest struct {
	OrderID         string
	GrossAmount     int32
	Customer        SnapCustomer
	Items           []SnapItem
	NotificationURL string
}

type SnapCustomer struct {
	FirstName string
	Email     string
	Phone     string
}

type SnapItem struct {
	ID       string
	Name     string
	Price    int32
	Quantity int32
}

type SnapTransaction struct {
	Token       string
	RedirectURL string
}

type MidtransSnapClient struct {
	client *midtrans.Client
}

func NewMidtransSnapClient(client *midtrans.Client) *MidtransSnapClient {
	if client == nil {
		return nil
	}
	return &MidtransSnapClient{client: client}
}

func (c *MidtransSnapClient) CreateTransaction(ctx context.Context, request SnapTransactionRequest) (SnapTransaction, error) {
	if c == nil || c.client == nil {
		return SnapTransaction{}, ErrPaymentGateway
	}
	items := make([]midtrans.ItemDetails, 0, len(request.Items))
	for _, item := range request.Items {
		items = append(items, midtrans.ItemDetails{
			ID:       item.ID,
			Name:     item.Name,
			Price:    item.Price,
			Quantity: item.Quantity,
		})
	}
	response, err := c.client.CreateTransaction(ctx, midtrans.TransactionRequest{
		OrderID:     request.OrderID,
		GrossAmount: request.GrossAmount,
		Customer: midtrans.CustomerDetails{
			FirstName: request.Customer.FirstName,
			Email:     request.Customer.Email,
			Phone:     request.Customer.Phone,
		},
		Items: items,
	})
	if err != nil {
		return SnapTransaction{}, err
	}
	return SnapTransaction{Token: response.Token, RedirectURL: response.RedirectURL}, nil
}

func NewPaymentService(repo paymentRepository, txRunner paymentTxRunner, snapClient snapTransactionCreator, orderConfirmer paymentOrderConfirmer, cartClearer paymentCartClearer, options PaymentServiceOptions) *PaymentService {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	newUUID := options.NewUUID
	if newUUID == nil {
		newUUID = generateUUID
	}
	return &PaymentService{
		repo:           repo,
		txRunner:       txRunner,
		snapClient:     snapClient,
		orderConfirmer: orderConfirmer,
		cartClearer:    cartClearer,
		serverKey:      options.ServerKey,
		webhookURL:     strings.TrimSpace(options.WebhookURL),
		now:            now,
		newUUID:        newUUID,
	}
}

func NewPaymentTxRunner(db cafedb.TxBeginner, repo *repository.Queries) *PaymentTxRunner {
	if db == nil || repo == nil {
		return nil
	}
	return &PaymentTxRunner{db: db, repo: repo}
}

func (r *PaymentTxRunner) Run(ctx context.Context, fn func(paymentRepository) error) error {
	if r == nil || r.db == nil || r.repo == nil {
		return errors.New("payment transaction runner missing")
	}

	return cafedb.WithTx(ctx, r.db, func(ctx context.Context, tx pgx.Tx) error {
		return fn(r.repo.WithTx(tx))
	})
}

func (s *PaymentService) InitiatePayment(ctx context.Context, input InitiatePaymentInput) (*PaymentInitiation, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if repository.UserRole(input.UserRole) != repository.UserRoleCUSTOMER {
		return nil, ErrPaymentForbidden
	}
	if !input.IsVerified {
		return nil, ErrPaymentEmailUnverified
	}
	if strings.TrimSpace(input.PhoneNumber) == "" {
		return nil, ErrPaymentPhoneRequired
	}

	orderUUID, err := parseRequiredUUID(strings.TrimSpace(input.OrderID))
	if err != nil {
		return nil, ErrInvalidPaymentOrderID
	}
	userUUID, err := parseRequiredUUID(strings.TrimSpace(input.UserID))
	if err != nil {
		return nil, ErrPaymentForbidden
	}

	order, err := s.repo.GetOrderByID(ctx, orderUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentOrderNotFound
		}
		return nil, fmt.Errorf("get order: %w", err)
	}
	if order.UserID.String() != userUUID.String() {
		return nil, ErrPaymentOrderNotFound
	}
	if order.Status != repository.OrderStatusPENDING {
		return nil, ErrPaymentOrderNotPayable
	}
	if order.ExpiresAt.Valid && !s.now().Before(order.ExpiresAt.Time) {
		return nil, ErrPaymentOrderExpired
	}

	activePayment, err := s.repo.GetActivePaymentByOrderID(ctx, order.ID)
	if err == nil {
		return &PaymentInitiation{
			PaymentID:       activePayment.ID.String(),
			OrderID:         order.ID.String(),
			SnapRedirectURL: textOrEmpty(activePayment.SnapRedirectUrl),
			ExpiresAt:       timestamptzPtr(order.ExpiresAt),
		}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("get active payment: %w", err)
	}
	if s.snapClient == nil {
		return nil, ErrPaymentGateway
	}

	items, err := s.repo.ListOrderItemsByOrderID(ctx, order.ID)
	if err != nil {
		return nil, fmt.Errorf("list order items: %w", err)
	}

	paymentID, err := s.newUUID()
	if err != nil {
		return nil, fmt.Errorf("generate payment id: %w", err)
	}
	paymentUUID, err := parseRequiredUUID(paymentID)
	if err != nil {
		return nil, fmt.Errorf("generated payment id invalid: %w", err)
	}
	midtransOrderID := fmt.Sprintf("P-%s-%d", paymentID, s.now().Unix())

	snapRequest := SnapTransactionRequest{
		OrderID:     midtransOrderID,
		GrossAmount: order.TotalAmount,
		Customer: SnapCustomer{
			FirstName: strings.TrimSpace(input.FullName),
			Email:     strings.TrimSpace(input.Email),
			Phone:     strings.TrimSpace(input.PhoneNumber),
		},
		Items:           snapItemsFromOrderItems(items),
		NotificationURL: s.webhookURL,
	}
	snap, err := s.snapClient.CreateTransaction(ctx, snapRequest)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrPaymentGateway, err)
	}
	if strings.TrimSpace(snap.RedirectURL) == "" {
		return nil, ErrPaymentGateway
	}

	create := func(repo paymentRepository) error {
		created, err := repo.CreatePayment(ctx, repository.CreatePaymentParams{
			ID:              paymentUUID,
			OrderID:         order.ID,
			Status:          repository.PaymentStatusPENDINGPAYMENT,
			Amount:          order.TotalAmount,
			MidtransOrderID: midtransOrderID,
			SnapRedirectUrl: pgtype.Text{String: snap.RedirectURL, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("create payment: %w", err)
		}
		activePayment = created
		return nil
	}
	if s.txRunner != nil {
		if err := s.txRunner.Run(ctx, create); err != nil {
			return nil, err
		}
	} else if err := create(s.repo); err != nil {
		return nil, err
	}

	return &PaymentInitiation{
		PaymentID:       activePayment.ID.String(),
		OrderID:         order.ID.String(),
		SnapRedirectURL: textOrEmpty(activePayment.SnapRedirectUrl),
		ExpiresAt:       timestamptzPtr(order.ExpiresAt),
	}, nil
}

func (s *PaymentService) HandleWebhook(ctx context.Context, input WebhookInput) (*WebhookResult, error) {
	if strings.TrimSpace(s.serverKey) == "" {
		return &WebhookResult{}, nil
	}
	expected := ComputeMidtransSignature(input.OrderID, input.StatusCode, input.GrossAmount, s.serverKey)
	if !strings.EqualFold(expected, strings.TrimSpace(input.SignatureKey)) {
		return &WebhookResult{}, nil
	}
	paymentID, err := parsePaymentIDFromMidtransOrderID(input.OrderID)
	if err != nil {
		return &WebhookResult{}, nil
	}
	paymentUUID, err := parseRequiredUUID(paymentID)
	if err != nil {
		return &WebhookResult{}, nil
	}
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	payment, err := s.repo.GetPaymentByID(ctx, paymentUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &WebhookResult{}, nil
		}
		return nil, fmt.Errorf("get payment: %w", err)
	}
	if isFinalPaymentStatus(payment.Status) {
		return &WebhookResult{Processed: true, Status: string(payment.Status)}, nil
	}

	status, ok := mapMidtransStatus(input.TransactionStatus, input.FraudStatus)
	if !ok {
		return &WebhookResult{}, nil
	}

	updated, err := s.repo.UpdatePaymentAfterWebhook(ctx, repository.UpdatePaymentAfterWebhookParams{
		ID:                    payment.ID,
		Status:                status,
		PaymentMethod:         nullableText(input.PaymentType),
		MidtransTransactionID: nullableText(input.TransactionID),
	})
	if err != nil {
		return nil, fmt.Errorf("update payment: %w", err)
	}

	if status == repository.PaymentStatusSUCCESS {
		if err := s.confirmOrderFromPayment(ctx, updated.OrderID.String()); err != nil {
			return &WebhookResult{Processed: true, Status: string(status)}, err
		}
	}

	return &WebhookResult{Processed: true, Status: string(status)}, nil
}

func (s *PaymentService) GetPaymentsByOrder(ctx context.Context, input GetPaymentsByOrderInput) (*PaymentsByOrder, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	orderUUID, err := parseRequiredUUID(strings.TrimSpace(input.OrderID))
	if err != nil {
		return nil, ErrInvalidPaymentOrderID
	}
	actorUUID, err := parseRequiredUUID(strings.TrimSpace(input.ActorUserID))
	if err != nil {
		return nil, ErrPaymentForbidden
	}

	order, err := s.repo.GetOrderByID(ctx, orderUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrPaymentOrderNotFound
		}
		return nil, fmt.Errorf("get order: %w", err)
	}
	customerView := false
	switch repository.UserRole(input.ActorRole) {
	case repository.UserRoleCUSTOMER:
		customerView = true
		if order.UserID.String() != actorUUID.String() {
			return nil, ErrPaymentOrderNotFound
		}
	case repository.UserRoleADMIN:
	default:
		return nil, ErrPaymentForbidden
	}

	rows, err := s.repo.ListPaymentsByOrderID(ctx, order.ID)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	if len(rows) == 0 {
		return nil, ErrPaymentNotFound
	}
	if customerView {
		rows = rows[:1]
	}

	payments := make([]Payment, 0, len(rows))
	for _, row := range rows {
		payments = append(payments, paymentFromRow(row, order))
	}
	return &PaymentsByOrder{OrderID: order.ID.String(), Payments: payments}, nil
}

func (s *PaymentService) confirmOrderFromPayment(ctx context.Context, orderID string) error {
	if s.orderConfirmer == nil {
		return ErrPaymentOrderSyncFailed
	}
	result, err := s.orderConfirmer.ConfirmOrderFromPayment(ctx, InternalConfirmOrderInput{OrderID: orderID})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPaymentOrderSyncFailed, err)
	}
	if result != nil && len(result.CartItemIDs) > 0 {
		if s.cartClearer == nil {
			return ErrPaymentOrderSyncFailed
		}
		if err := s.cartClearer.ClearItemsByIDs(ctx, result.CartItemIDs); err != nil {
			return fmt.Errorf("%w: %v", ErrPaymentOrderSyncFailed, err)
		}
	}
	return nil
}

func snapItemsFromOrderItems(items []repository.OrderItem) []SnapItem {
	result := make([]SnapItem, 0, len(items))
	for _, item := range items {
		result = append(result, SnapItem{
			ID:       item.ID.String(),
			Name:     item.ProductName,
			Price:    item.PriceAtCheckout,
			Quantity: item.Quantity,
		})
	}
	return result
}

func mapMidtransStatus(transactionStatus, fraudStatus string) (repository.PaymentStatus, bool) {
	switch strings.ToLower(strings.TrimSpace(transactionStatus)) {
	case "settlement":
		return repository.PaymentStatusSUCCESS, true
	case "capture":
		fraud := strings.ToLower(strings.TrimSpace(fraudStatus))
		if fraud == "" || fraud == "accept" || fraud == "challenge" {
			return repository.PaymentStatusSUCCESS, true
		}
		return repository.PaymentStatusFAILED, true
	case "pending":
		return repository.PaymentStatusPENDINGPAYMENT, true
	case "deny", "cancel", "failure":
		return repository.PaymentStatusFAILED, true
	case "expire":
		return repository.PaymentStatusEXPIRED, true
	case "refund":
		return repository.PaymentStatusREFUNDED, true
	default:
		return "", false
	}
}

func isFinalPaymentStatus(status repository.PaymentStatus) bool {
	switch status {
	case repository.PaymentStatusSUCCESS, repository.PaymentStatusFAILED, repository.PaymentStatusEXPIRED, repository.PaymentStatusREFUNDED:
		return true
	default:
		return false
	}
}

func parsePaymentIDFromMidtransOrderID(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	prefix := "P-"
	if strings.HasPrefix(trimmed, "PAY-") {
		prefix = "PAY-"
	}
	if !strings.HasPrefix(trimmed, prefix) {
		return "", ErrPaymentWebhookInvalid
	}
	rest := strings.TrimPrefix(trimmed, prefix)
	lastDash := strings.LastIndex(rest, "-")
	if lastDash <= 0 || lastDash == len(rest)-1 {
		return "", ErrPaymentWebhookInvalid
	}
	return rest[:lastDash], nil
}

func ComputeMidtransSignature(orderID, statusCode, grossAmount, serverKey string) string {
	return midtrans.ComputeSignature(orderID, statusCode, grossAmount, serverKey)
}

func nullableText(value string) pgtype.Text {
	value = strings.TrimSpace(value)
	if value == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: value, Valid: true}
}

func paymentFromRow(row repository.Payment, order repository.Order) Payment {
	return Payment{
		PaymentID:             row.ID.String(),
		OrderID:               row.OrderID.String(),
		OrderNumber:           order.OrderNumber,
		Status:                string(row.Status),
		Amount:                row.Amount,
		PaymentMethod:         textPtr(row.PaymentMethod),
		MidtransTransactionID: textPtr(row.MidtransTransactionID),
		SnapRedirectURL:       textPtr(row.SnapRedirectUrl),
		RefundAmount:          int32Ptr(row.RefundAmount),
		RefundReason:          textPtr(row.RefundReason),
		RefundedAt:            timestamptzPtr(row.RefundedAt),
		CreatedAt:             row.CreatedAt.Time,
		UpdatedAt:             row.UpdatedAt.Time,
	}
}

func int32Ptr(value pgtype.Int4) *int32 {
	if !value.Valid {
		return nil
	}
	result := value.Int32
	return &result
}

func generateUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	encoded := make([]byte, 36)
	hex.Encode(encoded[0:8], b[0:4])
	encoded[8] = '-'
	hex.Encode(encoded[9:13], b[4:6])
	encoded[13] = '-'
	hex.Encode(encoded[14:18], b[6:8])
	encoded[18] = '-'
	hex.Encode(encoded[19:23], b[8:10])
	encoded[23] = '-'
	hex.Encode(encoded[24:36], b[10:16])
	return string(encoded), nil
}
