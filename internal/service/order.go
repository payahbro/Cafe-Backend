package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	cafedb "cafeTelkom/internal/db"
	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrInvalidOrderUserID           = errors.New("invalid order user id")
	ErrInvalidOrderID               = errors.New("invalid order id")
	ErrInvalidOrderCartItemID       = errors.New("invalid order cart item id")
	ErrOrderValidation              = errors.New("order validation error")
	ErrOrderEmailUnverified         = errors.New("order email unverified")
	ErrOrderPhoneNumberRequired     = errors.New("order phone number required")
	ErrOrderForbidden               = errors.New("order forbidden")
	ErrOrderInvalidStatus           = errors.New("order invalid status")
	ErrOrderInvalidCursor           = errors.New("order invalid cursor")
	ErrOrderCartItemNotFound        = errors.New("order cart item not found")
	ErrOrderCartItemAlreadyPending  = errors.New("order cart item already pending")
	ErrOrderProductNotFound         = errors.New("order product not found")
	ErrOrderProductUnavailable      = errors.New("order product unavailable")
	ErrOrderProductOutOfStock       = errors.New("order product out of stock")
	ErrOrderInsufficientStock       = errors.New("order insufficient stock")
	ErrOrderNotFound                = errors.New("order not found")
	ErrOrderNotCancellable          = errors.New("order not cancellable")
	ErrOrderAlreadyCancelled        = errors.New("order already cancelled")
	ErrOrderInvalidStatusTransition = errors.New("order invalid status transition")
)

type orderRepository interface {
	ListCheckoutCartItemsForUser(ctx context.Context, arg repository.ListCheckoutCartItemsForUserParams) ([]repository.ListCheckoutCartItemsForUserRow, error)
	CountPendingOrderItemsByCartItemIDs(ctx context.Context, arg repository.CountPendingOrderItemsByCartItemIDsParams) (int64, error)
	AcquireOrderNumberDateLock(ctx context.Context, dateKey string) error
	CountOrdersByOrderNumberPrefix(ctx context.Context, prefix string) (int64, error)
	LockOrderByIDForUpdate(ctx context.Context, id pgtype.UUID) (repository.Order, error)
	LockProductByIDForUpdate(ctx context.Context, id pgtype.UUID) (repository.Product, error)
	DecrementProductStock(ctx context.Context, arg repository.DecrementProductStockParams) (repository.Product, error)
	IncrementProductStock(ctx context.Context, arg repository.IncrementProductStockParams) (repository.Product, error)
	IncrementProductTotalSold(ctx context.Context, arg repository.IncrementProductTotalSoldParams) (repository.Product, error)
	CreateOrder(ctx context.Context, arg repository.CreateOrderParams) (repository.Order, error)
	CreateOrderItem(ctx context.Context, arg repository.CreateOrderItemParams) (repository.OrderItem, error)
	ListExpiredPendingOrdersByCartItemIDs(ctx context.Context, arg repository.ListExpiredPendingOrdersByCartItemIDsParams) ([]repository.Order, error)
	ListOrders(ctx context.Context, arg repository.ListOrdersParams) ([]repository.ListOrdersRow, error)
	GetOrderByID(ctx context.Context, id pgtype.UUID) (repository.Order, error)
	ListOrderItemsByOrderID(ctx context.Context, orderID pgtype.UUID) ([]repository.OrderItem, error)
	UpdateOrderStatus(ctx context.Context, arg repository.UpdateOrderStatusParams) (repository.Order, error)
}

type orderTxRunner interface {
	Run(ctx context.Context, fn func(orderRepository) error) error
}

type OrderTxRunner struct {
	db   cafedb.TxBeginner
	repo *repository.Queries
}

type OrderService struct {
	repo     orderRepository
	txRunner orderTxRunner
	now      func() time.Time
}

type CheckoutInput struct {
	UserID      string
	IsVerified  bool
	PhoneNumber string
	Notes       *string
	Items       []CheckoutItemInput
}

type CheckoutItemInput struct {
	CartItemID string
	Attributes map[string]string
}

type ListOrdersInput struct {
	ActorUserID string
	ActorRole   string
	Cursor      string
	Direction   string
	Limit       int32
	Status      string
	UserID      string
}

type GetOrderInput struct {
	OrderID     string
	ActorUserID string
	ActorRole   string
}

type CancelOrderInput struct {
	OrderID     string
	ActorUserID string
	ActorRole   string
}

type UpdateOrderStatusInput struct {
	OrderID   string
	ActorRole string
	Status    string
}

type InternalConfirmOrderInput struct {
	OrderID string
}

type OrderStatusUpdate struct {
	OrderID   string
	Status    string
	UpdatedAt time.Time
}

type InternalConfirmOrderResult struct {
	OrderID     string
	Status      string
	CartItemIDs []string
}

type Order struct {
	ID          string
	OrderNumber string
	UserID      string
	Status      string
	Notes       *string
	TotalAmount int32
	ExpiresAt   *time.Time
	Items       []OrderItem
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

type OrderItem struct {
	ID                 string
	ProductID          string
	ProductName        string
	PriceAtCheckout    int32
	Quantity           int32
	Subtotal           int32
	SelectedAttributes []byte
}

type OrderSummary struct {
	ID          string
	OrderNumber string
	Status      string
	TotalAmount int32
	CreatedAt   time.Time
}

type OrderList struct {
	Items      []OrderSummary
	NextCursor *string
	PrevCursor *string
	Limit      int32
	HasNext    bool
	HasPrev    bool
}

type checkoutPreparedItem struct {
	row      repository.ListCheckoutCartItemsForUserRow
	attrs    []byte
	subtotal int32
}

func NewOrderService(repo orderRepository, txRunner orderTxRunner, now func() time.Time) *OrderService {
	if now == nil {
		now = time.Now
	}
	return &OrderService{repo: repo, txRunner: txRunner, now: now}
}

func NewOrderTxRunner(db cafedb.TxBeginner, repo *repository.Queries) *OrderTxRunner {
	if db == nil || repo == nil {
		return nil
	}
	return &OrderTxRunner{db: db, repo: repo}
}

func (r *OrderTxRunner) Run(ctx context.Context, fn func(orderRepository) error) error {
	if r == nil || r.db == nil || r.repo == nil {
		return errors.New("order transaction runner missing")
	}

	return cafedb.WithTx(ctx, r.db, func(ctx context.Context, tx pgx.Tx) error {
		return fn(r.repo.WithTx(tx))
	})
}

func (s *OrderService) Checkout(ctx context.Context, input CheckoutInput) (*Order, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("order transaction runner missing")
	}
	if !input.IsVerified {
		return nil, ErrOrderEmailUnverified
	}
	if strings.TrimSpace(input.PhoneNumber) == "" {
		return nil, ErrOrderPhoneNumberRequired
	}
	notes := normalizeOptionalString(input.Notes)
	if notes != nil && len(*notes) > 255 {
		return nil, ErrOrderValidation
	}
	if len(input.Items) == 0 {
		return nil, ErrOrderValidation
	}

	userUUID, err := parseRequiredUUID(input.UserID)
	if err != nil {
		return nil, ErrInvalidOrderUserID
	}

	itemIDs := make([]pgtype.UUID, 0, len(input.Items))
	seen := make(map[string]struct{}, len(input.Items))
	for _, item := range input.Items {
		itemUUID, err := parseRequiredUUID(strings.TrimSpace(item.CartItemID))
		if err != nil {
			return nil, ErrInvalidOrderCartItemID
		}
		key := itemUUID.String()
		if _, ok := seen[key]; ok {
			return nil, ErrOrderValidation
		}
		seen[key] = struct{}{}
		itemIDs = append(itemIDs, itemUUID)
	}

	var createdOrder repository.Order
	var createdItems []repository.OrderItem
	err = s.txRunner.Run(ctx, func(repo orderRepository) error {
		rows, err := repo.ListCheckoutCartItemsForUser(ctx, repository.ListCheckoutCartItemsForUserParams{
			UserID:      userUUID,
			CartItemIds: itemIDs,
		})
		if err != nil {
			return fmt.Errorf("list checkout cart items: %w", err)
		}
		if len(rows) != len(itemIDs) {
			return ErrOrderCartItemNotFound
		}

		expiredOrders, err := repo.ListExpiredPendingOrdersByCartItemIDs(ctx, repository.ListExpiredPendingOrdersByCartItemIDsParams{
			UserID:      userUUID,
			ExpiresAt:   pgtype.Timestamptz{Time: s.now(), Valid: true},
			CartItemIds: itemIDs,
		})
		if err != nil {
			return fmt.Errorf("list expired pending orders: %w", err)
		}
		if err := cancelExpiredPendingOrders(ctx, repo, expiredOrders, s.now()); err != nil {
			return err
		}

		pendingCount, err := repo.CountPendingOrderItemsByCartItemIDs(ctx, repository.CountPendingOrderItemsByCartItemIDsParams{
			UserID:      userUUID,
			CartItemIds: itemIDs,
		})
		if err != nil {
			return fmt.Errorf("count pending order items: %w", err)
		}
		if pendingCount > 0 {
			return ErrOrderCartItemAlreadyPending
		}

		rowsByID := make(map[string]repository.ListCheckoutCartItemsForUserRow, len(rows))
		for _, row := range rows {
			rowsByID[row.CartItemID.String()] = row
		}

		prepared := make([]checkoutPreparedItem, 0, len(input.Items))
		var totalAmount int32
		for index, inputItem := range input.Items {
			row := rowsByID[itemIDs[index].String()]
			if row.DeletedAt.Valid {
				return ErrOrderProductNotFound
			}
			switch row.Status {
			case repository.ProductStatusUnavailable:
				return ErrOrderProductUnavailable
			case repository.ProductStatusOutOfStock:
				return ErrOrderProductOutOfStock
			}
			if row.Stock < row.Quantity {
				return ErrOrderInsufficientStock
			}

			attrs, err := selectedAttributes(row.Category, row.Attributes, inputItem.Attributes)
			if err != nil {
				return err
			}
			subtotal := row.Price * row.Quantity
			totalAmount += subtotal
			prepared = append(prepared, checkoutPreparedItem{row: row, attrs: attrs, subtotal: subtotal})
		}

		now := s.now()
		wib := time.FixedZone("WIB", 7*60*60)
		dateKey := now.In(wib).Format("20060102")
		prefix := "ORD-" + dateKey + "-"
		if err := repo.AcquireOrderNumberDateLock(ctx, dateKey); err != nil {
			return fmt.Errorf("lock order number date: %w", err)
		}
		count, err := repo.CountOrdersByOrderNumberPrefix(ctx, prefix)
		if err != nil {
			return fmt.Errorf("count daily orders: %w", err)
		}
		orderNumber := fmt.Sprintf("%s%03d", prefix, count+1)

		order, err := repo.CreateOrder(ctx, repository.CreateOrderParams{
			OrderNumber: orderNumber,
			UserID:      userUUID,
			Status:      repository.OrderStatusPENDING,
			Notes:       optionalText(notes),
			TotalAmount: totalAmount,
			ExpiresAt:   pgtype.Timestamptz{Time: now.Add(15 * time.Minute), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("create order: %w", err)
		}
		createdOrder = order

		createdItems = make([]repository.OrderItem, 0, len(prepared))
		for _, item := range prepared {
			locked, err := repo.LockProductByIDForUpdate(ctx, item.row.ProductID)
			if err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrOrderProductNotFound
				}
				return fmt.Errorf("lock product: %w", err)
			}
			if locked.DeletedAt.Valid {
				return ErrOrderProductNotFound
			}
			switch locked.Status {
			case repository.ProductStatusUnavailable:
				return ErrOrderProductUnavailable
			case repository.ProductStatusOutOfStock:
				return ErrOrderProductOutOfStock
			}
			if locked.Stock < item.row.Quantity {
				return ErrOrderInsufficientStock
			}

			orderItem, err := repo.CreateOrderItem(ctx, repository.CreateOrderItemParams{
				OrderID:            order.ID,
				ProductID:          item.row.ProductID,
				CartItemID:         item.row.CartItemID,
				ProductName:        item.row.ProductName,
				PriceAtCheckout:    item.row.Price,
				Quantity:           item.row.Quantity,
				Subtotal:           item.subtotal,
				SelectedAttributes: item.attrs,
			})
			if err != nil {
				return fmt.Errorf("create order item: %w", err)
			}
			createdItems = append(createdItems, orderItem)

			if _, err := repo.DecrementProductStock(ctx, repository.DecrementProductStockParams{
				ID:       item.row.ProductID,
				Quantity: item.row.Quantity,
			}); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return ErrOrderInsufficientStock
				}
				return fmt.Errorf("decrement product stock: %w", err)
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return orderFromRows(createdOrder, createdItems), nil
}

func (s *OrderService) ListOrders(ctx context.Context, input ListOrdersInput) (*OrderList, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	actorUUID, err := parseRequiredUUID(input.ActorUserID)
	if err != nil {
		return nil, ErrInvalidOrderUserID
	}
	limit := normalizeOrderLimit(input.Limit)
	offset, err := decodeOrderCursor(input.Cursor)
	if err != nil {
		return nil, ErrOrderInvalidCursor
	}
	if input.Direction == "prev" {
		offset -= int(limit)
		if offset < 0 {
			offset = 0
		}
	} else if input.Direction != "" && input.Direction != "next" {
		return nil, ErrOrderValidation
	}

	userFilter := pgtype.UUID{}
	switch repository.UserRole(input.ActorRole) {
	case repository.UserRoleCUSTOMER:
		userFilter = actorUUID
	case repository.UserRolePEGAWAI:
	case repository.UserRoleADMIN:
		if strings.TrimSpace(input.UserID) != "" {
			parsed, err := parseRequiredUUID(input.UserID)
			if err != nil {
				return nil, ErrInvalidOrderUserID
			}
			userFilter = parsed
		}
	default:
		return nil, ErrOrderForbidden
	}

	statusFilter := ""
	if strings.TrimSpace(input.Status) != "" {
		status := repository.OrderStatus(input.Status)
		if !isOrderStatus(status) {
			return nil, ErrOrderInvalidStatus
		}
		statusFilter = string(status)
	}

	rows, err := s.repo.ListOrders(ctx, repository.ListOrdersParams{
		UserID: userFilter,
		Status: statusFilter,
		Limit:  limit + 1,
		Offset: int32(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("list orders: %w", err)
	}

	hasNext := len(rows) > int(limit)
	if hasNext {
		rows = rows[:limit]
	}
	items := make([]OrderSummary, 0, len(rows))
	for _, row := range rows {
		items = append(items, orderSummaryFromRow(row))
	}

	var nextCursor *string
	if hasNext {
		value := encodeOrderCursor(offset + int(limit))
		nextCursor = &value
	}
	var prevCursor *string
	if offset > 0 {
		prevOffset := offset - int(limit)
		if prevOffset < 0 {
			prevOffset = 0
		}
		value := encodeOrderCursor(prevOffset)
		prevCursor = &value
	}

	return &OrderList{
		Items:      items,
		NextCursor: nextCursor,
		PrevCursor: prevCursor,
		Limit:      limit,
		HasNext:    hasNext,
		HasPrev:    offset > 0,
	}, nil
}

func (s *OrderService) GetOrder(ctx context.Context, input GetOrderInput) (*Order, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	orderUUID, err := parseRequiredUUID(input.OrderID)
	if err != nil {
		return nil, ErrInvalidOrderID
	}
	actorUUID, err := parseRequiredUUID(input.ActorUserID)
	if err != nil {
		return nil, ErrInvalidOrderUserID
	}

	order, err := s.repo.GetOrderByID(ctx, orderUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("get order: %w", err)
	}

	switch repository.UserRole(input.ActorRole) {
	case repository.UserRoleCUSTOMER:
		if order.UserID.String() != actorUUID.String() {
			return nil, ErrOrderNotFound
		}
	case repository.UserRolePEGAWAI, repository.UserRoleADMIN:
	default:
		return nil, ErrOrderForbidden
	}

	items, err := s.repo.ListOrderItemsByOrderID(ctx, order.ID)
	if err != nil {
		return nil, fmt.Errorf("list order items: %w", err)
	}

	return orderFromRows(order, items), nil
}

func (s *OrderService) CancelOrder(ctx context.Context, input CancelOrderInput) (*OrderStatusUpdate, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("order transaction runner missing")
	}

	orderUUID, err := parseRequiredUUID(input.OrderID)
	if err != nil {
		return nil, ErrInvalidOrderID
	}
	actorUUID, err := parseRequiredUUID(input.ActorUserID)
	if err != nil {
		return nil, ErrInvalidOrderUserID
	}
	role := repository.UserRole(input.ActorRole)
	if role != repository.UserRoleCUSTOMER && role != repository.UserRoleADMIN {
		return nil, ErrOrderForbidden
	}

	var updated repository.Order
	err = s.txRunner.Run(ctx, func(repo orderRepository) error {
		order, err := repo.LockOrderByIDForUpdate(ctx, orderUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrOrderNotFound
			}
			return fmt.Errorf("lock order: %w", err)
		}
		if role == repository.UserRoleCUSTOMER && order.UserID.String() != actorUUID.String() {
			return ErrOrderNotFound
		}
		if order.Status == repository.OrderStatusCANCELLED {
			return ErrOrderAlreadyCancelled
		}
		if order.Status != repository.OrderStatusPENDING {
			return ErrOrderNotCancellable
		}

		items, err := repo.ListOrderItemsByOrderID(ctx, order.ID)
		if err != nil {
			return fmt.Errorf("list order items: %w", err)
		}
		for _, item := range items {
			if _, err := repo.LockProductByIDForUpdate(ctx, item.ProductID); err != nil {
				return fmt.Errorf("lock product: %w", err)
			}
			if _, err := repo.IncrementProductStock(ctx, repository.IncrementProductStockParams{
				ID:       item.ProductID,
				Quantity: item.Quantity,
			}); err != nil {
				return fmt.Errorf("restore product stock: %w", err)
			}
		}

		updatedOrder, err := repo.UpdateOrderStatus(ctx, repository.UpdateOrderStatusParams{
			ID:     order.ID,
			Status: repository.OrderStatusCANCELLED,
		})
		if err != nil {
			return fmt.Errorf("update order status: %w", err)
		}
		updated = updatedOrder
		return nil
	})
	if err != nil {
		return nil, err
	}

	return orderStatusUpdateFromRow(updated), nil
}

func (s *OrderService) UpdateOrderStatus(ctx context.Context, input UpdateOrderStatusInput) (*OrderStatusUpdate, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("order transaction runner missing")
	}

	orderUUID, err := parseRequiredUUID(input.OrderID)
	if err != nil {
		return nil, ErrInvalidOrderID
	}
	role := repository.UserRole(input.ActorRole)
	if role != repository.UserRolePEGAWAI && role != repository.UserRoleADMIN {
		return nil, ErrOrderForbidden
	}
	targetStatus := repository.OrderStatus(strings.TrimSpace(input.Status))
	if !isOrderStatus(targetStatus) {
		return nil, ErrOrderInvalidStatus
	}
	if targetStatus != repository.OrderStatusCOMPLETED {
		return nil, ErrOrderInvalidStatusTransition
	}

	var updated repository.Order
	err = s.txRunner.Run(ctx, func(repo orderRepository) error {
		order, err := repo.LockOrderByIDForUpdate(ctx, orderUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrOrderNotFound
			}
			return fmt.Errorf("lock order: %w", err)
		}
		if order.Status != repository.OrderStatusCONFIRMED {
			return ErrOrderInvalidStatusTransition
		}

		updatedOrder, err := repo.UpdateOrderStatus(ctx, repository.UpdateOrderStatusParams{
			ID:     order.ID,
			Status: repository.OrderStatusCOMPLETED,
		})
		if err != nil {
			return fmt.Errorf("update order status: %w", err)
		}

		items, err := repo.ListOrderItemsByOrderID(ctx, order.ID)
		if err != nil {
			return fmt.Errorf("list order items: %w", err)
		}
		for _, item := range items {
			if _, err := repo.IncrementProductTotalSold(ctx, repository.IncrementProductTotalSoldParams{
				ID:       item.ProductID,
				Quantity: item.Quantity,
			}); err != nil {
				return fmt.Errorf("increment product total sold: %w", err)
			}
		}

		updated = updatedOrder
		return nil
	})
	if err != nil {
		return nil, err
	}

	return orderStatusUpdateFromRow(updated), nil
}

func (s *OrderService) ConfirmOrderFromPayment(ctx context.Context, input InternalConfirmOrderInput) (*InternalConfirmOrderResult, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}
	if s.txRunner == nil {
		return nil, errors.New("order transaction runner missing")
	}

	orderUUID, err := parseRequiredUUID(input.OrderID)
	if err != nil {
		return nil, ErrInvalidOrderID
	}

	var result InternalConfirmOrderResult
	err = s.txRunner.Run(ctx, func(repo orderRepository) error {
		order, err := repo.LockOrderByIDForUpdate(ctx, orderUUID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrOrderNotFound
			}
			return fmt.Errorf("lock order: %w", err)
		}
		if order.Status != repository.OrderStatusPENDING && order.Status != repository.OrderStatusCONFIRMED {
			return ErrOrderInvalidStatusTransition
		}

		confirmed := order
		if order.Status == repository.OrderStatusPENDING {
			updatedOrder, err := repo.UpdateOrderStatus(ctx, repository.UpdateOrderStatusParams{
				ID:     order.ID,
				Status: repository.OrderStatusCONFIRMED,
			})
			if err != nil {
				return fmt.Errorf("update order status: %w", err)
			}
			confirmed = updatedOrder
		}

		items, err := repo.ListOrderItemsByOrderID(ctx, order.ID)
		if err != nil {
			return fmt.Errorf("list order items: %w", err)
		}
		cartItemIDs := make([]string, 0, len(items))
		for _, item := range items {
			if item.CartItemID.Valid {
				cartItemIDs = append(cartItemIDs, item.CartItemID.String())
			}
		}

		result = InternalConfirmOrderResult{
			OrderID:     confirmed.ID.String(),
			Status:      string(confirmed.Status),
			CartItemIDs: cartItemIDs,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return &result, nil
}

func cancelExpiredPendingOrders(ctx context.Context, repo orderRepository, orders []repository.Order, now time.Time) error {
	for _, expired := range orders {
		order, err := repo.LockOrderByIDForUpdate(ctx, expired.ID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return fmt.Errorf("lock expired order: %w", err)
		}
		if order.Status != repository.OrderStatusPENDING {
			continue
		}
		if !order.ExpiresAt.Valid || now.Before(order.ExpiresAt.Time) {
			continue
		}

		items, err := repo.ListOrderItemsByOrderID(ctx, order.ID)
		if err != nil {
			return fmt.Errorf("list expired order items: %w", err)
		}
		for _, item := range items {
			if _, err := repo.LockProductByIDForUpdate(ctx, item.ProductID); err != nil {
				return fmt.Errorf("lock expired order product: %w", err)
			}
			if _, err := repo.IncrementProductStock(ctx, repository.IncrementProductStockParams{
				ID:       item.ProductID,
				Quantity: item.Quantity,
			}); err != nil {
				return fmt.Errorf("restore expired order stock: %w", err)
			}
		}
		if _, err := repo.UpdateOrderStatus(ctx, repository.UpdateOrderStatusParams{
			ID:     order.ID,
			Status: repository.OrderStatusCANCELLED,
		}); err != nil {
			return fmt.Errorf("cancel expired order: %w", err)
		}
	}
	return nil
}

func selectedAttributes(category repository.ProductCategory, productAttributes []byte, input map[string]string) ([]byte, error) {
	options := map[string][]string{}
	if len(productAttributes) > 0 {
		if err := json.Unmarshal(productAttributes, &options); err != nil {
			return nil, ErrOrderValidation
		}
	}

	selected := map[string]string{}
	switch category {
	case repository.ProductCategoryCoffee:
		temperature, err := requiredAttribute(input, options, "temperature")
		if err != nil {
			return nil, err
		}
		size, err := requiredAttribute(input, options, "sizes")
		if err != nil {
			return nil, err
		}
		sugar, err := optionalAttribute(input, options, "sugar_levels", "normal")
		if err != nil {
			return nil, err
		}
		selected["temperature"] = temperature
		selected["sizes"] = size
		selected["sugar_levels"] = sugar
		if temperature == "iced" {
			ice, err := optionalAttribute(input, options, "ice_levels", "normal")
			if err != nil {
				return nil, err
			}
			selected["ice_levels"] = ice
		}
	case repository.ProductCategoryFood, repository.ProductCategorySnack:
		portion, err := requiredAttribute(input, options, "portions")
		if err != nil {
			return nil, err
		}
		spicy, err := optionalAttribute(input, options, "spicy_levels", "no_spicy")
		if err != nil {
			return nil, err
		}
		selected["portions"] = portion
		selected["spicy_levels"] = spicy
	default:
		return nil, ErrOrderValidation
	}

	raw, err := json.Marshal(selected)
	if err != nil {
		return nil, ErrOrderValidation
	}
	return raw, nil
}

func requiredAttribute(input map[string]string, options map[string][]string, key string) (string, error) {
	value := strings.TrimSpace(input[key])
	if value == "" {
		return "", ErrOrderValidation
	}
	if !containsString(options[key], value) {
		return "", ErrOrderValidation
	}
	return value, nil
}

func optionalAttribute(input map[string]string, options map[string][]string, key, defaultValue string) (string, error) {
	value := strings.TrimSpace(input[key])
	if value == "" {
		value = defaultValue
	}
	allowed := options[key]
	if len(allowed) > 0 && !containsString(allowed, value) {
		return "", ErrOrderValidation
	}
	return value, nil
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func normalizeOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func normalizeOrderLimit(limit int32) int32 {
	if limit <= 0 {
		return 10
	}
	if limit > 50 {
		return 50
	}
	return limit
}

func encodeOrderCursor(offset int) string {
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}

func decodeOrderCursor(raw string) (int, error) {
	if strings.TrimSpace(raw) == "" {
		return 0, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		return 0, err
	}
	offset, err := strconv.Atoi(string(decoded))
	if err != nil || offset < 0 {
		return 0, ErrOrderInvalidCursor
	}
	return offset, nil
}

func isOrderStatus(status repository.OrderStatus) bool {
	switch status {
	case repository.OrderStatusPENDING, repository.OrderStatusCONFIRMED, repository.OrderStatusCOMPLETED, repository.OrderStatusCANCELLED:
		return true
	default:
		return false
	}
}

func orderFromRows(order repository.Order, items []repository.OrderItem) *Order {
	result := &Order{
		ID:          order.ID.String(),
		OrderNumber: order.OrderNumber,
		UserID:      order.UserID.String(),
		Status:      string(order.Status),
		Notes:       textPtr(order.Notes),
		TotalAmount: order.TotalAmount,
		ExpiresAt:   timestamptzPtr(order.ExpiresAt),
		Items:       make([]OrderItem, 0, len(items)),
		CreatedAt:   order.CreatedAt.Time,
		UpdatedAt:   order.UpdatedAt.Time,
	}
	for _, item := range items {
		result.Items = append(result.Items, OrderItem{
			ID:                 item.ID.String(),
			ProductID:          item.ProductID.String(),
			ProductName:        item.ProductName,
			PriceAtCheckout:    item.PriceAtCheckout,
			Quantity:           item.Quantity,
			Subtotal:           item.Subtotal,
			SelectedAttributes: item.SelectedAttributes,
		})
	}
	return result
}

func orderSummaryFromRow(row repository.ListOrdersRow) OrderSummary {
	return OrderSummary{
		ID:          row.ID.String(),
		OrderNumber: row.OrderNumber,
		Status:      string(row.Status),
		TotalAmount: row.TotalAmount,
		CreatedAt:   row.CreatedAt.Time,
	}
}

func orderStatusUpdateFromRow(row repository.Order) *OrderStatusUpdate {
	return &OrderStatusUpdate{
		OrderID:   row.ID.String(),
		Status:    string(row.Status),
		UpdatedAt: row.UpdatedAt.Time,
	}
}
