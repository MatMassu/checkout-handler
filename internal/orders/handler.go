package orders

import (
	"encoding/json"
	"errors"
	"net/http"
)

type Handler struct {
	Service *Service
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

	resp := CheckoutResponse{
		OrderID:   orderID,
		Status:    "pending",
		ExpiresAt: expiresAt,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(resp)
}
