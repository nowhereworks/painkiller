package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"painkiller-shell/internal/models"
)

type SessionStore interface {
	Create(ctx context.Context, session *models.Session) error
	GetByAttemptID(ctx context.Context, attemptID uuid.UUID) (*models.Session, error)
	GetByTerminalToken(ctx context.Context, token string) (*models.Session, error)
	UpdateFirstOpenedAt(ctx context.Context, id uuid.UUID, t time.Time) error
}

type sessionStore struct {
	db *Store
}

func (s *Store) Sessions() SessionStore {
	return &sessionStore{db: s}
}

func (ss *sessionStore) Create(ctx context.Context, session *models.Session) error {
	query := `INSERT INTO sessions (id, attempt_id, environment_id, terminal_token, first_opened_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := ss.db.db.ExecContext(ctx, query, session.ID, session.AttemptID, session.EnvironmentID, session.TerminalToken, session.FirstOpenedAt, session.CreatedAt)
	return err
}

func (ss *sessionStore) GetByAttemptID(ctx context.Context, attemptID uuid.UUID) (*models.Session, error) {
	var session models.Session
	query := `SELECT id, attempt_id, environment_id, terminal_token, first_opened_at, created_at FROM sessions WHERE attempt_id = $1`
	err := ss.db.db.GetContext(ctx, &session, query, attemptID)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (ss *sessionStore) GetByTerminalToken(ctx context.Context, token string) (*models.Session, error) {
	var session models.Session
	query := `SELECT id, attempt_id, environment_id, terminal_token, first_opened_at, created_at FROM sessions WHERE terminal_token = $1`
	err := ss.db.db.GetContext(ctx, &session, query, token)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (ss *sessionStore) UpdateFirstOpenedAt(ctx context.Context, id uuid.UUID, t time.Time) error {
	query := `UPDATE sessions SET first_opened_at = $1 WHERE id = $2`
	_, err := ss.db.db.ExecContext(ctx, query, t, id)
	return err
}
