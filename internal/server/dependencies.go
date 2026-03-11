package server

import (
	"github.com/MatMassu/checkout-handler/internal/checkout"
	"github.com/MatMassu/checkout-handler/internal/payment"
	"github.com/MatMassu/checkout-handler/internal/repository"
	"github.com/jackc/pgx/v5/pgxpool"
)

type deps struct {
	checkoutController *checkout.Controller
	paymentController  *payment.Controller
	expiryWorker       *checkout.ExpiryWorker
}

func buildDeps(db *pgxpool.Pool, cfg config) deps {
	repo := repository.NewPostgres(db)
	mpRepo := repository.NewMercadoPago(cfg.mpAccessToken, cfg.mpSandbox, cfg.mpNotificationURL)

	checkoutSvc := checkout.NewService(repo)
	paymentSvc := payment.NewService(repo, mpRepo, checkoutSvc, cfg.mpWebhookSecret)

	// Break the initialization cycle: checkout.Service needs payment.Service
	// only through the PaymentStarter interface; set it after both are created.
	checkoutSvc.SetPayments(paymentSvc)

	return deps{
		checkoutController: checkout.NewController(checkoutSvc),
		paymentController:  payment.NewController(paymentSvc),
		expiryWorker:       checkout.NewExpiryWorker(repo, cfg.expiryInterval),
	}
}
