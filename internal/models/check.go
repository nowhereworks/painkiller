package models

import (
	"time"

	"github.com/google/uuid"
)

type Check struct {
	ID        uuid.UUID `db:"id"`
	TaskID    uuid.UUID `db:"task_id"`
	ExternalID string   `db:"external_id"`
	ClusterID string    `db:"cluster_id"`
	Type      CheckType `db:"type"`
	Command   string    `db:"command"`
	Points    int       `db:"points"`
	CreatedAt time.Time `db:"created_at"`
}

func (Check) TableName() string {
	return "checks"
}
