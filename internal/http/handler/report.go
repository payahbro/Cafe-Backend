package handler

import (
	"context"
	"net/http"
	"strings"
	"time"

	"cafeTelkom/internal/http/dto"
	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/repository"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
)

type ReportHandler struct {
	reportService reportManager
}

type reportManager interface {
	GetRevenue(ctx context.Context, input service.ReportRangeInput) (*service.RevenueReport, error)
	GetProductsSold(ctx context.Context, input service.ReportRangeInput) (*service.ProductsSoldReport, error)
}

type RevenueReportResponse struct {
	TotalRevenue int64  `json:"total_revenue"`
	Currency     string `json:"currency"`
}

type ProductsSoldReportResponse struct {
	TotalProductsSold int32                     `json:"total_products_sold"`
	Items             []ProductSoldItemResponse `json:"items"`
}

type ProductSoldItemResponse struct {
	ProductID    string `json:"product_id"`
	ProductName  string `json:"product_name"`
	QuantitySold int32  `json:"quantity_sold"`
}

func NewReportHandler(reportService reportManager) *ReportHandler {
	return &ReportHandler{reportService: reportService}
}

func (h *ReportHandler) GetRevenue(c *gin.Context) {
	if h.reportService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Report service unavailable", nil)
		return
	}
	if !isAuthenticatedReportAdmin(c) {
		return
	}
	input, validationErrors := parseReportRange(c)
	if len(validationErrors) > 0 {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", validationErrors)
		return
	}

	report, err := h.reportService.GetRevenue(c.Request.Context(), input)
	if err != nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		return
	}
	dto.WriteSuccess(c, http.StatusOK, revenueReportResponse(report), "Report pendapatan berhasil diambil")
}

func (h *ReportHandler) GetProductsSold(c *gin.Context) {
	if h.reportService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Report service unavailable", nil)
		return
	}
	if !isAuthenticatedReportAdmin(c) {
		return
	}
	input, validationErrors := parseReportRange(c)
	if len(validationErrors) > 0 {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", validationErrors)
		return
	}

	report, err := h.reportService.GetProductsSold(c.Request.Context(), input)
	if err != nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		return
	}
	dto.WriteSuccess(c, http.StatusOK, productsSoldReportResponse(report), "Report produk terjual berhasil diambil")
}

func isAuthenticatedReportAdmin(c *gin.Context) bool {
	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return false
	}
	if user.Role != repository.UserRoleADMIN {
		dto.WriteError(c, http.StatusForbidden, "FORBIDDEN", "Role tidak diizinkan", nil)
		return false
	}
	return true
}

func parseReportRange(c *gin.Context) (service.ReportRangeInput, map[string]string) {
	errors := map[string]string{}
	input := service.ReportRangeInput{}

	dateFrom, ok := parseReportDate(strings.TrimSpace(c.Query("date_from")))
	if !ok {
		errors["date_from"] = "Tanggal mulai harus format YYYY-MM-DD"
	} else {
		input.DateFrom = dateFrom
	}

	var inclusiveDateTo *time.Time
	dateTo, ok := parseReportDate(strings.TrimSpace(c.Query("date_to")))
	if !ok {
		errors["date_to"] = "Tanggal akhir harus format YYYY-MM-DD"
	} else if dateTo != nil {
		inclusiveDateTo = dateTo
		exclusiveEnd := dateTo.AddDate(0, 0, 1)
		input.DateTo = &exclusiveEnd
	}

	if input.DateFrom != nil && inclusiveDateTo != nil && inclusiveDateTo.Before(*input.DateFrom) {
		errors["date_to"] = "Tanggal akhir tidak boleh sebelum tanggal mulai"
	}
	return input, errors
}

func parseReportDate(raw string) (*time.Time, bool) {
	if raw == "" {
		return nil, true
	}
	value, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return nil, false
	}
	return &value, true
}

func revenueReportResponse(report *service.RevenueReport) RevenueReportResponse {
	if report == nil {
		return RevenueReportResponse{Currency: "IDR"}
	}
	return RevenueReportResponse{
		TotalRevenue: report.TotalRevenue,
		Currency:     report.Currency,
	}
}

func productsSoldReportResponse(report *service.ProductsSoldReport) ProductsSoldReportResponse {
	if report == nil {
		return ProductsSoldReportResponse{Items: []ProductSoldItemResponse{}}
	}
	items := make([]ProductSoldItemResponse, 0, len(report.Items))
	for _, item := range report.Items {
		items = append(items, ProductSoldItemResponse{
			ProductID:    item.ProductID,
			ProductName:  item.ProductName,
			QuantitySold: item.QuantitySold,
		})
	}
	return ProductsSoldReportResponse{
		TotalProductsSold: report.TotalProductsSold,
		Items:             items,
	}
}
