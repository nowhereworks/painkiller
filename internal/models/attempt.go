package models

import (
	"time"

	"github.com/google/uuid"
)

type Attempt struct {
	ID             uuid.UUID     `db:"id"`
	PurchasedTestID uuid.UUID    `db:"purchased_test_id"`
	Status         AttemptStatus `db:"status"`
	Score          *int          `db:"score"`
	MaxScore       *int          `db:"max_score"`
	StartedAt      *time.Time    `db:"started_at"`
	EndedAt        *time.Time    `db:"ended_at"`
	CreatedAt      time.Time     `db:"created_at"`
}

func (Attempt) TableName() string {
	return "attempts"
}
