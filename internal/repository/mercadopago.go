package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	checkoutdto "github.com/MatMassu/checkout-handler/internal/checkout/dto"
	"github.com/MatMassu/checkout-handler/internal/domain"
	"github.com/google/uuid"
)

const mpAPIBase = "https://api.mercadopago.com"

// MercadoPago implements payment.MPRepository.
type MercadoPago struct {
	accessToken     string
	sandbox         bool
	notificationURL string
	http            *http.Client
}

func NewMercadoPago(accessToken string, sandbox bool, notificationURL string) *MercadoPago {
	return &MercadoPago{
		accessToken:     accessToken,
		sandbox:         sandbox,
		notificationURL: notificationURL,
		http:            &http.Client{Timeout: 10 * time.Second},
	}
}

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

type preferencePayer struct {
	Email     string `json:"email,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

type preferenceRequest struct {
	Items             []preferenceItem   `json:"items"`
	BackURLs          preferenceBackURLs `json:"back_urls"`
	AutoReturn        string             `json:"auto_return"`
	NotificationURL   string             `json:"notification_url,omitempty"`
	ExternalReference string             `json:"external_reference"`
	Expires           bool               `json:"expires"`
	ExpirationDateTo  string             `json:"expiration_date_to"`
	Payer             preferencePayer    `json:"payer,omitempty"`
}

type preferenceResponse struct {
	ID               string `json:"id"`
	InitPoint        string `json:"init_point"`
	SandboxInitPoint string `json:"sandbox_init_point"`
}

// CreatePreference creates a MercadoPago checkout preference for the given order.
// Returns the preference ID and the appropriate checkout URL (sandbox or production).
func (r *MercadoPago) CreatePreference(ctx context.Context, orderID uuid.UUID, amount int64, expiresAt time.Time, payer checkoutdto.PayerInfo) (string, string, error) {
	req := preferenceRequest{
		Items: []preferenceItem{{
			Title:     "Vinilo Market",
			Quantity:  1,
			UnitPrice: float64(amount),
		}},
		BackURLs: preferenceBackURLs{
			Success: "https://vinilomarket.vercel.app/success",
			Failure: "https://vinilomarket.vercel.app/failure",
			Pending: "https://vinilomarket.vercel.app/pending",
		},
		AutoReturn:        "approved",
		NotificationURL:   r.notificationURL,
		ExternalReference: orderID.String(),
		Expires:           true,
		ExpirationDateTo:  expiresAt.Format(time.RFC3339),
		Payer: preferencePayer{
			Email:     payer.Email,
			FirstName: payer.FirstName,
			LastName:  payer.LastName,
		},
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		mpAPIBase+"/checkout/preferences", bytes.NewReader(body))
	if err != nil {
		return "", "", err
	}
	httpReq.Header.Set("Authorization", "Bearer "+r.accessToken)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := r.http.Do(httpReq)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return "", "", fmt.Errorf("mercadopago: create preference: status %d: %s", resp.StatusCode, b)
	}

	var result preferenceResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", "", err
	}

	checkoutURL := result.InitPoint
	if r.sandbox {
		checkoutURL = result.SandboxInitPoint
	}
	return result.ID, checkoutURL, nil
}

// mpPaymentResponse holds the fields we use from the MercadoPago payments API response.
type mpPaymentResponse struct {
	ID                int64  `json:"id"`
	Status            string `json:"status"`
	StatusDetail      string `json:"status_detail"`
	ExternalReference string `json:"external_reference"`
}

// GetPayment fetches a payment from the MercadoPago API by its numeric ID.
func (r *MercadoPago) GetPayment(ctx context.Context, paymentID int64) (domain.MPPayment, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		fmt.Sprintf("%s/v1/payments/%d", mpAPIBase, paymentID), nil)
	if err != nil {
		return domain.MPPayment{}, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+r.accessToken)

	resp, err := r.http.Do(httpReq)
	if err != nil {
		return domain.MPPayment{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return domain.MPPayment{}, fmt.Errorf("mercadopago: get payment %d: status %d: %s", paymentID, resp.StatusCode, b)
	}

	var raw mpPaymentResponse
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return domain.MPPayment{}, err
	}

	orderID, err := uuid.Parse(raw.ExternalReference)
	if err != nil {
		return domain.MPPayment{}, fmt.Errorf("mercadopago: parse external_reference %q: %w", raw.ExternalReference, err)
	}

	return domain.MPPayment{
		ID:           raw.ID,
		OrderID:      orderID,
		Status:       raw.Status,
		StatusDetail: raw.StatusDetail,
	}, nil
}
