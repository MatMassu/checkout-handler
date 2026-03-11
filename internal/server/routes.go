package server

import "net/http"

func registerRoutes(mux *http.ServeMux, d deps) {
	mux.HandleFunc("POST /checkout", d.checkoutController.Checkout)
	mux.HandleFunc("POST /orders/{id}/cancel", d.checkoutController.Cancel)
	mux.HandleFunc("POST /payments/webhook", d.paymentController.Webhook)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}
