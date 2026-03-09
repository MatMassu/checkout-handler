package orders

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Order status constants — all valid states and their transitions:
//
//	pending → paid       (payment approved by MercadoPago webhook)
//	pending → failed     (payment rejected by MercadoPago webhook)
//	pending → cancelled  (explicit cancellation by user)
//	pending → expired    (expiry worker: expires_at passed with no payment)
//
// paid, failed, cancelled, and expired are terminal — no further transitions allowed.
const (
	StatusPending   = "pending"
	StatusPaid      = "paid"
	StatusFailed    = "failed"
	StatusCancelled = "cancelled"
	StatusExpired   = "expired"
)

var (
	ErrProductNotFound   = errors.New("product not found")
	ErrInsufficientStock = errors.New("insufficient stock")
)

type CheckoutItem struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int       `json:"quantity"`
}

type CheckoutRequest struct {
	UserID uuid.UUID      `json:"user_id"`
	Items  []CheckoutItem `json:"items"`
}

type CheckoutResponse struct {
	OrderID    uuid.UUID `json:"order_id"`
	Status     string    `json:"status"`
	ExpiresAt  time.Time `json:"expires_at"`
	PaymentURL string    `json:"payment_url"`
}

// PaymentStarter is implemented by payments.Service.
// Defined here to keep the dependency direction: payments imports orders, not the reverse.
type PaymentStarter interface {
	StartPayment(ctx context.Context, orderID uuid.UUID, expiresAt time.Time) (string, error)
}
