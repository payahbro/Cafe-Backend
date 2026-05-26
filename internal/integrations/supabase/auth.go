package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type AuthClient struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

type AuthUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type AuthError struct {
	Status  int
	Code    string
	Message string
}

func (e *AuthError) Error() string {
	if e.Code == "" {
		return fmt.Sprintf("supabase auth error: status=%d message=%s", e.Status, e.Message)
	}
	return fmt.Sprintf("supabase auth error: status=%d code=%s message=%s", e.Status, e.Code, e.Message)
}

func NewAuthClient(baseURL, apiKey string) *AuthClient {
	cleanURL := strings.TrimRight(baseURL, "/")
	return &AuthClient{
		baseURL: cleanURL,
		apiKey:  apiKey,
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *AuthClient) SignUp(ctx context.Context, email, password string, data map[string]interface{}) (*AuthUser, error) {
	if c.baseURL == "" || c.apiKey == "" {
		return nil, errors.New("supabase auth client not configured")
	}

	payload := map[string]interface{}{
		"email":    email,
		"password": password,
		"data":     data,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal signup payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/auth/v1/signup", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build signup request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("apikey", c.apiKey)
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("signup request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return nil, decodeAuthError(resp)
	}

	var payloadResp struct {
		User *AuthUser `json:"user"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payloadResp); err != nil {
		return nil, fmt.Errorf("decode signup response: %w", err)
	}
	if payloadResp.User == nil {
		return nil, errors.New("supabase signup response missing user")
	}
	return payloadResp.User, nil
}

func decodeAuthError(resp *http.Response) error {
	var errResp struct {
		Code             string `json:"code"`
		ErrorCode        string `json:"error_code"`
		Error            string `json:"error"`
		Msg              string `json:"msg"`
		Message          string `json:"message"`
		ErrorDescription string `json:"error_description"`
	}

	_ = json.NewDecoder(resp.Body).Decode(&errResp)

	code := firstNonEmpty(errResp.Code, errResp.ErrorCode, errResp.Error)
	message := firstNonEmpty(errResp.Message, errResp.Msg, errResp.ErrorDescription)
	if message == "" {
		message = resp.Status
	}

	return &AuthError{
		Status:  resp.StatusCode,
		Code:    code,
		Message: message,
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
