package models

import (
	"time"

	"github.com/google/uuid"
)

type Task struct {
	ID                uuid.UUID `db:"id"`
	ScenarioVersionID uuid.UUID `db:"scenario_version_id"`
	ExternalID        string    `db:"external_id"`
	ClusterID         string    `db:"cluster_id"`
	KubeContext       string    `db:"kube_context"`
	Points            int       `db:"points"`
	Prompt            string    `db:"prompt"`
	CreatedAt         time.Time `db:"created_at"`
}

func (Task) TableName() string {
	return "tasks"
}
