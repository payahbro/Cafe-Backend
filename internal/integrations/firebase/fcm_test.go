package firebase

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"cafeTelkom/internal/outbox"
)

func TestFCMClientSendsProductCreatedMessageToTopic(t *testing.T) {
	var authHeader string
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"projects/cafeflow-demo/messages/123"}`))
	}))
	defer server.Close()
	client := NewFCMClientWithTokenProvider("cafeflow-demo", "new-products", server.URL, server.Client(), staticTokenProvider("test-token"))

	err := client.SendProductCreated(context.Background(), outbox.ProductCreatedMessage{
		Type:              "product_created",
		ProductID:         "11111111-1111-4111-8111-111111111111",
		Name:              "Nasi Goreng",
		Category:          "snack",
		ImageURL:          "https://example.supabase.co/storage/v1/object/public/products/nasi-goreng.png",
		NotificationTitle: "Produk baru tersedia",
		NotificationBody:  "Nasi Goreng sudah tersedia.",
	})
	if err != nil {
		t.Fatalf("send product created: %v", err)
	}

	if authHeader != "Bearer test-token" {
		t.Fatalf("authorization header = %q", authHeader)
	}
	message := requestBody["message"].(map[string]any)
	if message["topic"] != "new-products" {
		t.Fatalf("topic = %q", message["topic"])
	}
	notification := message["notification"].(map[string]any)
	if notification["title"] != "Produk baru tersedia" {
		t.Fatalf("notification title = %q", notification["title"])
	}
	if notification["body"] != "Nasi Goreng sudah tersedia." {
		t.Fatalf("notification body = %q", notification["body"])
	}
	data := message["data"].(map[string]any)
	if data["type"] != "product_created" {
		t.Fatalf("data type = %q", data["type"])
	}
	if data["product_id"] != "11111111-1111-4111-8111-111111111111" {
		t.Fatalf("data product_id = %q", data["product_id"])
	}
	if data["name"] != "Nasi Goreng" {
		t.Fatalf("data name = %q", data["name"])
	}
	if data["category"] != "snack" {
		t.Fatalf("data category = %q", data["category"])
	}
	if data["image_url"] != "https://example.supabase.co/storage/v1/object/public/products/nasi-goreng.png" {
		t.Fatalf("data image_url = %q", data["image_url"])
	}
}

type staticTokenProvider string

func (s staticTokenProvider) AccessToken(ctx context.Context) (string, error) {
	return string(s), nil
}
