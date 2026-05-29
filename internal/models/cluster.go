package models

import (
	"time"

	"github.com/google/uuid"
)

type Cluster struct {
	ID            uuid.UUID `db:"id"`
	EnvironmentID uuid.UUID `db:"environment_id"`
	Name          string    `db:"name"`
	KubeContext   string    `db:"kube_context"`
	CreatedAt     time.Time `db:"created_at"`
}

func (Cluster) TableName() string {
	return "clusters"
}
