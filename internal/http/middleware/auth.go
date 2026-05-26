package middleware

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"cafeTelkom/internal/http/dto"
	"cafeTelkom/internal/integrations/supabase"
	"cafeTelkom/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const (
	errCodeUnauthorized     = "UNAUTHORIZED"
	errCodeForbidden        = "FORBIDDEN"
	errCodeAccountDisabled  = "ACCOUNT_DISABLED"
	errCodeProfileNotSynced = "PROFILE_NOT_SYNCED"
	errCodeInternal         = "INTERNAL_SERVER_ERROR"

	authenticatedUserKey = "authenticated_user"
)

type TokenVerifier interface {
	Verify(ctx context.Context, token string) (*supabase.JWTClaims, error)
}

type UserLookup interface {
	GetUserById(ctx context.Context, id pgtype.UUID) (repository.GetUserByIdRow, error)
}

type AuthenticatedUser struct {
	ID          string
	UUID        pgtype.UUID
	Email       string
	FullName    string
	Role        repository.UserRole
	IsVerified  bool
	IsActive    bool
	PhoneNumber string
	AvatarURL   string
}

func AuthRequired(verifier TokenVerifier, users UserLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		token, ok := ExtractBearerToken(c.GetHeader("Authorization"))
		if !ok {
			writeAuthError(c, http.StatusUnauthorized, errCodeUnauthorized, "Token tidak ada atau tidak valid")
			return
		}
		if verifier == nil || users == nil {
			dto.WriteError(c, http.StatusInternalServerError, errCodeInternal, "Auth middleware belum terkonfigurasi", nil)
			c.Abort()
			return
		}

		claims, err := verifier.Verify(c.Request.Context(), token)
		if err != nil {
			writeAuthError(c, http.StatusUnauthorized, errCodeUnauthorized, "Token tidak ada atau tidak valid")
			return
		}

		var userID pgtype.UUID
		if err := userID.Scan(claims.Subject); err != nil {
			writeAuthError(c, http.StatusUnauthorized, errCodeUnauthorized, "Token tidak ada atau tidak valid")
			return
		}

		row, err := users.GetUserById(c.Request.Context(), userID)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				dto.WriteError(c, http.StatusInternalServerError, errCodeProfileNotSynced, "Profil tidak tersinkron", nil)
			} else {
				dto.WriteError(c, http.StatusInternalServerError, errCodeInternal, "Terjadi kesalahan internal", nil)
			}
			c.Abort()
			return
		}

		if !row.IsActive {
			dto.WriteError(c, http.StatusForbidden, errCodeAccountDisabled, "Akun dinonaktifkan", nil)
			c.Abort()
			return
		}

		c.Set(authenticatedUserKey, AuthenticatedUser{
			ID:          row.ID,
			UUID:        userID,
			Email:       row.Email,
			FullName:    row.FullName,
			Role:        row.Role,
			IsVerified:  row.IsVerified,
			IsActive:    row.IsActive,
			PhoneNumber: textValue(row.PhoneNumber),
			AvatarURL:   textValue(row.AvatarUrl),
		})
		c.Next()
	}
}

func RequireRoles(roles ...repository.UserRole) gin.HandlerFunc {
	allowed := make(map[repository.UserRole]struct{}, len(roles))
	for _, role := range roles {
		allowed[role] = struct{}{}
	}

	return func(c *gin.Context) {
		user, ok := GetAuthenticatedUser(c)
		if !ok {
			writeAuthError(c, http.StatusUnauthorized, errCodeUnauthorized, "Token tidak ada atau tidak valid")
			return
		}
		if _, ok := allowed[user.Role]; !ok {
			dto.WriteError(c, http.StatusForbidden, errCodeForbidden, "Role tidak diizinkan", nil)
			c.Abort()
			return
		}
		c.Next()
	}
}

func GetAuthenticatedUser(c *gin.Context) (AuthenticatedUser, bool) {
	value, ok := c.Get(authenticatedUserKey)
	if !ok {
		return AuthenticatedUser{}, false
	}
	user, ok := value.(AuthenticatedUser)
	return user, ok
}

func ExtractBearerToken(header string) (string, bool) {
	prefix, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(prefix, "Bearer") {
		return "", false
	}
	token = strings.TrimSpace(token)
	return token, token != ""
}

func writeAuthError(c *gin.Context, status int, code, message string) {
	dto.WriteError(c, status, code, message, nil)
	c.Abort()
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}
	return value.String
}
