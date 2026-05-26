package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"cafeTelkom/internal/integrations/supabase"
	"cafeTelkom/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

func TestExtractBearerToken(t *testing.T) {
	token, ok := ExtractBearerToken("Bearer abc.def.ghi")
	if !ok {
		t.Fatal("expected bearer token")
	}
	if token != "abc.def.ghi" {
		t.Fatalf("token = %q", token)
	}

	_, ok = ExtractBearerToken("Basic abc")
	if ok {
		t.Fatal("expected non-bearer auth header to be rejected")
	}
}

func TestAuthRequiredRejectsMissingBearerToken(t *testing.T) {
	response := runAuthMiddlewareTest(t, nil, nil, "")

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestAuthRequiredRejectsDisabledAccount(t *testing.T) {
	verifier := fakeTokenVerifier{
		claims: &supabase.JWTClaims{Subject: "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b"},
	}
	users := fakeUserLookup{
		row: repository.GetUserByIdRow{
			ID:       "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b",
			Email:    "user2@gmail.com",
			Role:     repository.UserRoleCUSTOMER,
			IsActive: false,
		},
	}

	response := runAuthMiddlewareTest(t, verifier, users, "Bearer valid-token")

	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d", response.Code)
	}
}

func TestAuthRequiredStoresActiveUserInContext(t *testing.T) {
	verifier := fakeTokenVerifier{
		claims: &supabase.JWTClaims{Subject: "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b"},
	}
	users := fakeUserLookup{
		row: repository.GetUserByIdRow{
			ID:         "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b",
			Email:      "user2@gmail.com",
			FullName:   "User Two",
			Role:       repository.UserRoleCUSTOMER,
			IsVerified: true,
			IsActive:   true,
			PhoneNumber: pgtype.Text{
				String: "+628123456788",
				Valid:  true,
			},
		},
	}

	response := runAuthMiddlewareTest(t, verifier, users, "Bearer valid-token")

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if response.Body.String() != "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b" {
		t.Fatalf("body = %q", response.Body.String())
	}
}

func TestAuthRequiredReturnsProfileNotSyncedWhenUserMissing(t *testing.T) {
	verifier := fakeTokenVerifier{
		claims: &supabase.JWTClaims{Subject: "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b"},
	}
	users := fakeUserLookup{err: pgx.ErrNoRows}

	response := runAuthMiddlewareTest(t, verifier, users, "Bearer valid-token")

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", response.Code)
	}
}

func runAuthMiddlewareTest(t *testing.T, verifier TokenVerifier, users UserLookup, authorization string) *httptest.ResponseRecorder {
	t.Helper()

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.GET("/protected", AuthRequired(verifier, users), func(c *gin.Context) {
		user, ok := GetAuthenticatedUser(c)
		if !ok {
			t.Fatal("authenticated user missing from context")
		}
		c.String(http.StatusOK, user.ID)
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	if authorization != "" {
		req.Header.Set("Authorization", authorization)
	}

	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	return response
}

type fakeTokenVerifier struct {
	claims *supabase.JWTClaims
	err    error
}

func (f fakeTokenVerifier) Verify(context.Context, string) (*supabase.JWTClaims, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.claims == nil {
		return nil, errors.New("missing claims")
	}
	return f.claims, nil
}

type fakeUserLookup struct {
	row repository.GetUserByIdRow
	err error
}

func (f fakeUserLookup) GetUserById(context.Context, pgtype.UUID) (repository.GetUserByIdRow, error) {
	if f.err != nil {
		return repository.GetUserByIdRow{}, f.err
	}
	return f.row, nil
}
