package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestProcessorSendsProductCreatedEventWithDynamicPayload(t *testing.T) {
	eventID := uuidValue(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	productID := uuidValue(t, "11111111-1111-4111-8111-111111111111")
	payload := ProductCreatedPayload{
		Type:      "product_created",
		ProductID: productID.String(),
		Name:      "Nasi Goreng",
		Category:  "snack",
		ImageURL:  "https://example.supabase.co/storage/v1/object/public/products/nasi-goreng.png",
	}
	rawPayload, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	repo := &fakeOutboxRepo{
		events: []repository.OutboxEvent{
			{
				ID:            eventID,
				AggregateType: "product",
				AggregateID:   productID,
				EventType:     "product.created",
				Payload:       rawPayload,
				RetryCount:    0,
			},
		},
	}
	sender := &fakeProductCreatedSender{}
	processor := NewProcessor(repo, sender, ProcessorOptions{
		MaxRetries: 3,
		RetryDelay: 5 * time.Minute,
		Now: func() time.Time {
			return time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
		},
	})

	processed, err := processor.ProcessBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("process batch: %v", err)
	}

	if processed != 1 {
		t.Fatalf("processed = %d", processed)
	}
	if sender.message.Name != "Nasi Goreng" {
		t.Fatalf("message name = %q", sender.message.Name)
	}
	if sender.message.Category != "snack" {
		t.Fatalf("message category = %q", sender.message.Category)
	}
	if sender.message.NotificationTitle != "Produk baru tersedia" {
		t.Fatalf("notification title = %q", sender.message.NotificationTitle)
	}
	if sender.message.NotificationBody != "Nasi Goreng sudah tersedia." {
		t.Fatalf("notification body = %q", sender.message.NotificationBody)
	}
	if !repo.markProcessingCalled {
		t.Fatalf("expected event to be marked processing")
	}
	if repo.sentID.String() != eventID.String() {
		t.Fatalf("sent id = %q", repo.sentID.String())
	}
	if repo.retryCalled {
		t.Fatalf("event should not be retried on successful send")
	}
}

func TestProcessorRetriesProductCreatedEventWhenSenderFails(t *testing.T) {
	eventID := uuidValue(t, "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa")
	productID := uuidValue(t, "11111111-1111-4111-8111-111111111111")
	rawPayload, err := json.Marshal(ProductCreatedPayload{
		Type:      "product_created",
		ProductID: productID.String(),
		Name:      "Cafe Latte",
		Category:  "coffee",
		ImageURL:  "",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	repo := &fakeOutboxRepo{
		events: []repository.OutboxEvent{
			{
				ID:            eventID,
				AggregateType: "product",
				AggregateID:   productID,
				EventType:     "product.created",
				Payload:       rawPayload,
				RetryCount:    1,
			},
		},
	}
	sender := &fakeProductCreatedSender{err: errors.New("fcm unavailable")}
	now := time.Date(2026, 6, 20, 1, 0, 0, 0, time.UTC)
	processor := NewProcessor(repo, sender, ProcessorOptions{
		MaxRetries: 3,
		RetryDelay: 5 * time.Minute,
		Now: func() time.Time {
			return now
		},
	})

	processed, err := processor.ProcessBatch(context.Background(), 10)
	if err != nil {
		t.Fatalf("process batch: %v", err)
	}

	if processed != 1 {
		t.Fatalf("processed = %d", processed)
	}
	if !repo.retryCalled {
		t.Fatalf("expected retry mark")
	}
	if repo.retryArg.ID.String() != eventID.String() {
		t.Fatalf("retry id = %q", repo.retryArg.ID.String())
	}
	if repo.retryArg.Dead {
		t.Fatalf("event should not be dead before max retries")
	}
	if !repo.retryArg.NextRetryAt.Time.Equal(now.Add(5 * time.Minute)) {
		t.Fatalf("next retry at = %v", repo.retryArg.NextRetryAt.Time)
	}
	if repo.retryArg.LastError.String != "send product created notification: fcm unavailable" {
		t.Fatalf("last error = %q", repo.retryArg.LastError.String)
	}
}

type fakeOutboxRepo struct {
	events               []repository.OutboxEvent
	lockLimit            int32
	markProcessingCalled bool
	sentID               pgtype.UUID
	retryArg             repository.MarkOutboxRetryParams
	retryCalled          bool
}

func (f *fakeOutboxRepo) LockPendingOutboxEvents(ctx context.Context, limit int32) ([]repository.OutboxEvent, error) {
	f.lockLimit = limit
	return f.events, nil
}

func (f *fakeOutboxRepo) MarkOutboxProcessing(ctx context.Context, id pgtype.UUID) error {
	f.markProcessingCalled = true
	return nil
}

func (f *fakeOutboxRepo) MarkOutboxSent(ctx context.Context, id pgtype.UUID) error {
	f.sentID = id
	return nil
}

func (f *fakeOutboxRepo) MarkOutboxRetry(ctx context.Context, arg repository.MarkOutboxRetryParams) error {
	f.retryCalled = true
	f.retryArg = arg
	return nil
}

type fakeProductCreatedSender struct {
	message ProductCreatedMessage
	err     error
}

func (f *fakeProductCreatedSender) SendProductCreated(ctx context.Context, message ProductCreatedMessage) error {
	f.message = message
	return f.err
}

func uuidValue(t *testing.T, value string) pgtype.UUID {
	t.Helper()

	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}
