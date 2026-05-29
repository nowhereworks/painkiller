package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"painkiller-shell/internal/models"
)

type EnvironmentStore interface {
	Create(ctx context.Context, env *models.Environment) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Environment, error)
	GetByAttemptID(ctx context.Context, attemptID uuid.UUID) (*models.Environment, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.EnvironmentStatus) error
	UpdateProviderMetadata(ctx context.Context, id uuid.UUID, metadata json.RawMessage) error
}

type environmentStore struct {
	db *Store
}

func (s *Store) Environments() EnvironmentStore {
	return &environmentStore{db: s}
}

func (e *environmentStore) Create(ctx context.Context, env *models.Environment) error {
	query := `INSERT INTO environments (id, attempt_id, status, workstation_ip, provider_metadata, ssh_private_key, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := e.db.db.ExecContext(ctx, query, env.ID, env.AttemptID, env.Status, env.WorkstationIP, env.ProviderMetadata, env.SSHPrivateKey, env.CreatedAt)
	return err
}

func (e *environmentStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Environment, error) {
	var env models.Environment
	query := `SELECT id, attempt_id, status, workstation_ip, provider_metadata, ssh_private_key, created_at FROM environments WHERE id = $1`
	err := e.db.db.GetContext(ctx, &env, query, id)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

func (e *environmentStore) GetByAttemptID(ctx context.Context, attemptID uuid.UUID) (*models.Environment, error) {
	var env models.Environment
	query := `SELECT id, attempt_id, status, workstation_ip, provider_metadata, ssh_private_key, created_at FROM environments WHERE attempt_id = $1`
	err := e.db.db.GetContext(ctx, &env, query, attemptID)
	if err != nil {
		return nil, err
	}
	return &env, nil
}

func (e *environmentStore) UpdateStatus(ctx context.Context, id uuid.UUID, status models.EnvironmentStatus) error {
	query := `UPDATE environments SET status = $1 WHERE id = $2`
	_, err := e.db.db.ExecContext(ctx, query, status, id)
	return err
}

func (e *environmentStore) UpdateProviderMetadata(ctx context.Context, id uuid.UUID, metadata json.RawMessage) error {
	query := `UPDATE environments SET provider_metadata = $1 WHERE id = $2`
	_, err := e.db.db.ExecContext(ctx, query, metadata, id)
	return err
}
