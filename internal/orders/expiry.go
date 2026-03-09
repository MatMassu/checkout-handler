package orders

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type ExpiryWorker struct {
	db       *pgxpool.Pool
	interval time.Duration
}

func NewExpiryWorker(db *pgxpool.Pool, interval time.Duration) *ExpiryWorker {
	return &ExpiryWorker{db: db, interval: interval}
}

func (w *ExpiryWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.expireOrders(ctx); err != nil {
				log.Printf("expiry worker: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (w *ExpiryWorker) expireOrders(ctx context.Context) error {
	rows, err := w.db.Query(ctx,
		`SELECT id FROM orders WHERE status = $1 AND expires_at < NOW()`,
		StatusPending,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Collect all IDs before closing the cursor so we don't hold
	// an open connection across the per-order transactions below.
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	for _, id := range ids {
		if err := w.expireOrder(ctx, id); err != nil {
			// Log and continue — a single failure shouldn't block the rest.
			log.Printf("expiry worker: failed to expire order %s: %v", id, err)
		}
	}
	return nil
}

func (w *ExpiryWorker) expireOrder(ctx context.Context, orderID uuid.UUID) error {
	tx, err := w.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// Re-check status under lock. Between the outer SELECT and now, a webhook
	// or cancellation may have already transitioned this order — skip it if so.
	var status string
	err = tx.QueryRow(ctx,
		`SELECT status FROM orders WHERE id = $1 FOR UPDATE`,
		orderID,
	).Scan(&status)
	if err != nil {
		return err
	}
	if status != StatusPending {
		return nil
	}

	// Release reserved stock. GREATEST guards against accidental negative values
	// if data ever gets out of sync.
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
		StatusExpired, orderID,
	)
	if err != nil {
		return err
	}

	return tx.Commit(ctx)
}
