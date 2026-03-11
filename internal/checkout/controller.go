package checkout

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/MatMassu/checkout-handler/internal/checkout/dto"
	"github.com/MatMassu/checkout-handler/internal/domain"
	"github.com/google/uuid"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) Cancel(w http.ResponseWriter, r *http.Request) {
	orderID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		http.Error(w, "invalid order id", http.StatusBadRequest)
		return
	}

	if err := c.service.CancelOrder(r.Context(), orderID); err != nil {
		log.Printf("cancel order %s: %v", orderID, err)
		http.Error(w, "failed to cancel order", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *Controller) Checkout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1 MB

	var req dto.CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.UserID == uuid.Nil {
		http.Error(w, "user_id is required", http.StatusBadRequest)
		return
	}
	if len(req.Items) == 0 {
		http.Error(w, "items cannot be empty", http.StatusBadRequest)
		return
	}
	for _, item := range req.Items {
		if item.ProductID == uuid.Nil {
			http.Error(w, "each item must have a valid product_id", http.StatusBadRequest)
			return
		}
		if item.Quantity < 1 {
			http.Error(w, "each item must have a quantity of at least 1", http.StatusBadRequest)
			return
		}
	}

	order, paymentURL, err := c.service.Checkout(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrProductNotFound):
			http.Error(w, "product not found", http.StatusUnprocessableEntity)
		case errors.Is(err, domain.ErrInsufficientStock):
			http.Error(w, "insufficient stock for one or more items", http.StatusConflict)
		default:
			log.Printf("checkout: %v", err)
			http.Error(w, "failed to create order", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(dto.CheckoutResponse{
		OrderID:    order.ID,
		Status:     order.Status,
		ExpiresAt:  order.ExpiresAt,
		PaymentURL: paymentURL,
	})
}
