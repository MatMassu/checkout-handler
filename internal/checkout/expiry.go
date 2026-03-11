package checkout

import (
	"context"
	"log"
	"time"
)

type ExpiryWorker struct {
	repo     Repository
	interval time.Duration
}

func NewExpiryWorker(repo Repository, interval time.Duration) *ExpiryWorker {
	return &ExpiryWorker{repo: repo, interval: interval}
}

func (w *ExpiryWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := w.expireAll(ctx); err != nil {
				log.Printf("expiry worker: %v", err)
			}
		case <-ctx.Done():
			return
		}
	}
}

func (w *ExpiryWorker) expireAll(ctx context.Context) error {
	ids, err := w.repo.FindExpiredOrderIDs(ctx)
	if err != nil {
		return err
	}
	// Cursor closed by FindExpiredOrderIDs; process each order independently.
	for _, id := range ids {
		if err := w.repo.ExpireOrder(ctx, id); err != nil {
			log.Printf("expiry worker: expire order %s: %v", id, err)
		}
	}
	return nil
}
