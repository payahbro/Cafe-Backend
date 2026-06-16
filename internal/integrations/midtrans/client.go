package midtrans

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	serverKey   string
	snapBaseURL string
	webhookURL  string
	httpClient  *http.Client
}

type TransactionRequest struct {
	OrderID     string
	GrossAmount int32
	Customer    CustomerDetails
	Items       []ItemDetails
}

type CustomerDetails struct {
	FirstName string
	Email     string
	Phone     string
}

type ItemDetails struct {
	ID       string
	Name     string
	Price    int32
	Quantity int32
}

type TransactionResponse struct {
	Token       string
	RedirectURL string
}

func NewClient(serverKey, snapBaseURL, webhookURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{
		serverKey:   serverKey,
		snapBaseURL: strings.TrimRight(snapBaseURL, "/"),
		webhookURL:  strings.TrimSpace(webhookURL),
		httpClient:  httpClient,
	}
}

func (c *Client) CreateTransaction(ctx context.Context, input TransactionRequest) (TransactionResponse, error) {
	if c == nil || c.httpClient == nil {
		return TransactionResponse{}, fmt.Errorf("midtrans client unavailable")
	}
	if strings.TrimSpace(c.serverKey) == "" {
		return TransactionResponse{}, fmt.Errorf("midtrans server key is empty")
	}
	if strings.TrimSpace(c.snapBaseURL) == "" {
		return TransactionResponse{}, fmt.Errorf("midtrans snap base url is empty")
	}

	payload := snapCreateTransactionRequest{
		TransactionDetails: snapTransactionDetails{
			OrderID:     input.OrderID,
			GrossAmount: input.GrossAmount,
		},
		CustomerDetails: snapCustomerDetails{
			FirstName: input.Customer.FirstName,
			Email:     input.Customer.Email,
			Phone:     input.Customer.Phone,
		},
		CreditCard: snapCreditCardDetails{Secure: true},
		Items:      make([]snapItemDetails, 0, len(input.Items)),
	}
	for _, item := range input.Items {
		payload.Items = append(payload.Items, snapItemDetails{
			ID:       item.ID,
			Name:     item.Name,
			Price:    item.Price,
			Quantity: item.Quantity,
		})
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return TransactionResponse{}, fmt.Errorf("marshal snap request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.snapBaseURL+"/snap/v1/transactions", bytes.NewReader(body))
	if err != nil {
		return TransactionResponse{}, fmt.Errorf("build snap request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(c.serverKey+":")))
	if c.webhookURL != "" {
		req.Header.Set("X-Append-Notification", c.webhookURL)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return TransactionResponse{}, fmt.Errorf("call snap transaction: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return TransactionResponse{}, fmt.Errorf("read snap response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return TransactionResponse{}, fmt.Errorf("snap transaction failed: status=%d body=%s", resp.StatusCode, string(raw))
	}

	var decoded snapCreateTransactionResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return TransactionResponse{}, fmt.Errorf("decode snap response: %w", err)
	}
	if strings.TrimSpace(decoded.RedirectURL) == "" {
		return TransactionResponse{}, fmt.Errorf("snap response missing redirect_url")
	}

	return TransactionResponse{
		Token:       decoded.Token,
		RedirectURL: decoded.RedirectURL,
	}, nil
}

func ComputeSignature(orderID, statusCode, grossAmount, serverKey string) string {
	sum := sha512.Sum512([]byte(orderID + statusCode + grossAmount + serverKey))
	return hex.EncodeToString(sum[:])
}

type snapCreateTransactionRequest struct {
	TransactionDetails snapTransactionDetails `json:"transaction_details"`
	CustomerDetails    snapCustomerDetails    `json:"customer_details,omitempty"`
	CreditCard         snapCreditCardDetails  `json:"credit_card,omitempty"`
	Items              []snapItemDetails      `json:"item_details,omitempty"`
}

type snapTransactionDetails struct {
	OrderID     string `json:"order_id"`
	GrossAmount int32  `json:"gross_amount"`
}

type snapCustomerDetails struct {
	FirstName string `json:"first_name,omitempty"`
	Email     string `json:"email,omitempty"`
	Phone     string `json:"phone,omitempty"`
}

type snapCreditCardDetails struct {
	Secure bool `json:"secure"`
}

type snapItemDetails struct {
	ID       string `json:"id,omitempty"`
	Name     string `json:"name"`
	Price    int32  `json:"price"`
	Quantity int32  `json:"quantity"`
}

type snapCreateTransactionResponse struct {
	Token       string `json:"token"`
	RedirectURL string `json:"redirect_url"`
}
