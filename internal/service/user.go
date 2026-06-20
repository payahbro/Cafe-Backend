package service

import (
	"context"
	"errors"
	"fmt"

	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type UserService struct {
	repo userRepository
}

type userRepository interface {
	ListUsers(ctx context.Context) ([]repository.ListUsersRow, error)
	UpdateUserProfile(ctx context.Context, arg repository.UpdateUserProfileParams) (repository.UpdateUserProfileRow, error)
}

type UpdateProfileInput struct {
	UserID      pgtype.UUID
	FullName    *string
	PhoneNumber *string
	AvatarURL   *string
}

func NewUserService(repo userRepository) *UserService {
	return &UserService{repo: repo}
}

func (s *UserService) ListUsers(ctx context.Context) ([]UserProfile, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	rows, err := s.repo.ListUsers(ctx)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}

	users := make([]UserProfile, 0, len(rows))
	for _, row := range rows {
		users = append(users, userProfileFromListRow(row))
	}
	return users, nil
}

func (s *UserService) UpdateProfile(ctx context.Context, input UpdateProfileInput) (*UserProfile, error) {
	if s.repo == nil {
		return nil, errors.New("database repository missing")
	}

	row, err := s.repo.UpdateUserProfile(ctx, repository.UpdateUserProfileParams{
		ID:          input.UserID,
		FullName:    optionalText(input.FullName),
		PhoneNumber: optionalText(input.PhoneNumber),
		AvatarUrl:   optionalText(input.AvatarURL),
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrProfileNotSynced
		}
		return nil, fmt.Errorf("update user profile: %w", err)
	}

	return userProfileFromRow(row), nil
}

func userProfileFromListRow(row repository.ListUsersRow) UserProfile {
	return UserProfile{
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

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
