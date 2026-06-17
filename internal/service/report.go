package service

import (
	"context"
	"fmt"
	"time"

	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5/pgtype"
)

type reportRepository interface {
	GetRevenueReport(ctx context.Context, arg repository.GetRevenueReportParams) (int64, error)
	ListProductsSoldReport(ctx context.Context, arg repository.ListProductsSoldReportParams) ([]repository.ListProductsSoldReportRow, error)
}

type ReportService struct {
	repo reportRepository
}

type ReportRangeInput struct {
	DateFrom *time.Time
	DateTo   *time.Time
}

type RevenueReport struct {
	TotalRevenue int64
	Currency     string
}

type ProductSoldItem struct {
	ProductID    string
	ProductName  string
	QuantitySold int32
}

type ProductsSoldReport struct {
	TotalProductsSold int32
	Items             []ProductSoldItem
}

func NewReportService(repo reportRepository) *ReportService {
	return &ReportService{repo: repo}
}

func (s *ReportService) GetRevenue(ctx context.Context, input ReportRangeInput) (*RevenueReport, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database repository missing")
	}
	total, err := s.repo.GetRevenueReport(ctx, repository.GetRevenueReportParams{
		DateFrom: reportDate(input.DateFrom),
		DateTo:   reportDate(input.DateTo),
	})
	if err != nil {
		return nil, fmt.Errorf("get revenue report: %w", err)
	}
	return &RevenueReport{
		TotalRevenue: total,
		Currency:     "IDR",
	}, nil
}

func (s *ReportService) GetProductsSold(ctx context.Context, input ReportRangeInput) (*ProductsSoldReport, error) {
	if s.repo == nil {
		return nil, fmt.Errorf("database repository missing")
	}
	rows, err := s.repo.ListProductsSoldReport(ctx, repository.ListProductsSoldReportParams{
		DateFrom: reportDate(input.DateFrom),
		DateTo:   reportDate(input.DateTo),
	})
	if err != nil {
		return nil, fmt.Errorf("list products sold report: %w", err)
	}

	result := &ProductsSoldReport{Items: make([]ProductSoldItem, 0, len(rows))}
	for _, row := range rows {
		result.TotalProductsSold += row.QuantitySold
		result.Items = append(result.Items, ProductSoldItem{
			ProductID:    row.ProductID.String(),
			ProductName:  row.ProductName,
			QuantitySold: row.QuantitySold,
		})
	}
	return result, nil
}

func reportDate(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}
