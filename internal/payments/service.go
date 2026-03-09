package payments

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/MatMassu/checkout-handler/internal/orders"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db              *pgxpool.Pool
	client          *Client
	orders          *orders.Service
	webhookSecret   string
	notificationURL string
}

func NewService(
	db *pgxpool.Pool,
	client *Client,
	orders *orders.Service,
	webhookSecret string,
	notificationURL string,
) *Service {
	return &Service{
		db:              db,
		client:          client,
		orders:          orders,
		webhookSecret:   webhookSecret,
		notificationURL: notificationURL,
	}
}

// StartPayment creates a MercadoPago checkout preference for the given order,
// inserts a record into the payments table, and returns the checkout URL.
// The URL is the sandbox URL when the client is in sandbox mode, production otherwise.
func (s *Service) StartPayment(ctx context.Context, orderID uuid.UUID, expiresAt time.Time) (string, error) {
	var totalAmount int64
	err := s.db.QueryRow(ctx,
		`SELECT total_amount FROM orders WHERE id = $1`,
		orderID,
	).Scan(&totalAmount)
	if err != nil {
		return "", fmt.Errorf("fetch order total: %w", err)
	}

	pref, err := s.client.createPreference(ctx, preferenceRequest{
		Items: []preferenceItem{{
			Title:    "Vinilo Market",
			Quantity: 1,
			// Prices are stored as whole ARS pesos in the DB.
			UnitPrice: float64(totalAmount),
		}},
		BackURLs: preferenceBackURLs{
			Success: "https://vinilomarket.vercel.app/success",
			Failure: "https://vinilomarket.vercel.app/failure",
			Pending: "https://vinilomarket.vercel.app/pending",
		},
		AutoReturn:        "approved",
		NotificationURL:   s.notificationURL,
		ExternalReference: orderID.String(),
		Expires:           true,
		ExpirationDateTo:  expiresAt.Format(time.RFC3339),
	})
	if err != nil {
		return "", fmt.Errorf("create preference: %w", err)
	}

	_, err = s.db.Exec(ctx,
		`INSERT INTO payments (id, order_id, preference_id, status, amount)
		 VALUES ($1, $2, $3, 'pending', $4)`,
		uuid.New(), orderID, pref.ID, totalAmount,
	)
	if err != nil {
		return "", fmt.Errorf("insert payment record: %w", err)
	}

	if s.client.sandbox {
		return pref.SandboxInitPoint, nil
	}
	return pref.InitPoint, nil
}

// ProcessWebhook validates the request signature then handles the notification.
// Only "payment" type events trigger order transitions; all others are accepted and ignored.
func (s *Service) ProcessWebhook(ctx context.Context, xSig, xReqID string, n WebhookNotification) error {
	if err := validateSignature(xSig, xReqID, n.Data.ID, s.webhookSecret); err != nil {
		return err // caller checks for ErrInvalidSignature
	}

	if n.Type != "payment" {
		return nil
	}

	paymentID, err := strconv.ParseInt(n.Data.ID, 10, 64)
	if err != nil {
		return fmt.Errorf("parse payment id %q: %w", n.Data.ID, err)
	}

	payment, err := s.client.getPayment(ctx, paymentID)
	if err != nil {
		return fmt.Errorf("get payment %d: %w", paymentID, err)
	}

	orderID, err := uuid.Parse(payment.ExternalReference)
	if err != nil {
		return fmt.Errorf("parse external_reference %q: %w", payment.ExternalReference, err)
	}

	_, err = s.db.Exec(ctx,
		`UPDATE payments
		 SET mp_payment_id = $1, status = $2, status_detail = $3, updated_at = NOW()
		 WHERE order_id = $4`,
		payment.ID, payment.Status, payment.StatusDetail, orderID,
	)
	if err != nil {
		return fmt.Errorf("update payment record: %w", err)
	}

	return s.orders.ConfirmPayment(ctx, orderID, payment.Status)
}

// Ensure Service satisfies the orders.PaymentStarter interface at compile time.
var _ orders.PaymentStarter = (*Service)(nil)

// ErrInvalidSignature is re-exported so callers don't need to import this package
// internals — they can use errors.Is against payments.ErrInvalidSignature.
var _ = errors.Is // suppress unused import warning; ErrInvalidSignature is defined in webhook.go
