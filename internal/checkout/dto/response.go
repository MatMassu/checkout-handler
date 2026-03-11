package dto

import (
	"time"

	"github.com/google/uuid"
)

type CheckoutResponse struct {
	OrderID    uuid.UUID `json:"order_id"`
	Status     string    `json:"status"`
	ExpiresAt  time.Time `json:"expires_at"`
	PaymentURL string    `json:"payment_url"`
}
