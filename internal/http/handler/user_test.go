package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cafeTelkom/internal/http/middleware"
	"cafeTelkom/internal/repository"
	"cafeTelkom/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestUserHandlerGetProfileReturnsAuthenticatedUser(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	userHandler := NewUserHandler(nil, "https://example.supabase.co")
	router.GET("/users/profile", func(c *gin.Context) {
		c.Set("authenticated_user", middleware.AuthenticatedUser{
			ID:          "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b",
			Email:       "user2@gmail.com",
			FullName:    "User Two",
			Role:        repository.UserRoleCUSTOMER,
			IsVerified:  true,
			IsActive:    true,
			PhoneNumber: "+628123456788",
			AvatarURL:   "https://example.supabase.co/storage/v1/object/public/avatars/user.png",
		})
		userHandler.GetProfile(c)
	})

	req := httptest.NewRequest(http.MethodGet, "/users/profile", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}

	body := resp.Body.String()
	for _, want := range []string{
		`"success":true`,
		`"id":"491f9f58-6f04-4c86-81d2-3fd58d2a4c1b"`,
		`"email":"user2@gmail.com"`,
		`"role":"CUSTOMER"`,
		`"is_verified":true`,
		`"is_active":true`,
		`"phone_number":"+628123456788"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response body missing %s: %s", want, body)
		}
	}
}

func TestValidateUpdateProfileRequest(t *testing.T) {
	fullName := "User Two"
	phoneNumber := "+628123456788"
	avatarURL := "https://example.supabase.co/storage/v1/object/public/avatars/user.png"
	shortName := "Us"
	invalidPhone := "08123"
	wrongHostAvatar := "https://evil.example.com/storage/v1/object/public/avatars/user.png"

	tests := []struct {
		name        string
		req         UpdateProfileRequest
		supabaseURL string
		wantErr     bool
	}{
		{
			name: "valid partial payload",
			req: UpdateProfileRequest{
				FullName:    &fullName,
				PhoneNumber: &phoneNumber,
				AvatarURL:   &avatarURL,
			},
			supabaseURL: "https://example.supabase.co",
			wantErr:     false,
		},
		{
			name: "short full name",
			req: UpdateProfileRequest{
				FullName: &shortName,
			},
			supabaseURL: "https://example.supabase.co",
			wantErr:     true,
		},
		{
			name: "invalid phone number",
			req: UpdateProfileRequest{
				PhoneNumber: &invalidPhone,
			},
			supabaseURL: "https://example.supabase.co",
			wantErr:     true,
		},
		{
			name: "avatar host must match supabase project",
			req: UpdateProfileRequest{
				AvatarURL: &wrongHostAvatar,
			},
			supabaseURL: "https://example.supabase.co",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			got := validateUpdateProfileRequest(tt.req, tt.supabaseURL)
			if tt.wantErr && len(got) == 0 {
				t.Fatalf("expected validation errors")
			}
			if !tt.wantErr && len(got) > 0 {
				t.Fatalf("did not expect errors, got %v", got)
			}
		})
	}
}

func TestUserHandlerUpdateProfileReturnsUpdatedProfile(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var userUUID pgtype.UUID
	if err := userUUID.Scan("491f9f58-6f04-4c86-81d2-3fd58d2a4c1b"); err != nil {
		t.Fatalf("scan uuid: %v", err)
	}

	router := gin.New()
	userHandler := NewUserHandler(fakeProfileUpdater{
		profile: &service.UserProfile{
			ID:          "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b",
			Email:       "user2@gmail.com",
			FullName:    "User Updated",
			PhoneNumber: "+628123456788",
			Role:        "CUSTOMER",
			IsVerified:  true,
			IsActive:    true,
		},
	}, "https://example.supabase.co")
	router.PATCH("/users/profile", func(c *gin.Context) {
		c.Set("authenticated_user", middleware.AuthenticatedUser{
			ID:       "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b",
			UUID:     userUUID,
			Role:     repository.UserRoleCUSTOMER,
			Email:    "user2@gmail.com",
			IsActive: true,
		})
		userHandler.UpdateProfile(c)
	})

	req := httptest.NewRequest(http.MethodPatch, "/users/profile", strings.NewReader(`{"full_name":" User Updated "}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
	if !strings.Contains(resp.Body.String(), `"full_name":"User Updated"`) {
		t.Fatalf("response body = %s", resp.Body.String())
	}
}

func TestUserHandlerGetProfileRequiresAuthenticatedUserContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	userHandler := NewUserHandler(nil, "https://example.supabase.co")
	router.GET("/users/profile", userHandler.GetProfile)

	req := httptest.NewRequest(http.MethodGet, "/users/profile", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d body = %s", resp.Code, resp.Body.String())
	}
}

type fakeProfileUpdater struct {
	profile *service.UserProfile
	err     error
}

func (f fakeProfileUpdater) UpdateProfile(context.Context, service.UpdateProfileInput) (*service.UserProfile, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.profile, nil
}
