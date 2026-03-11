package dto

import "github.com/google/uuid"

type CheckoutItem struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int       `json:"quantity"`
}

type CheckoutRequest struct {
	UserID uuid.UUID      `json:"user_id"`
	Items  []CheckoutItem `json:"items"`
}
