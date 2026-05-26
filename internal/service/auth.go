package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cafeTelkom/internal/integrations/supabase"
	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrEmailAlreadyExists = errors.New("email already exists")
	ErrProfileNotSynced   = errors.New("profile not synced")
	ErrRateLimitExceeded  = errors.New("rate limit exceeded")
)

type AuthService struct {
	supabase *supabase.AuthClient
	repo     repository.Querier
}

type RegisterInput struct {
	Email       string
	Password    string
	FullName    string
	PhoneNumber string
}

type UserProfile struct {
	ID          string
	Email       string
	FullName    string
	PhoneNumber string
	Role        string
	IsVerified  bool
	IsActive    bool
	AvatarURL   *string
}

func NewAuthService(supabaseClient *supabase.AuthClient, repo repository.Querier) *AuthService {
	return &AuthService{supabase: supabaseClient, repo: repo}
}

func (s *AuthService) Register(ctx context.Context, input RegisterInput) (*UserProfile, error) {
	if s.supabase == nil {
		return nil, errors.New("supabase auth client missing")
	}
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	user, err := s.supabase.SignUp(ctx, input.Email, input.Password, map[string]interface{}{
		"full_name":    input.FullName,
		"phone_number": input.PhoneNumber,
	})
	if err != nil {
		if authErr := parseSupabaseAuthError(err); authErr != nil {
			if isEmailExistsCode(authErr.Code) {
				return nil, ErrEmailAlreadyExists
			}
			if authErr.Code == "over_email_send_rate_limit" {
				return nil, ErrRateLimitExceeded
			}
		}
		return nil, err
	}

	profile, err := s.fetchUserProfileWithRetry(ctx, user.ID)
	if err != nil {
		return nil, err
	}

	return profile, nil
}

func (s *AuthService) fetchUserProfileWithRetry(ctx context.Context, userID string) (*UserProfile, error) {
	const maxAttempts = 3
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		profile, err := s.fetchUserProfile(ctx, userID)
		if err == nil {
			return profile, nil
		}
		if !errors.Is(err, ErrProfileNotSynced) {
			return nil, err
		}
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(120 * time.Millisecond):
			}
		}
	}
	return nil, ErrProfileNotSynced
}

func (s *AuthService) fetchUserProfile(ctx context.Context, userID string) (*UserProfile, error) {
	var userUUID pgtype.UUID
	if err := userUUID.Scan(userID); err != nil {
		return nil, fmt.Errorf("invalid user id format: %w", err)
	}

	row, err := s.repo.GetUserById(ctx, userUUID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProfileNotSynced
		}
		return nil, fmt.Errorf("fetch user profile: %w", err)
	}

	return userProfileFromRow(row), nil
}

func userProfileFromRow(row repository.GetUserByIdRow) *UserProfile {
	return &UserProfile{
		ID:          row.ID,
		Email:       row.Email,
		FullName:    row.FullName,
		PhoneNumber: textOrEmpty(row.PhoneNumber),
		Role:        string(row.Role),
		IsVerified:  row.IsVerified,
		IsActive:    row.IsActive,
		AvatarURL:   textPtr(row.AvatarUrl),
	}
}

func parseSupabaseAuthError(err error) *supabase.AuthError {
	if err == nil {
		return nil
	}
	var authErr *supabase.AuthError
	if errors.As(err, &authErr) {
		return authErr
	}
	return nil
}

func isEmailExistsCode(code string) bool {
	normalized := strings.ToLower(code)
	switch normalized {
	case "email_exists", "user_already_registered", "email_already_exists":
		return true
	default:
		return false
	}
}

func textPtr(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func textOrEmpty(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
