package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5/pgtype"
)

const ProductCreatedEventType = "product.created"

type ProductCreatedPayload struct {
	Type      string `json:"type"`
	ProductID string `json:"product_id"`
	Name      string `json:"name"`
	Category  string `json:"category"`
	ImageURL  string `json:"image_url"`
}

type ProductCreatedMessage struct {
	Type              string
	ProductID         string
	Name              string
	Category          string
	ImageURL          string
	NotificationTitle string
	NotificationBody  string
}

type ProductCreatedSender interface {
	SendProductCreated(ctx context.Context, message ProductCreatedMessage) error
}

type Repository interface {
	LockPendingOutboxEvents(ctx context.Context, limit int32) ([]repository.OutboxEvent, error)
	MarkOutboxProcessing(ctx context.Context, id pgtype.UUID) error
	MarkOutboxSent(ctx context.Context, id pgtype.UUID) error
	MarkOutboxRetry(ctx context.Context, arg repository.MarkOutboxRetryParams) error
}

type ProcessorOptions struct {
	MaxRetries int32
	RetryDelay time.Duration
	Now        func() time.Time
}

type Processor struct {
	repo       Repository
	sender     ProductCreatedSender
	maxRetries int32
	retryDelay time.Duration
	now        func() time.Time
}

func NewProcessor(repo Repository, sender ProductCreatedSender, options ProcessorOptions) *Processor {
	maxRetries := options.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 3
	}
	retryDelay := options.RetryDelay
	if retryDelay <= 0 {
		retryDelay = 30 * time.Second
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}

	return &Processor{
		repo:       repo,
		sender:     sender,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		now:        now,
	}
}

func (p *Processor) ProcessBatch(ctx context.Context, limit int32) (int, error) {
	if p.repo == nil {
		return 0, fmt.Errorf("outbox repository missing")
	}
	if p.sender == nil {
		return 0, fmt.Errorf("product created sender missing")
	}
	if limit <= 0 {
		limit = 10
	}

	events, err := p.repo.LockPendingOutboxEvents(ctx, limit)
	if err != nil {
		return 0, fmt.Errorf("lock pending outbox events: %w", err)
	}

	processed := 0
	for _, event := range events {
		processed++
		if err := p.processEvent(ctx, event); err != nil {
			return processed, err
		}
	}

	return processed, nil
}

func (p *Processor) processEvent(ctx context.Context, event repository.OutboxEvent) error {
	if err := p.repo.MarkOutboxProcessing(ctx, event.ID); err != nil {
		return fmt.Errorf("mark outbox processing: %w", err)
	}

	err := p.dispatch(ctx, event)
	if err == nil {
		if markErr := p.repo.MarkOutboxSent(ctx, event.ID); markErr != nil {
			return fmt.Errorf("mark outbox sent: %w", markErr)
		}
		return nil
	}

	dead := event.RetryCount+1 >= p.maxRetries
	retryErr := p.repo.MarkOutboxRetry(ctx, repository.MarkOutboxRetryParams{
		ID:          event.ID,
		NextRetryAt: pgtype.Timestamptz{Time: p.now().Add(p.retryDelay), Valid: true},
		Dead:        dead,
		LastError:   pgtype.Text{String: err.Error(), Valid: true},
	})
	if retryErr != nil {
		return fmt.Errorf("mark outbox retry: %w", retryErr)
	}

	return nil
}

func (p *Processor) dispatch(ctx context.Context, event repository.OutboxEvent) error {
	switch event.EventType {
	case ProductCreatedEventType:
		return p.dispatchProductCreated(ctx, event)
	default:
		return fmt.Errorf("unsupported outbox event type: %s", event.EventType)
	}
}

func (p *Processor) dispatchProductCreated(ctx context.Context, event repository.OutboxEvent) error {
	var payload ProductCreatedPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		return fmt.Errorf("decode product created payload: %w", err)
	}

	message := ProductCreatedMessage{
		Type:              payload.Type,
		ProductID:         payload.ProductID,
		Name:              payload.Name,
		Category:          payload.Category,
		ImageURL:          payload.ImageURL,
		NotificationTitle: "Produk baru tersedia",
		NotificationBody:  payload.Name + " sudah tersedia.",
	}
	if err := p.sender.SendProductCreated(ctx, message); err != nil {
		return fmt.Errorf("send product created notification: %w", err)
	}

	return nil
}
