package payment

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"

	"github.com/MatMassu/checkout-handler/internal/payment/dto"
)

type Controller struct {
	service *Service
}

func NewController(service *Service) *Controller {
	return &Controller{service: service}
}

func (c *Controller) Webhook(w http.ResponseWriter, r *http.Request) {
	var n dto.WebhookNotification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	xSignature := r.Header.Get("x-signature")
	xRequestID := r.Header.Get("x-request-id")

	err := c.service.ProcessWebhook(r.Context(), xSignature, xRequestID, n)
	if err != nil {
		if errors.Is(err, ErrInvalidSignature) {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		log.Printf("webhook: %v", err)
		http.Error(w, "failed to process webhook", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
