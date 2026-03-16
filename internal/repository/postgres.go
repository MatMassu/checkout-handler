package repository

import (
	"context"
	"sort"

	"github.com/MatMassu/checkout-handler/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres implements checkout.Repository and payment.DBRepository.
type Postgres struct {
	db *pgxpool.Pool
}

func NewPostgres(db *pgxpool.Pool) *Postgres {
	return &Postgres{db: db}
}

// CreateOrder opens a transaction, locks product rows in a consistent order
// (sorted by UUID string to prevent deadlocks), validates available stock,
// inserts the order + items, and reserves stock. Returns the created Order.
func (r *Postgres) CreateOrder(ctx context.Context, userID uuid.UUID, productIDs []uuid.UUID, quantities map[uuid.UUID]int) (domain.Order, error) {
	// Sort IDs for consistent lock ordering — prevents deadlocks with concurrent
	// transactions that request the same rows in a different order.
	sorted := make([]uuid.UUID, len(productIDs))
	copy(sorted, productIDs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].String() < sorted[j].String()
	})

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return domain.Order{}, err
	}
	defer tx.Rollback(ctx)

	// Lock rows, validate stock, collect prices and product names.
	type productInfo struct {
		price  int64
		artist string
		title  string
	}
	info := make(map[uuid.UUID]productInfo, len(sorted))
	for _, productID := range sorted {
		var price int64
		var stock, reserved int
		var artist, title string
		err := tx.QueryRow(ctx,
			`SELECT price, stock, reserved, artist, title FROM products WHERE id = $1 FOR UPDATE`,
			productID,
		).Scan(&price, &stock, &reserved, &artist, &title)
		if err == pgx.ErrNoRows {
			return domain.Order{}, domain.ErrProductNotFound
		}
		if err != nil {
			return domain.Order{}, err
		}
		if stock-reserved < quantities[productID] {
			return domain.Order{}, domain.ErrInsufficientStock
		}
		info[productID] = productInfo{price: price, artist: artist, title: title}
	}

	// Compute subtotal.
	var subtotal int64
	for productID, qty := range quantities {
		subtotal += info[productID].price * int64(qty)
	}

	// Insert order; expires_at is set by the DB clock.
	orderID := uuid.New()
	var order domain.Order
	err = tx.QueryRow(ctx,
		`INSERT INTO orders (id, user_id, status, currency, subtotal_amount, tax_amount, total_amount, expires_at)
		 VALUES ($1, $2, 'pending', 'ARS', $3, 0, $3, NOW() + interval '10 minutes')
		 RETURNING id, user_id, status, currency, subtotal_amount, tax_amount, total_amount, expires_at, created_at, updated_at`,
		orderID, userID, subtotal,
	).Scan(
		&order.ID, &order.UserID, &order.Status, &order.Currency,
		&order.SubtotalAmount, &order.TaxAmount, &order.TotalAmount,
		&order.ExpiresAt, &order.CreatedAt, &order.UpdatedAt,
	)
	if err != nil {
		return domain.Order{}, err
	}

	// Insert order items, reserve stock, and build domain.OrderItems.
	orderItems := make([]domain.OrderItem, 0, len(sorted))
	for _, productID := range sorted {
		qty := quantities[productID]
		unitPrice := info[productID].price

		_, err = tx.Exec(ctx,
			`INSERT INTO order_items (id, order_id, product_id, quantity, unit_price, total_price)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			uuid.New(), orderID, productID, qty, unitPrice, unitPrice*int64(qty),
		)
		if err != nil {
			return domain.Order{}, err
		}

		_, err = tx.Exec(ctx,
			`UPDATE products SET reserved = reserved + $1 WHERE id = $2`,
			qty, productID,
		)
		if err != nil {
			return domain.Order{}, err
		}

		orderItems = append(orderItems, domain.OrderItem{
			ProductID: productID,
			Artist:    info[productID].artist,
			Title:     info[productID].title,
			Quantity:  qty,
			UnitPrice: unitPrice,
		})
	}
	order.Items = orderItems

	if err := tx.Commit(ctx); err != nil {
		return domain.Order{}, err
	}
	return order, nil
}

// ConfirmPayment transitions an order from pending to the given status.
// Locks the order row to guard against races with the expiry worker or
// duplicate webhook deliveries; silently no-ops if already transitioned.
func (r *Postgres) ConfirmPayment(ctx context.Context, orderID uuid.UUID, status string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var current string
	err = tx.QueryRow(ctx,
		`SELECT status FROM orders WHERE id = $1 FOR UPDATE`,
		orderID,
	).Scan(&current)
	if err != nil {
		return err
	}
	if current != domain.StatusPending {
		return nil
	}

	if status == domain.StatusPaid {
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
		status, orderID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// FindExpiredOrderIDs returns IDs of all pending orders past their expiry time.
// The result set is fully collected before returning so the caller can open
// per-order transactions without holding an open cursor.
func (r *Postgres) FindExpiredOrderIDs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.db.Query(ctx,
		`SELECT id FROM orders WHERE status = $1 AND expires_at < NOW()`,
		domain.StatusPending,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ExpireOrder transitions a single order to expired, releasing its stock reservation.
// Re-checks status under lock before acting.
func (r *Postgres) ExpireOrder(ctx context.Context, orderID uuid.UUID) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status string
	err = tx.QueryRow(ctx,
		`SELECT status FROM orders WHERE id = $1 FOR UPDATE`,
		orderID,
	).Scan(&status)
	if err != nil {
		return err
	}
	if status != domain.StatusPending {
		return nil
	}

	_, err = tx.Exec(ctx,
		`UPDATE products p
		 SET reserved = GREATEST(p.reserved - oi.quantity, 0)
		 FROM order_items oi
		 WHERE oi.order_id = $1 AND p.id = oi.product_id`,
		orderID,
	)
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx,
		`UPDATE orders SET status = $1, updated_at = NOW() WHERE id = $2`,
		domain.StatusExpired, orderID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}

// InsertPayment records a new pending payment linked to an order.
func (r *Postgres) InsertPayment(ctx context.Context, paymentID uuid.UUID, orderID uuid.UUID, preferenceID string, amount int64) error {
	_, err := r.db.Exec(ctx,
		`INSERT INTO payments (id, order_id, preference_id, status, amount)
		 VALUES ($1, $2, $3, 'pending', $4)`,
		paymentID, orderID, preferenceID, amount,
	)
	return err
}

// UpdatePayment updates a payment record with data received from the MercadoPago API.
func (r *Postgres) UpdatePayment(ctx context.Context, p domain.MPPayment) error {
	_, err := r.db.Exec(ctx,
		`UPDATE payments
		 SET mp_payment_id = $1, status = $2, status_detail = $3, updated_at = NOW()
		 WHERE order_id = $4`,
		p.ID, p.Status, p.StatusDetail, p.OrderID,
	)
	return err
}
