package checkout

import (
	"context"
	"time"

	"github.com/MatMassu/checkout-handler/internal/domain"
	"github.com/google/uuid"
)

// Repository abstracts all persistence operations for the checkout feature.
// The postgres implementation owns transaction boundaries — each method is atomic.
type Repository interface {
	// CreateOrder locks product rows FOR UPDATE in sorted order (deadlock prevention),
	// validates available stock, inserts the order and order items, and increments
	// products.reserved. Returns the created Order with ID, TotalAmount, and ExpiresAt.
	CreateOrder(ctx context.Context, userID uuid.UUID, productIDs []uuid.UUID, quantities map[uuid.UUID]int) (domain.Order, error)

	// ConfirmPayment locks the order, transitions it from pending to the given status,
	// and adjusts stock: paid → decrement stock+reserved; failed → release reserved only.
	ConfirmPayment(ctx context.Context, orderID uuid.UUID, status string) error

	// FindExpiredOrderIDs returns IDs of pending orders past their expires_at.
	FindExpiredOrderIDs(ctx context.Context) ([]uuid.UUID, error)

	// ExpireOrder locks the order, releases its stock reservation, and sets status to expired.
	ExpireOrder(ctx context.Context, orderID uuid.UUID) error
}

// PaymentStarter is implemented by payment.Service.
// Defined here to keep the dependency direction: payment imports checkout, not the reverse.
type PaymentStarter interface {
	StartPayment(ctx context.Context, orderID uuid.UUID, amount int64, expiresAt time.Time) (string, error)
}
