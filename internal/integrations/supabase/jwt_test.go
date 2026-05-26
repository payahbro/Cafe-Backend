package supabase

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestJWTVerifierAcceptsValidSupabaseToken(t *testing.T) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	const kid = "test-key"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/auth/v1/.well-known/jwks.json" {
			t.Fatalf("unexpected jwks path: %s", r.URL.Path)
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"keys": []map[string]string{
				{
					"kty": "EC",
					"kid": kid,
					"alg": "ES256",
					"crv": "P-256",
					"x":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.X.Bytes()),
					"y":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.Y.Bytes()),
				},
			},
		})
	}))
	defer server.Close()

	token := signTestToken(t, privateKey, kid, map[string]interface{}{
		"iss":   server.URL + "/auth/v1",
		"sub":   "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b",
		"aud":   "authenticated",
		"exp":   time.Now().Add(time.Hour).Unix(),
		"iat":   time.Now().Unix(),
		"email": "user2@gmail.com",
		"role":  "authenticated",
	})

	claims, err := NewJWTVerifier(server.URL).Verify(context.Background(), token)
	if err != nil {
		t.Fatalf("verify token: %v", err)
	}

	if claims.Subject != "491f9f58-6f04-4c86-81d2-3fd58d2a4c1b" {
		t.Fatalf("subject = %q", claims.Subject)
	}
}

func signTestToken(t *testing.T, privateKey *ecdsa.PrivateKey, kid string, claims map[string]interface{}) string {
	t.Helper()

	header := map[string]interface{}{
		"alg": "ES256",
		"kid": kid,
		"typ": "JWT",
	}

	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}

	encodedHeader := base64.RawURLEncoding.EncodeToString(headerJSON)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := encodedHeader + "." + encodedClaims

	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, privateKey, digest[:])
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	signature := append(leftPad32(r), leftPad32(s)...)
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func leftPad32(value *big.Int) []byte {
	raw := value.Bytes()
	if len(raw) >= 32 {
		return raw
	}
	return append(make([]byte, 32-len(raw)), raw...)
}

func TestJWTVerifierRejectsMalformedToken(t *testing.T) {
	_, err := NewJWTVerifier("https://example.supabase.co").Verify(context.Background(), strings.Repeat("x", 10))
	if err == nil {
		t.Fatal("expected malformed token to be rejected")
	}
}
