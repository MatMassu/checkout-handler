package domain

import "github.com/google/uuid"

// MPPayment is a domain value object representing a MercadoPago payment,
// translated from the raw API response by the repository layer.
type MPPayment struct {
	ID           int64
	OrderID      uuid.UUID
	Status       string
	StatusDetail string
}
