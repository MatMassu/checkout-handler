package payment

import (
	"context"
	"fmt"
	"time"

	"github.com/MatMassu/checkout-handler/internal/payment/dto"
	"github.com/google/uuid"
)

type Service struct {
	db            DBRepository
	mp            MPRepository
	orders        OrderConfirmer
	webhookSecret string
}

func NewService(db DBRepository, mp MPRepository, orders OrderConfirmer, webhookSecret string) *Service {
	return &Service{db: db, mp: mp, orders: orders, webhookSecret: webhookSecret}
}

// StartPayment creates a MercadoPago preference and records the pending payment.
// Returns the checkout URL to redirect the user to.
func (s *Service) StartPayment(ctx context.Context, orderID uuid.UUID, amount int64, expiresAt time.Time) (string, error) {
	preferenceID, checkoutURL, err := s.mp.CreatePreference(ctx, orderID, amount, expiresAt)
	if err != nil {
		return "", fmt.Errorf("create preference: %w", err)
	}

	paymentID := uuid.New()
	if err := s.db.InsertPayment(ctx, paymentID, orderID, preferenceID, amount); err != nil {
		return "", fmt.Errorf("insert payment: %w", err)
	}

	return checkoutURL, nil
}

// ProcessWebhook validates the signature and handles a payment notification.
func (s *Service) ProcessWebhook(ctx context.Context, xSignature, xRequestID string, n dto.WebhookNotification) error {
	if err := validateSignature(xSignature, xRequestID, n.Data.ID, s.webhookSecret); err != nil {
		return err
	}

	if n.Type != "payment" {
		return nil
	}

	mpPaymentID, err := parsePaymentID(n.Data.ID)
	if err != nil {
		return fmt.Errorf("parse payment id: %w", err)
	}

	payment, err := s.mp.GetPayment(ctx, mpPaymentID)
	if err != nil {
		return fmt.Errorf("get payment: %w", err)
	}

	if err := s.db.UpdatePayment(ctx, payment); err != nil {
		return fmt.Errorf("update payment: %w", err)
	}

	if err := s.orders.ConfirmPayment(ctx, payment.OrderID, payment.Status); err != nil {
		return fmt.Errorf("confirm payment: %w", err)
	}

	return nil
}
