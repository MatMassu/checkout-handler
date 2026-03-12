package dto

import "github.com/google/uuid"

type CheckoutItem struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int       `json:"quantity"`
}

type PayerInfo struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

type CheckoutRequest struct {
	UserID uuid.UUID      `json:"user_id"`
	Items  []CheckoutItem `json:"items"`
	Payer  PayerInfo      `json:"payer"`
}
