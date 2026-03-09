package payments

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

type Handler struct {
	Service *Service
}

func (h *Handler) Webhook(w http.ResponseWriter, r *http.Request) {
	var n WebhookNotification
	if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}

	err := h.Service.ProcessWebhook(
		r.Context(),
		r.Header.Get("x-signature"),
		r.Header.Get("x-request-id"),
		n,
	)
	if err != nil {
		if errors.Is(err, ErrInvalidSignature) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		// Return 500 so MercadoPago retries — appropriate for transient errors
		// (DB unavailable, MP API down). Logic errors (already processed orders)
		// are handled idempotently in ConfirmPayment and won't reach here as errors.
		log.Printf("webhook: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}
