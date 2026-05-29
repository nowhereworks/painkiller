package store

import (
	"context"

	"github.com/google/uuid"
	"painkiller-shell/internal/models"
)

type AttemptStore interface {
	Create(ctx context.Context, attempt *models.Attempt) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Attempt, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status models.AttemptStatus) error
	UpdateScore(ctx context.Context, id uuid.UUID, score, maxScore int) error
	ListByPurchaseID(ctx context.Context, purchaseID uuid.UUID) ([]*models.Attempt, error)
	ListByStatus(ctx context.Context, status models.AttemptStatus) ([]*models.Attempt, error)
}

type attemptStore struct {
	db *Store
}

func (s *Store) Attempts() AttemptStore {
	return &attemptStore{db: s}
}

func (a *attemptStore) Create(ctx context.Context, attempt *models.Attempt) error {
	query := `INSERT INTO attempts (id, purchased_test_id, status, score, max_score, started_at, ended_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := a.db.db.ExecContext(ctx, query, attempt.ID, attempt.PurchasedTestID, attempt.Status, attempt.Score, attempt.MaxScore, attempt.StartedAt, attempt.EndedAt, attempt.CreatedAt)
	return err
}

func (a *attemptStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Attempt, error) {
	var attempt models.Attempt
	query := `SELECT id, purchased_test_id, status, score, max_score, started_at, ended_at, created_at FROM attempts WHERE id = $1`
	err := a.db.db.GetContext(ctx, &attempt, query, id)
	if err != nil {
		return nil, err
	}
	return &attempt, nil
}

func (a *attemptStore) UpdateStatus(ctx context.Context, id uuid.UUID, status models.AttemptStatus) error {
	query := `UPDATE attempts SET status = $1 WHERE id = $2`
	_, err := a.db.db.ExecContext(ctx, query, status, id)
	return err
}

func (a *attemptStore) UpdateScore(ctx context.Context, id uuid.UUID, score, maxScore int) error {
	query := `UPDATE attempts SET score = $1, max_score = $2 WHERE id = $3`
	_, err := a.db.db.ExecContext(ctx, query, score, maxScore, id)
	return err
}

func (a *attemptStore) ListByPurchaseID(ctx context.Context, purchaseID uuid.UUID) ([]*models.Attempt, error) {
	var attempts []*models.Attempt
	query := `SELECT id, purchased_test_id, status, score, max_score, started_at, ended_at, created_at FROM attempts WHERE purchased_test_id = $1 ORDER BY created_at DESC`
	err := a.db.db.SelectContext(ctx, &attempts, query, purchaseID)
	return attempts, err
}

func (a *attemptStore) ListByStatus(ctx context.Context, status models.AttemptStatus) ([]*models.Attempt, error) {
	var attempts []*models.Attempt
	query := `SELECT id, purchased_test_id, status, score, max_score, started_at, ended_at, created_at FROM attempts WHERE status = $1 ORDER BY created_at`
	err := a.db.db.SelectContext(ctx, &attempts, query, status)
	return attempts, err
}
