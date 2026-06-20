package service

import (
	"context"
	"testing"

	"cafeTelkom/internal/repository"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestUserServiceListUsersMapsRows(t *testing.T) {
	avatarURL := "https://example.supabase.co/storage/v1/object/public/avatars/admin.png"
	repo := &fakeUserRepo{
		users: []repository.ListUsersRow{
			{
				ID:          "11111111-1111-4111-8111-111111111111",
				Email:       "admin@cafe.test",
				FullName:    "Admin Cafe",
				Role:        repository.UserRoleADMIN,
				IsVerified:  true,
				IsActive:    true,
				AvatarUrl:   pgtype.Text{String: avatarURL, Valid: true},
				PhoneNumber: pgtype.Text{String: "+628111111111", Valid: true},
			},
			{
				ID:         "22222222-2222-4222-8222-222222222222",
				Email:      "staff@cafe.test",
				FullName:   "Staff Cafe",
				Role:       repository.UserRolePEGAWAI,
				IsVerified: true,
				IsActive:   true,
			},
		},
	}

	service := NewUserService(repo)
	users, err := service.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("list users: %v", err)
	}

	if !repo.listCalled {
		t.Fatalf("expected repository to be called")
	}
	if len(users) != 2 {
		t.Fatalf("users len = %d", len(users))
	}
	if users[0].ID != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("id = %q", users[0].ID)
	}
	if users[0].Role != "ADMIN" {
		t.Fatalf("role = %q", users[0].Role)
	}
	if users[0].PhoneNumber != "+628111111111" {
		t.Fatalf("phone number = %q", users[0].PhoneNumber)
	}
	if users[0].AvatarURL == nil || *users[0].AvatarURL != avatarURL {
		t.Fatalf("avatar url = %v", users[0].AvatarURL)
	}
	if users[1].AvatarURL != nil {
		t.Fatalf("expected nil avatar url, got %v", users[1].AvatarURL)
	}
}

type fakeUserRepo struct {
	users      []repository.ListUsersRow
	listCalled bool
	err        error
}

func (f *fakeUserRepo) ListUsers(ctx context.Context) ([]repository.ListUsersRow, error) {
	f.listCalled = true
	if f.err != nil {
		return nil, f.err
	}
	return f.users, nil
}

func (f *fakeUserRepo) UpdateUserProfile(ctx context.Context, arg repository.UpdateUserProfileParams) (repository.UpdateUserProfileRow, error) {
	return repository.UpdateUserProfileRow{}, nil
}
