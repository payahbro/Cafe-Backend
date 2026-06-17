package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/repository"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestReportHandlerGetRevenueReturnsTotalRevenue(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeReportService{
		revenue: &service.RevenueReport{TotalRevenue: 250000, Currency: "IDR"},
	}
	router := gin.New()
	reportHandler := NewReportHandler(fakeService)
	router.GET("/reports/revenue", authenticatedReportAdminMiddleware(), reportHandler.GetRevenue)

	req := httptest.NewRequest(http.MethodGet, "/reports/revenue?date_from=2026-06-01&date_to=2026-06-30", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.revenueCalled {
		t.Fatalf("expected revenue service call")
	}
	if fakeService.revenueInput.DateFrom == nil || !fakeService.revenueInput.DateFrom.Equal(time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date_from = %v", fakeService.revenueInput.DateFrom)
	}
	if fakeService.revenueInput.DateTo == nil || !fakeService.revenueInput.DateTo.Equal(time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("date_to exclusive = %v", fakeService.revenueInput.DateTo)
	}
	for _, want := range []string{
		`"success":true`,
		`"total_revenue":250000`,
		`"currency":"IDR"`,
		`"message":"Report pendapatan berhasil diambil"`,
	} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("response body missing %s: %s", want, resp.Body.String())
		}
	}
}

func TestReportHandlerGetProductsSoldReturnsTotalAndBreakdown(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeReportService{
		productsSold: &service.ProductsSoldReport{
			TotalProductsSold: 12,
			Items: []service.ProductSoldItem{
				{
					ProductID:    "33333333-3333-4333-8333-333333333333",
					ProductName:  "Americano",
					QuantitySold: 8,
				},
				{
					ProductID:    "44444444-4444-4444-8444-444444444444",
					ProductName:  "Latte",
					QuantitySold: 4,
				},
			},
		},
	}
	router := gin.New()
	reportHandler := NewReportHandler(fakeService)
	router.GET("/reports/products-sold", authenticatedReportAdminMiddleware(), reportHandler.GetProductsSold)

	req := httptest.NewRequest(http.MethodGet, "/reports/products-sold?date_from=2026-06-01&date_to=2026-06-30", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !fakeService.productsSoldCalled {
		t.Fatalf("expected products sold service call")
	}
	for _, want := range []string{
		`"success":true`,
		`"total_products_sold":12`,
		`"product_name":"Americano"`,
		`"quantity_sold":8`,
		`"message":"Report produk terjual berhasil diambil"`,
	} {
		if !strings.Contains(resp.Body.String(), want) {
			t.Fatalf("response body missing %s: %s", want, resp.Body.String())
		}
	}
}

func TestReportHandlerRejectsInvalidDate(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeService := &fakeReportService{}
	router := gin.New()
	reportHandler := NewReportHandler(fakeService)
	router.GET("/reports/revenue", authenticatedReportAdminMiddleware(), reportHandler.GetRevenue)

	req := httptest.NewRequest(http.MethodGet, "/reports/revenue?date_from=2026/06/01", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if fakeService.revenueCalled {
		t.Fatalf("service should not be called for invalid date")
	}
	if !strings.Contains(resp.Body.String(), `"code":"VALIDATION_ERROR"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

type fakeReportService struct {
	revenue            *service.RevenueReport
	productsSold       *service.ProductsSoldReport
	err                error
	revenueInput       service.ReportRangeInput
	productsSoldInput  service.ReportRangeInput
	revenueCalled      bool
	productsSoldCalled bool
}

func (f *fakeReportService) GetRevenue(_ context.Context, input service.ReportRangeInput) (*service.RevenueReport, error) {
	f.revenueCalled = true
	f.revenueInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.revenue, nil
}

func (f *fakeReportService) GetProductsSold(_ context.Context, input service.ReportRangeInput) (*service.ProductsSoldReport, error) {
	f.productsSoldCalled = true
	f.productsSoldInput = input
	if f.err != nil {
		return nil, f.err
	}
	return f.productsSold, nil
}

func authenticatedReportAdminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		var userID pgtype.UUID
		_ = userID.Scan("99999999-9999-4999-8999-999999999999")
		c.Set("authenticated_user", middleware.AuthenticatedUser{
			ID:         "99999999-9999-4999-8999-999999999999",
			UUID:       userID,
			Role:       repository.UserRoleADMIN,
			IsVerified: true,
			IsActive:   true,
		})
		c.Next()
	}
}
