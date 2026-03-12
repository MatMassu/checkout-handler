package checkout

import (
	"context"
	"fmt"

	"github.com/MatMassu/checkout-handler/internal/checkout/dto"
	"github.com/MatMassu/checkout-handler/internal/domain"
	"github.com/google/uuid"
)

type Service struct {
	repo     Repository
	payments PaymentStarter
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// SetPayments breaks the initialization cycle between checkout.Service and payment.Service.
// Must be called before the server starts serving requests.
func (s *Service) SetPayments(p PaymentStarter) {
	s.payments = p
}

// Checkout consolidates items, creates the order with reserved stock, and initiates payment.
// Returns the created Order and the MercadoPago checkout URL.
func (s *Service) Checkout(ctx context.Context, req dto.CheckoutRequest) (domain.Order, string, error) {
	// Consolidate duplicate product IDs by summing quantities.
	quantities := make(map[uuid.UUID]int, len(req.Items))
	for _, item := range req.Items {
		quantities[item.ProductID] += item.Quantity
	}
	productIDs := make([]uuid.UUID, 0, len(quantities))
	for id := range quantities {
		productIDs = append(productIDs, id)
	}

	order, err := s.repo.CreateOrder(ctx, req.UserID, productIDs, quantities)
	if err != nil {
		return domain.Order{}, "", err
	}

	var paymentURL string
	if s.payments != nil {
		paymentURL, err = s.payments.StartPayment(ctx, order.ID, order.TotalAmount, order.ExpiresAt, req.Payer)
		if err != nil {
			return domain.Order{}, "", fmt.Errorf("start payment: %w", err)
		}
	}

	return order, paymentURL, nil
}

// CancelOrder transitions a pending order to cancelled, releasing its stock reservation.
// Safe to call on already-terminal orders — the repository no-ops in that case.
func (s *Service) CancelOrder(ctx context.Context, orderID uuid.UUID) error {
	return s.repo.ConfirmPayment(ctx, orderID, domain.StatusCancelled)
}

// ConfirmPayment is called by payment.Service via the OrderConfirmer interface.
// Maps the MercadoPago payment status to an order status and delegates to the repository.
func (s *Service) ConfirmPayment(ctx context.Context, orderID uuid.UUID, mpStatus string) error {
	var status string
	switch mpStatus {
	case "approved":
		status = domain.StatusPaid
	case "rejected", "cancelled":
		status = domain.StatusFailed
	default:
		// pending, in_process, in_mediation — no transition yet
		return nil
	}
	return s.repo.ConfirmPayment(ctx, orderID, status)
}
