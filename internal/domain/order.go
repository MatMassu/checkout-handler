package domain

import (
	"time"

	"github.com/google/uuid"
)

type Order struct {
	ID             uuid.UUID
	UserID         uuid.UUID
	Status         string
	Currency       string
	SubtotalAmount int64
	TaxAmount      int64
	TotalAmount    int64
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Product struct {
	ID       uuid.UUID
	Price    int64
	Stock    int
	Reserved int
}

func (p Product) Available() int {
	return p.Stock - p.Reserved
}
