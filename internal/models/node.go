package models

import (
	"time"

	"github.com/google/uuid"
)

type Node struct {
	ID           uuid.UUID `db:"id"`
	ClusterID    uuid.UUID `db:"cluster_id"`
	Name         string    `db:"name"`
	Role         NodeRole  `db:"role"`
	ProviderVMID string    `db:"provider_vm_id"`
	IPAddress    string    `db:"ip_address"`
	CreatedAt    time.Time `db:"created_at"`
}

func (Node) TableName() string {
	return "nodes"
}
