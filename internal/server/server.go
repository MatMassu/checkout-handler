package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	database "github.com/MatMassu/checkout-handler/internal/database"
	"github.com/MatMassu/checkout-handler/internal/middleware"
)

type config struct {
	port              string
	mpAccessToken     string
	mpWebhookSecret   string
	mpNotificationURL string
	mpSandbox         bool
	expiryInterval    time.Duration
}

func loadConfig() config {
	sandbox, _ := strconv.ParseBool(os.Getenv("MERCADOPAGO_SANDBOX"))
	return config{
		port:              envOrDefault("PORT", "8080"),
		mpAccessToken:     os.Getenv("MERCADOPAGO_ACCESS_TOKEN"),
		mpWebhookSecret:   os.Getenv("MERCADOPAGO_WEBHOOK_SECRET"),
		mpNotificationURL: os.Getenv("MERCADOPAGO_NOTIFICATION_URL"),
		mpSandbox:         sandbox,
		expiryInterval:    time.Minute,
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Start wires the application and blocks until a shutdown signal is received.
func Start() error {
	cfg := loadConfig()

	pool := database.NewPool()
	defer pool.Close()

	d := buildDeps(pool, cfg)

	mux := http.NewServeMux()
	registerRoutes(mux, d)

	handler := middleware.CORS(envOrDefault("ALLOWED_ORIGIN", "https://vinilomarket.vercel.app"))(
		middleware.Recovery(
			middleware.Logging(mux),
		),
	)

	srv := &http.Server{
		Addr:    ":" + cfg.port,
		Handler: handler,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		log.Printf("server listening on :%s", cfg.port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("server error: %v", err)
		}
	}()

	go d.expiryWorker.Run(ctx)

	<-ctx.Done()
	log.Println("shutting down...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
