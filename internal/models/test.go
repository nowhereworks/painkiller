package models

import (
	"time"

	"github.com/google/uuid"
)

type Test struct {
	ID                uuid.UUID `db:"id"`
	ProductID         uuid.UUID `db:"product_id"`
	ScenarioVersionID uuid.UUID `db:"scenario_version_id"`
	DurationMinutes   int       `db:"duration_minutes"`
	AccessWindowHours int       `db:"access_window_hours"`
	AttemptsAllowed   int       `db:"attempts_allowed"`
	CreatedAt         time.Time `db:"created_at"`
}

func (Test) TableName() string {
	return "tests"
}
