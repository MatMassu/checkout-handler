package payment

import (
	"context"
	"time"

	"github.com/MatMassu/checkout-handler/internal/checkout/dto"
	"github.com/MatMassu/checkout-handler/internal/domain"
	"github.com/google/uuid"
)

// OrderConfirmer is implemented by checkout.Service.
// Defined here to keep the dependency direction: payment imports checkout, not the reverse.
type OrderConfirmer interface {
	ConfirmPayment(ctx context.Context, orderID uuid.UUID, mpStatus string) error
}

// DBRepository handles payment record persistence.
type DBRepository interface {
	InsertPayment(ctx context.Context, paymentID uuid.UUID, orderID uuid.UUID, preferenceID string, amount int64) error
	UpdatePayment(ctx context.Context, p domain.MPPayment) error
}

// MPRepository handles MercadoPago API interactions.
type MPRepository interface {
	// CreatePreference creates a checkout preference and returns its ID and the checkout URL.
	// The URL is the sandbox URL or production URL depending on repository configuration.
	CreatePreference(ctx context.Context, orderID uuid.UUID, amount int64, expiresAt time.Time, payer dto.PayerInfo) (preferenceID string, checkoutURL string, err error)

	// GetPayment fetches a payment from the MercadoPago API by its numeric ID.
	GetPayment(ctx context.Context, paymentID int64) (domain.MPPayment, error)
}
