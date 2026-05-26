package handler

import (
	"fmt"
	"net/mail"
	"regexp"
	"strings"

	"cafeTelkom/internal/http/dto"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
)

const (
	errCodeValidation        = "VALIDATION_ERROR"
	errCodeEmailAlreadyExist = "EMAIL_ALREADY_EXISTS"
	errCodeProfileNotSynced  = "PROFILE_NOT_SYNCED"
	errCodeInternal          = "INTERNAL_SERVER_ERROR"
	errCodeRateLimit         = "RATE_LIMIT_EXCEEDED"
)

var phoneRegex = regexp.MustCompile(`^\+[1-9]\d{1,14}$`)

type AuthHandler struct {
	authService *service.AuthService
}

type RegisterRequest struct {
	Email       string `json:"email"`
	Password    string `json:"password"`
	FullName    string `json:"full_name"`
	PhoneNumber string `json:"phone_number"`
}

type RegisterResponse struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	FullName    string  `json:"full_name"`
	PhoneNumber string  `json:"phone_number"`
	Role        string  `json:"role"`
	IsVerified  bool    `json:"is_verified"`
	IsActive    bool    `json:"is_active"`
	AvatarURL   *string `json:"avatar_url,omitempty"`
}

func NewAuthHandler(authService *service.AuthService) *AuthHandler {
	return &AuthHandler{authService: authService}
}

func (h *AuthHandler) Register(c *gin.Context) {
	if h.authService == nil {
		dto.WriteError(c, 500, errCodeInternal, "Auth service unavailable", nil)
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, 400, errCodeValidation, "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}

	if validationErrors := validateRegisterRequest(req); len(validationErrors) > 0 {
		dto.WriteError(c, 400, errCodeValidation, "Input tidak valid", validationErrors)
		return
	}

	profile, err := h.authService.Register(c.Request.Context(), service.RegisterInput{
		Email:       strings.TrimSpace(req.Email),
		Password:    req.Password,
		FullName:    strings.TrimSpace(req.FullName),
		PhoneNumber: strings.TrimSpace(req.PhoneNumber),
	})
	if err != nil {
		fmt.Printf("Register error: %v\n", err)
		switch {
		case err == service.ErrEmailAlreadyExists:
			dto.WriteError(c, 400, errCodeEmailAlreadyExist, "Email sudah terdaftar", nil)
		case err == service.ErrRateLimitExceeded:
			dto.WriteError(c, 429, errCodeRateLimit, "Limit pengiriman email Supabase tercapai, coba lagi nanti", nil)
		case err == service.ErrProfileNotSynced:
			dto.WriteError(c, 500, errCodeProfileNotSynced, "Profil tidak tersinkron", nil)
		default:
			dto.WriteError(c, 500, errCodeInternal, "Terjadi kesalahan internal", nil)
		}
		return
	}

	response := RegisterResponse{
		ID:          profile.ID,
		Email:       profile.Email,
		FullName:    profile.FullName,
		PhoneNumber: profile.PhoneNumber,
		Role:        profile.Role,
		IsVerified:  profile.IsVerified,
		IsActive:    profile.IsActive,
		AvatarURL:   profile.AvatarURL,
	}

	dto.WriteSuccess(c, 201, response, "Registrasi berhasil")
}

func validateRegisterRequest(req RegisterRequest) map[string]string {
	errors := make(map[string]string)

	email := strings.TrimSpace(req.Email)
	if email == "" {
		errors["email"] = "Email wajib diisi"
	} else if _, err := mail.ParseAddress(email); err != nil {
		errors["email"] = "Format email tidak valid"
	}

	if len(req.Password) < 8 {
		errors["password"] = "Password minimal 8 karakter"
	}

	fullName := strings.TrimSpace(req.FullName)
	if fullName == "" {
		errors["full_name"] = "Nama lengkap wajib diisi"
	} else if len(fullName) < 3 || len(fullName) > 50 {
		errors["full_name"] = "Nama lengkap harus 3-50 karakter"
	}

	phone := strings.TrimSpace(req.PhoneNumber)
	if phone == "" {
		errors["phone_number"] = "Nomor telepon wajib diisi"
	} else if !phoneRegex.MatchString(phone) {
		errors["phone_number"] = "Format nomor telepon harus E.164 (misal: +6281...)"
	}

	return errors
}
