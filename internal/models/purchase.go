package models

import (
	"time"

	"github.com/google/uuid"
)

type PurchasedTest struct {
	ID                uuid.UUID `db:"id"`
	UserID            uuid.UUID `db:"user_id"`
	TestID            uuid.UUID `db:"test_id"`
	StripeSessionID   string    `db:"stripe_session_id"`
	ExpiresAt         time.Time `db:"expires_at"`
	AttemptsRemaining int       `db:"attempts_remaining"`
	CreatedAt         time.Time `db:"created_at"`
}

func (PurchasedTest) TableName() string {
	return "purchased_tests"
}
