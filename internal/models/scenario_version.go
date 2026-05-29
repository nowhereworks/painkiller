package models

import (
	"time"

	"github.com/google/uuid"
)

type ScenarioVersion struct {
	ID          uuid.UUID `db:"id"`
	ExternalID  string    `db:"external_id"`
	Title       string    `db:"title"`
	GitCommit   string    `db:"git_commit"`
	DurationMinutes   int `db:"duration_minutes"`
	AccessWindowHours int `db:"access_window_hours"`
	AttemptsAllowed   int `db:"attempts_allowed"`
	TopologyJSON string   `db:"topology_json"`
	CreatedAt   time.Time `db:"created_at"`
}

func (ScenarioVersion) TableName() string {
	return "scenario_versions"
}
