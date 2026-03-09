package orders

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	db *pgxpool.Pool
}

func NewService(db *pgxpool.Pool) *Service {
	return &Service{db: db}
}

func (s *Service) CreateOrder(ctx context.Context, req CheckoutRequest) (uuid.UUID, time.Time, error) {
	// Consolidate duplicate product IDs and sort for consistent lock ordering
	// (sorted lock order prevents deadlocks with concurrent transactions)
	quantities := make(map[uuid.UUID]int, len(req.Items))
	for _, item := range req.Items {
		quantities[item.ProductID] += item.Quantity
	}
	productIDs := make([]uuid.UUID, 0, len(quantities))
	for id := range quantities {
		productIDs = append(productIDs, id)
	}
	sort.Slice(productIDs, func(i, j int) bool {
		return productIDs[i].String() < productIDs[j].String()
	})

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return uuid.Nil, time.Time{}, err
	}
	defer tx.Rollback(ctx)

	// Lock product rows in consistent order, validate stock, collect prices
	prices := make(map[uuid.UUID]int64, len(productIDs))
	for _, productID := range productIDs {
		var price int64
		var stock, reserved int
		err := tx.QueryRow(ctx,
			`SELECT price, stock, reserved FROM products WHERE id = $1 FOR UPDATE`,
			productID,
		).Scan(&price, &stock, &reserved)
		if err == pgx.ErrNoRows {
			return uuid.Nil, time.Time{}, ErrProductNotFound
		}
		if err != nil {
			return uuid.Nil, time.Time{}, err
		}
		if stock-reserved < quantities[productID] {
			return uuid.Nil, time.Time{}, ErrInsufficientStock
		}
		prices[productID] = price
	}

	// Compute subtotal
	var subtotal int64
	for productID, qty := range quantities {
		subtotal += prices[productID] * int64(qty)
	}

	// Insert order, retrieve expires_at from DB clock
	orderID := uuid.New()
	var expiresAt time.Time
	err = tx.QueryRow(ctx,
		`INSERT INTO orders (id, user_id, status, currency, subtotal_amount, tax_amount, total_amount, expires_at)
		 VALUES ($1, $2, 'pending', 'ARS', $3, 0, $3, NOW() + interval '10 minutes')
		 RETURNING expires_at`,
		orderID, req.UserID, subtotal,
	).Scan(&expiresAt)
	if err != nil {
		return uuid.Nil, time.Time{}, err
	}

	// Insert order items and reserve stock
	for _, productID := range productIDs {
		qty := quantities[productID]
		unitPrice := prices[productID]

		_, err = tx.Exec(ctx,
			`INSERT INTO order_items (id, order_id, product_id, quantity, unit_price, total_price)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			uuid.New(), orderID, productID, qty, unitPrice, unitPrice*int64(qty),
		)
		if err != nil {
			return uuid.Nil, time.Time{}, err
		}

		_, err = tx.Exec(ctx,
			`UPDATE products SET reserved = reserved + $1 WHERE id = $2`,
			qty, productID,
		)
		if err != nil {
			return uuid.Nil, time.Time{}, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, time.Time{}, err
	}
	return orderID, expiresAt, nil
}

// ConfirmPayment transitions an order based on the MercadoPago payment status.
//
// Approved payments: stock and reserved both decrease (units are sold).
// Rejected/cancelled payments: reserved decreases only (units return to available).
// Pending/in_process and other statuses: no transition — we wait for the next webhook.
//
// The order row is locked before the status check to prevent races with the
// expiry worker or duplicate webhook deliveries.
func (s *Service) ConfirmPayment(ctx context.Context, orderID uuid.UUID, mpStatus string) error {
	var newStatus string
	switch mpStatus {
	case "approved":
		newStatus = StatusPaid
	case "rejected", "cancelled":
		newStatus = StatusFailed
	default:
		// pending, in_process, in_mediation — no action yet
		return nil
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var currentStatus string
	err = tx.QueryRow(ctx,
		`SELECT status FROM orders WHERE id = $1 FOR UPDATE`,
		orderID,
	).Scan(&currentStatus)
	if err != nil {
		return err
	}
	if currentStatus != StatusPending {
		return nil // already transitioned (expiry job, duplicate webhook)
	}

	if newStatus == StatusPaid {
		// Decrement both stock and reserved — units are sold.
		_, err = tx.Exec(ctx,
			`UPDATE products p
			 SET stock    = p.stock    - oi.quantity,
			     reserved = p.reserved - oi.quantity
			 FROM order_items oi
			 WHERE oi.order_id = $1 AND p.id = oi.product_id`,
			orderID,
		)
	} else {
		// Release reservation only — units return to available stock.
		_, err = tx.Exec(ctx,
			`UPDATE products p
			 SET reserved = GREATEST(p.reserved - oi.quantity, 0)
			 FROM order_items oi
			 WHERE oi.order_id = $1 AND p.id = oi.product_id`,
			orderID,
		)
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`,
		newStatus, orderID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
