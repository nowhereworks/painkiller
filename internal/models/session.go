package models

import (
	"time"

	"github.com/google/uuid"
)

type Session struct {
	ID             uuid.UUID  `db:"id"`
	AttemptID      uuid.UUID  `db:"attempt_id"`
	EnvironmentID  uuid.UUID  `db:"environment_id"`
	TerminalToken  string     `db:"terminal_token"`
	FirstOpenedAt  *time.Time `db:"first_opened_at"`
	CreatedAt      time.Time  `db:"created_at"`
}

func (Session) TableName() string {
	return "sessions"
}
