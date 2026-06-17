package service

import (
	"context"
	"testing"
	"time"

	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestReportServiceGetRevenueReturnsSuccessfulPaymentTotal(t *testing.T) {
	from := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	to := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	repo := &fakeReportRepo{totalRevenue: 250000}
	service := NewReportService(repo)

	result, err := service.GetRevenue(context.Background(), ReportRangeInput{
		DateFrom: &from,
		DateTo:   &to,
	})
	if err != nil {
		t.Fatalf("get revenue: %v", err)
	}

	if !repo.revenueCalled {
		t.Fatalf("expected revenue query")
	}
	if !repo.revenueArg.DateFrom.Valid || !repo.revenueArg.DateFrom.Time.Equal(from) {
		t.Fatalf("date_from = %+v", repo.revenueArg.DateFrom)
	}
	if !repo.revenueArg.DateTo.Valid || !repo.revenueArg.DateTo.Time.Equal(to) {
		t.Fatalf("date_to = %+v", repo.revenueArg.DateTo)
	}
	if result.TotalRevenue != 250000 || result.Currency != "IDR" {
		t.Fatalf("result = %+v", result)
	}
}

func TestReportServiceGetProductsSoldReturnsTotalAndItems(t *testing.T) {
	repo := &fakeReportRepo{
		productsSold: []repository.ListProductsSoldReportRow{
			{
				ProductID:    mustUUIDForReport(t, "33333333-3333-4333-8333-333333333333"),
				ProductName:  "Americano",
				QuantitySold: 8,
			},
			{
				ProductID:    mustUUIDForReport(t, "44444444-4444-4444-8444-444444444444"),
				ProductName:  "Latte",
				QuantitySold: 4,
			},
		},
	}
	service := NewReportService(repo)

	result, err := service.GetProductsSold(context.Background(), ReportRangeInput{})
	if err != nil {
		t.Fatalf("get products sold: %v", err)
	}

	if !repo.productsSoldCalled {
		t.Fatalf("expected products sold query")
	}
	if result.TotalProductsSold != 12 {
		t.Fatalf("total products sold = %d", result.TotalProductsSold)
	}
	if len(result.Items) != 2 {
		t.Fatalf("items len = %d", len(result.Items))
	}
	if result.Items[0].ProductName != "Americano" || result.Items[0].QuantitySold != 8 {
		t.Fatalf("first item = %+v", result.Items[0])
	}
}

type fakeReportRepo struct {
	totalRevenue       int64
	productsSold       []repository.ListProductsSoldReportRow
	revenueArg         repository.GetRevenueReportParams
	productsSoldArg    repository.ListProductsSoldReportParams
	revenueCalled      bool
	productsSoldCalled bool
}

func (f *fakeReportRepo) GetRevenueReport(ctx context.Context, arg repository.GetRevenueReportParams) (int64, error) {
	f.revenueCalled = true
	f.revenueArg = arg
	return f.totalRevenue, nil
}

func (f *fakeReportRepo) ListProductsSoldReport(ctx context.Context, arg repository.ListProductsSoldReportParams) ([]repository.ListProductsSoldReportRow, error) {
	f.productsSoldCalled = true
	f.productsSoldArg = arg
	return f.productsSold, nil
}

func mustUUIDForReport(t *testing.T, value string) pgtype.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := id.Scan(value); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}
	return id
}
