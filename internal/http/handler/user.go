package handler

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"cafeTelkom/internal/http/dto"
	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
)

type UserHandler struct {
	userService userService
	supabaseURL string
}

type userService interface {
	ListUsers(ctx context.Context) ([]service.UserProfile, error)
	UpdateProfile(ctx context.Context, input service.UpdateProfileInput) (*service.UserProfile, error)
}

type UserProfileResponse struct {
	ID          string  `json:"id"`
	Email       string  `json:"email"`
	FullName    string  `json:"full_name"`
	Role        string  `json:"role"`
	IsVerified  bool    `json:"is_verified"`
	IsActive    bool    `json:"is_active"`
	PhoneNumber string  `json:"phone_number"`
	AvatarURL   *string `json:"avatar_url"`
}

type UpdateProfileRequest struct {
	FullName    *string `json:"full_name"`
	PhoneNumber *string `json:"phone_number"`
	AvatarURL   *string `json:"avatar_url"`
}

func NewUserHandler(userService userService, supabaseURL string) *UserHandler {
	return &UserHandler{userService: userService, supabaseURL: supabaseURL}
}

func (h *UserHandler) ListUsers(c *gin.Context) {
	if h.userService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "User service unavailable", nil)
		return
	}

	users, err := h.userService.ListUsers(c.Request.Context())
	if err != nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		return
	}

	responses := make([]UserProfileResponse, 0, len(users))
	for _, user := range users {
		responses = append(responses, profileResponse(&user))
	}
	dto.WriteSuccess(c, http.StatusOK, responses, "Daftar user berhasil diambil")
}

func (h *UserHandler) GetProfile(c *gin.Context) {
	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}

	dto.WriteSuccess(c, http.StatusOK, UserProfileResponse{
		ID:          user.ID,
		Email:       user.Email,
		FullName:    user.FullName,
		Role:        string(user.Role),
		IsVerified:  user.IsVerified,
		IsActive:    user.IsActive,
		PhoneNumber: user.PhoneNumber,
		AvatarURL:   optionalString(user.AvatarURL),
	}, "Profil berhasil diambil")
}

func (h *UserHandler) UpdateProfile(c *gin.Context) {
	user, ok := middleware.GetAuthenticatedUser(c)
	if !ok {
		dto.WriteError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Token tidak ada atau tidak valid", nil)
		return
	}
	if h.userService == nil {
		dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "User service unavailable", nil)
		return
	}

	var req UpdateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", map[string]string{
			"body": "Payload tidak valid",
		})
		return
	}

	req = normalizeUpdateProfileRequest(req)
	if validationErrors := validateUpdateProfileRequest(req, h.supabaseURL); len(validationErrors) > 0 {
		dto.WriteError(c, http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid", validationErrors)
		return
	}

	profile, err := h.userService.UpdateProfile(c.Request.Context(), service.UpdateProfileInput{
		UserID:      user.UUID,
		FullName:    req.FullName,
		PhoneNumber: req.PhoneNumber,
		AvatarURL:   req.AvatarURL,
	})
	if err != nil {
		switch err {
		case service.ErrProfileNotSynced:
			dto.WriteError(c, http.StatusInternalServerError, "PROFILE_NOT_SYNCED", "Profil tidak tersinkron", nil)
		default:
			dto.WriteError(c, http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "Terjadi kesalahan internal", nil)
		}
		return
	}

	dto.WriteSuccess(c, http.StatusOK, profileResponse(profile), "Profil berhasil diperbarui")
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func profileResponse(profile *service.UserProfile) UserProfileResponse {
	return UserProfileResponse{
		ID:          profile.ID,
		Email:       profile.Email,
		FullName:    profile.FullName,
		Role:        profile.Role,
		IsVerified:  profile.IsVerified,
		IsActive:    profile.IsActive,
		PhoneNumber: profile.PhoneNumber,
		AvatarURL:   profile.AvatarURL,
	}
}

func normalizeUpdateProfileRequest(req UpdateProfileRequest) UpdateProfileRequest {
	if req.FullName != nil {
		trimmed := strings.TrimSpace(*req.FullName)
		req.FullName = &trimmed
	}
	if req.PhoneNumber != nil {
		trimmed := strings.TrimSpace(*req.PhoneNumber)
		req.PhoneNumber = &trimmed
	}
	if req.AvatarURL != nil {
		trimmed := strings.TrimSpace(*req.AvatarURL)
		req.AvatarURL = &trimmed
	}
	return req
}

func validateUpdateProfileRequest(req UpdateProfileRequest, supabaseURL string) map[string]string {
	errors := make(map[string]string)

	if req.FullName != nil && (len(*req.FullName) < 3 || len(*req.FullName) > 50) {
		errors["full_name"] = "Nama lengkap harus 3-50 karakter"
	}
	if req.PhoneNumber != nil && !phoneRegex.MatchString(*req.PhoneNumber) {
		errors["phone_number"] = "Format nomor telepon harus E.164 (misal: +6281...)"
	}
	if req.AvatarURL != nil && !isSameSupabaseStorageURL(*req.AvatarURL, supabaseURL) {
		errors["avatar_url"] = "URL avatar harus berasal dari Supabase Storage project ini"
	}

	return errors
}

func isSameSupabaseStorageURL(rawAvatarURL, rawSupabaseURL string) bool {
	avatarURL, err := url.Parse(rawAvatarURL)
	if err != nil || avatarURL.Scheme != "https" || avatarURL.Host == "" {
		return false
	}

	supabaseURL, err := url.Parse(rawSupabaseURL)
	if err != nil || supabaseURL.Host == "" {
		return false
	}

	return strings.EqualFold(avatarURL.Host, supabaseURL.Host) &&
		strings.HasPrefix(avatarURL.Path, "/storage/v1/object/public/avatars/")
}
