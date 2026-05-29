package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Job struct {
	ID        uuid.UUID       `db:"id"`
	Kind      string          `db:"kind"`
	Payload   json.RawMessage `db:"payload"`
	Status    JobStatus       `db:"status"`
	Attempts  int             `db:"attempts"`
	RunAt     time.Time       `db:"run_at"`
	CreatedAt time.Time       `db:"created_at"`
}

func (Job) TableName() string {
	return "jobs"
}
