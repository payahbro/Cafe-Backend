package firebase

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"cafeTelkom/internal/outbox"
)

const firebaseMessagingScope = "https://www.googleapis.com/auth/firebase.messaging"

type TokenProvider interface {
	AccessToken(ctx context.Context) (string, error)
}

type FCMClient struct {
	projectID     string
	topic         string
	endpoint      string
	httpClient    *http.Client
	tokenProvider TokenProvider
}

func NewFCMClient(projectID, topic, credentialFile string, httpClient *http.Client) (*FCMClient, error) {
	tokenProvider, err := NewServiceAccountTokenProvider(credentialFile, httpClient)
	if err != nil {
		return nil, err
	}

	endpoint := fmt.Sprintf("https://fcm.googleapis.com/v1/projects/%s/messages:send", url.PathEscape(projectID))
	return NewFCMClientWithTokenProvider(projectID, topic, endpoint, httpClient, tokenProvider), nil
}

func NewFCMClientWithTokenProvider(projectID, topic, endpoint string, httpClient *http.Client, tokenProvider TokenProvider) *FCMClient {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	return &FCMClient{
		projectID:     projectID,
		topic:         topic,
		endpoint:      endpoint,
		httpClient:    httpClient,
		tokenProvider: tokenProvider,
	}
}

func (c *FCMClient) SendProductCreated(ctx context.Context, message outbox.ProductCreatedMessage) error {
	if c == nil {
		return fmt.Errorf("fcm client missing")
	}
	if c.projectID == "" {
		return fmt.Errorf("fcm project id missing")
	}
	if c.topic == "" {
		return fmt.Errorf("fcm topic missing")
	}
	if c.endpoint == "" {
		return fmt.Errorf("fcm endpoint missing")
	}
	if c.tokenProvider == nil {
		return fmt.Errorf("fcm token provider missing")
	}

	token, err := c.tokenProvider.AccessToken(ctx)
	if err != nil {
		return fmt.Errorf("get fcm access token: %w", err)
	}

	body := map[string]any{
		"message": map[string]any{
			"topic": c.topic,
			"notification": map[string]string{
				"title": message.NotificationTitle,
				"body":  message.NotificationBody,
			},
			"data": map[string]string{
				"type":       message.Type,
				"product_id": message.ProductID,
				"name":       message.Name,
				"category":   message.Category,
				"image_url":  message.ImageURL,
			},
		},
	}
	rawBody, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal fcm request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return fmt.Errorf("create fcm request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("send fcm request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("fcm request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	return nil
}

type serviceAccountFile struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

type ServiceAccountTokenProvider struct {
	clientEmail string
	privateKey  *rsa.PrivateKey
	tokenURI    string
	httpClient  *http.Client
	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
	now         func() time.Time
}

func NewServiceAccountTokenProvider(credentialFile string, httpClient *http.Client) (*ServiceAccountTokenProvider, error) {
	if credentialFile == "" {
		return nil, fmt.Errorf("firebase credential file missing")
	}
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	raw, err := os.ReadFile(credentialFile)
	if err != nil {
		return nil, fmt.Errorf("read firebase credential file: %w", err)
	}

	var account serviceAccountFile
	if err := json.Unmarshal(raw, &account); err != nil {
		return nil, fmt.Errorf("decode firebase credential file: %w", err)
	}
	if account.ClientEmail == "" {
		return nil, fmt.Errorf("firebase credential client_email missing")
	}
	if account.TokenURI == "" {
		account.TokenURI = "https://oauth2.googleapis.com/token"
	}

	privateKey, err := parseRSAPrivateKey(account.PrivateKey)
	if err != nil {
		return nil, err
	}

	return &ServiceAccountTokenProvider{
		clientEmail: account.ClientEmail,
		privateKey:  privateKey,
		tokenURI:    account.TokenURI,
		httpClient:  httpClient,
		now:         time.Now,
	}, nil
}

func (p *ServiceAccountTokenProvider) AccessToken(ctx context.Context) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.now()
	if p.accessToken != "" && now.Before(p.expiresAt.Add(-1*time.Minute)) {
		return p.accessToken, nil
	}

	assertion, err := p.signedAssertion(now)
	if err != nil {
		return "", err
	}

	form := url.Values{}
	form.Set("grant_type", "urn:ietf:params:oauth:grant-type:jwt-bearer")
	form.Set("assertion", assertion)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("create oauth token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request oauth token: %w", err)
	}
	defer resp.Body.Close()

	rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("oauth token request failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}

	var tokenResponse struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(rawBody, &tokenResponse); err != nil {
		return "", fmt.Errorf("decode oauth token response: %w", err)
	}
	if tokenResponse.AccessToken == "" {
		return "", fmt.Errorf("oauth token response missing access_token")
	}
	if tokenResponse.ExpiresIn <= 0 {
		tokenResponse.ExpiresIn = 3600
	}

	p.accessToken = tokenResponse.AccessToken
	p.expiresAt = now.Add(time.Duration(tokenResponse.ExpiresIn) * time.Second)
	return p.accessToken, nil
}

func (p *ServiceAccountTokenProvider) signedAssertion(now time.Time) (string, error) {
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}
	claims := map[string]any{
		"iss":   p.clientEmail,
		"scope": firebaseMessagingScope,
		"aud":   p.tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	}

	encodedHeader, err := encodeJWTPart(header)
	if err != nil {
		return "", err
	}
	encodedClaims, err := encodeJWTPart(claims)
	if err != nil {
		return "", err
	}

	unsigned := encodedHeader + "." + encodedClaims
	digest := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, p.privateKey, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign service account jwt: %w", err)
	}

	return unsigned + "." + base64.RawURLEncoding.EncodeToString(signature), nil
}

func encodeJWTPart(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal jwt part: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func parseRSAPrivateKey(rawPrivateKey string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(rawPrivateKey))
	if block == nil {
		return nil, fmt.Errorf("decode firebase private key pem")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse firebase private key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("firebase private key is not RSA")
	}
	return rsaKey, nil
}
