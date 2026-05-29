package models

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type Environment struct {
	ID               uuid.UUID         `db:"id"`
	AttemptID        uuid.UUID         `db:"attempt_id"`
	Status           EnvironmentStatus `db:"status"`
	WorkstationIP    *string           `db:"workstation_ip"`
	ProviderMetadata json.RawMessage   `db:"provider_metadata"`
	SSHPrivateKey    []byte            `db:"ssh_private_key"`
	CreatedAt        time.Time         `db:"created_at"`
}

func (Environment) TableName() string {
	return "environments"
}
