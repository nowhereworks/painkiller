package models

import (
	"time"

	"github.com/google/uuid"
)

type Product struct {
	ID            uuid.UUID `db:"id"`
	StripePriceID string    `db:"stripe_price_id"`
	Title         string    `db:"title"`
	Description   string    `db:"description"`
	CreatedAt     time.Time `db:"created_at"`
}

func (Product) TableName() string {
	return "products"
}
