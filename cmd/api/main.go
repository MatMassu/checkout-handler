package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	db "github.com/MatMassu/checkout-handler/internal/database"
	"github.com/MatMassu/checkout-handler/internal/orders"
	"github.com/MatMassu/checkout-handler/internal/payments"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	pool := db.NewPool()
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Orders
	ordersService := orders.NewService(pool)
	go orders.NewExpiryWorker(pool, time.Minute).Run(ctx)

	// Payments
	mpSandbox := os.Getenv("MERCADOPAGO_SANDBOX") == "true"
	mpClient := payments.NewClient(os.Getenv("MERCADOPAGO_ACCESS_TOKEN"), mpSandbox)
	paymentsService := payments.NewService(
		pool,
		mpClient,
		ordersService,
		os.Getenv("MERCADOPAGO_WEBHOOK_SECRET"),
		os.Getenv("MERCADOPAGO_NOTIFICATION_URL"),
	)

	// Handlers
	ordersHandler := &orders.Handler{
		Service:  ordersService,
		Payments: paymentsService,
	}
	paymentsHandler := &payments.Handler{Service: paymentsService}

	// Routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(pool))
	mux.HandleFunc("POST /checkout", ordersHandler.Checkout)
	mux.HandleFunc("POST /payments/webhook", paymentsHandler.Webhook)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	go func() {
		log.Printf("Server running on :%s", port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down...")
	cancel()

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("server shutdown error: %v", err)
	}
}

func healthHandler(pool *pgxpool.Pool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		var now time.Time
		if err := pool.QueryRow(ctx, "SELECT NOW()").Scan(&now); err != nil {
			http.Error(w, "database unavailable", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"status": "ok",
			"time":   now,
		})
	}
}
