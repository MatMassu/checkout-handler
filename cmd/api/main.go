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
	orders "github.com/MatMassu/checkout-handler/internal/orders"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	pool := db.NewPool()
	defer pool.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Services
	ordersService := orders.NewService(pool)
	ordersHandler := &orders.Handler{Service: ordersService}

	// Start expiry worker (checks every minute)
	go orders.NewExpiryWorker(pool, time.Minute).Run(ctx)

	// Routes
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", healthHandler(pool))
	mux.HandleFunc("POST /checkout", ordersHandler.Checkout)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	// Start server in background so we can listen for shutdown signals.
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
