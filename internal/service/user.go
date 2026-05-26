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
	repo repository.Querier
}

type UpdateProfileInput struct {
	UserID      pgtype.UUID
	FullName    *string
	PhoneNumber *string
	AvatarURL   *string
}

func NewUserService(repo repository.Querier) *UserService {
	return &UserService{repo: repo}
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

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}
