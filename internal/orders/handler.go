package orders

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type Handler struct {
	Service  *Service
	Payments PaymentStarter
}

func (h *Handler) Checkout(w http.ResponseWriter, r *http.Request) {
	var req CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if len(req.Items) == 0 {
		http.Error(w, "items cannot be empty", http.StatusBadRequest)
		return
	}

	orderID, expiresAt, err := h.Service.CreateOrder(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrProductNotFound):
			http.Error(w, "product not found", http.StatusUnprocessableEntity)
		case errors.Is(err, ErrInsufficientStock):
			http.Error(w, "insufficient stock for one or more items", http.StatusConflict)
		default:
			http.Error(w, "failed to create order", http.StatusInternalServerError)
		}
		return
	}

	var paymentURL string
	if h.Payments != nil {
		paymentURL, err = h.Payments.StartPayment(r.Context(), orderID, expiresAt)
		if err != nil {
			// Order exists with reserved stock; expiry job will clean up in 10 minutes.
			log.Printf("checkout: start payment for order %s: %v", orderID, err)
			http.Error(w, "failed to initiate payment", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(CheckoutResponse{
		OrderID:    orderID,
		Status:     StatusPending,
		ExpiresAt:  expiresAt,
		PaymentURL: paymentURL,
	})
}
