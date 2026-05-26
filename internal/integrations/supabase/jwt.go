package supabase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrInvalidJWT = errors.New("invalid supabase jwt")

type JWTClaims struct {
	Subject   string
	Issuer    string
	Audience  []string
	ExpiresAt time.Time
	IssuedAt  time.Time
	Email     string
	Role      string
}

type JWTVerifier struct {
	issuer   string
	audience string
	jwksURL  string
	http     *http.Client
	now      func() time.Time

	mu        sync.RWMutex
	cachedKey map[string]*ecdsa.PublicKey
	cachedAt  time.Time
}

func NewJWTVerifier(baseURL string) *JWTVerifier {
	cleanURL := strings.TrimRight(baseURL, "/")
	return &JWTVerifier{
		issuer:   cleanURL + "/auth/v1",
		audience: "authenticated",
		jwksURL:  cleanURL + "/auth/v1/.well-known/jwks.json",
		http: &http.Client{
			Timeout: 10 * time.Second,
		},
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

func (v *JWTVerifier) Verify(ctx context.Context, token string) (*JWTClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrInvalidJWT
	}

	var header jwtHeader
	if err := decodeSegment(parts[0], &header); err != nil {
		return nil, ErrInvalidJWT
	}
	if header.Algorithm != "ES256" || header.KeyID == "" {
		return nil, ErrInvalidJWT
	}

	var rawClaims jwtPayload
	if err := decodeSegment(parts[1], &rawClaims); err != nil {
		return nil, ErrInvalidJWT
	}

	claims, err := v.validateClaims(rawClaims)
	if err != nil {
		return nil, err
	}

	publicKey, err := v.publicKey(ctx, header.KeyID)
	if err != nil {
		return nil, err
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrInvalidJWT
	}
	if !verifyES256(publicKey, []byte(parts[0]+"."+parts[1]), signature) {
		return nil, ErrInvalidJWT
	}

	return claims, nil
}

func (v *JWTVerifier) validateClaims(raw jwtPayload) (*JWTClaims, error) {
	audience := raw.Audience.Values()
	now := v.now()

	if raw.Subject == "" || raw.Issuer != v.issuer || !contains(audience, v.audience) {
		return nil, ErrInvalidJWT
	}
	if raw.ExpiresAt <= 0 || time.Unix(raw.ExpiresAt, 0).Before(now) {
		return nil, ErrInvalidJWT
	}

	issuedAt := time.Time{}
	if raw.IssuedAt > 0 {
		issuedAt = time.Unix(raw.IssuedAt, 0)
	}

	return &JWTClaims{
		Subject:   raw.Subject,
		Issuer:    raw.Issuer,
		Audience:  audience,
		ExpiresAt: time.Unix(raw.ExpiresAt, 0),
		IssuedAt:  issuedAt,
		Email:     raw.Email,
		Role:      raw.Role,
	}, nil
}

func (v *JWTVerifier) publicKey(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
	if key := v.cachedPublicKey(kid); key != nil {
		return key, nil
	}

	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}

	if key := v.cachedPublicKey(kid); key != nil {
		return key, nil
	}

	return nil, ErrInvalidJWT
}

func (v *JWTVerifier) cachedPublicKey(kid string) *ecdsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()

	if v.now().Sub(v.cachedAt) > 10*time.Minute {
		return nil
	}
	return v.cachedKey[kid]
}

func (v *JWTVerifier) refreshKeys(ctx context.Context) error {
	if v.jwksURL == "" {
		return ErrInvalidJWT
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("build jwks request: %w", err)
	}

	resp, err := v.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch jwks: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return ErrInvalidJWT
	}

	var payload jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return ErrInvalidJWT
	}

	keys := make(map[string]*ecdsa.PublicKey, len(payload.Keys))
	for _, key := range payload.Keys {
		publicKey, err := key.publicKey()
		if err != nil {
			continue
		}
		keys[key.KeyID] = publicKey
	}

	v.mu.Lock()
	v.cachedKey = keys
	v.cachedAt = v.now()
	v.mu.Unlock()

	return nil
}

func verifyES256(publicKey *ecdsa.PublicKey, signingInput, signature []byte) bool {
	if len(signature) != 64 {
		return false
	}

	digest := sha256.Sum256(signingInput)
	r := new(big.Int).SetBytes(signature[:32])
	s := new(big.Int).SetBytes(signature[32:])
	return ecdsa.Verify(publicKey, digest[:], r, s)
}

func decodeSegment(segment string, target interface{}) error {
	raw, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type jwtHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
	Type      string `json:"typ"`
}

type jwtPayload struct {
	Issuer    string      `json:"iss"`
	Subject   string      `json:"sub"`
	Audience  jwtAudience `json:"aud"`
	ExpiresAt int64       `json:"exp"`
	IssuedAt  int64       `json:"iat"`
	Email     string      `json:"email"`
	Role      string      `json:"role"`
}

type jwtAudience []string

func (a *jwtAudience) UnmarshalJSON(raw []byte) error {
	var one string
	if err := json.Unmarshal(raw, &one); err == nil {
		*a = []string{one}
		return nil
	}

	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

func (a jwtAudience) Values() []string {
	return []string(a)
}

type jwksResponse struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Algorithm string `json:"alg"`
	Curve     string `json:"crv"`
	X         string `json:"x"`
	Y         string `json:"y"`
}

func (k jwkKey) publicKey() (*ecdsa.PublicKey, error) {
	if k.KeyType != "EC" || k.Curve != "P-256" || k.KeyID == "" {
		return nil, ErrInvalidJWT
	}

	xBytes, err := base64.RawURLEncoding.DecodeString(k.X)
	if err != nil {
		return nil, ErrInvalidJWT
	}
	yBytes, err := base64.RawURLEncoding.DecodeString(k.Y)
	if err != nil {
		return nil, ErrInvalidJWT
	}

	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	curve := elliptic.P256()
	if !curve.IsOnCurve(x, y) {
		return nil, ErrInvalidJWT
	}

	return &ecdsa.PublicKey{Curve: curve, X: x, Y: y}, nil
}
