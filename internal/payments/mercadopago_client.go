package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const mpAPIBase = "https://api.mercadopago.com"

type Client struct {
	accessToken string
	sandbox     bool
	http        *http.Client
}

func NewClient(accessToken string, sandbox bool) *Client {
	return &Client{
		accessToken: accessToken,
		sandbox:     sandbox,
		http:        &http.Client{Timeout: 10 * time.Second},
	}
}

// preferenceItem represents a line item in a MercadoPago checkout preference.
// UnitPrice is in whole ARS pesos (matching the integer stored in products.price).
type preferenceItem struct {
	Title     string  `json:"title"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type preferenceBackURLs struct {
	Success string `json:"success"`
	Failure string `json:"failure"`
	Pending string `json:"pending"`
}

type preferenceRequest struct {
	Items             []preferenceItem   `json:"items"`
	BackURLs          preferenceBackURLs `json:"back_urls"`
	AutoReturn        string             `json:"auto_return"`
	NotificationURL   string             `json:"notification_url,omitempty"`
	ExternalReference string             `json:"external_reference"`
	Expires           bool               `json:"expires"`
	ExpirationDateTo  string             `json:"expiration_date_to"`
}

type preferenceResponse struct {
	ID               string `json:"id"`
	InitPoint        string `json:"init_point"`
	SandboxInitPoint string `json:"sandbox_init_point"`
}

func (c *Client) createPreference(ctx context.Context, req preferenceRequest) (preferenceResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return preferenceResponse{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		mpAPIBase+"/checkout/preferences", bytes.NewReader(body))
	if err != nil {
		return preferenceResponse{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.accessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return preferenceResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return preferenceResponse{}, fmt.Errorf("mercadopago: create preference: status %d: %s", resp.StatusCode, b)
	}

	var result preferenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return preferenceResponse{}, err
	}
	return result, nil
}

// MPPayment holds the fields we use from the MercadoPago payments API response.
type MPPayment struct {
	ID                int64   `json:"id"`
	Status            string  `json:"status"`
	StatusDetail      string  `json:"status_detail"`
	ExternalReference string  `json:"external_reference"`
	TransactionAmount float64 `json:"transaction_amount"`
}

func (c *Client) getPayment(ctx context.Context, paymentID int64) (MPPayment, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/payments/%d", mpAPIBase, paymentID), nil)
	if err != nil {
		return MPPayment{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.accessToken)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return MPPayment{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return MPPayment{}, fmt.Errorf("mercadopago: get payment %d: status %d: %s", paymentID, resp.StatusCode, b)
	}

	var result MPPayment
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return MPPayment{}, err
	}
	return result, nil
}
