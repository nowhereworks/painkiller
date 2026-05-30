package store

import (
	"context"

	"github.com/google/uuid"
	"painkiller-shell/internal/models"
)

type PurchaseStore interface {
	Create(ctx context.Context, purchase *models.PurchasedTest) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.PurchasedTest, error)
	ListByUserID(ctx context.Context, userID uuid.UUID) ([]*models.PurchasedTest, error)
	DecrementAttemptsRemaining(ctx context.Context, id uuid.UUID) error
	IncrementAttemptsRemaining(ctx context.Context, id uuid.UUID) error
}

type purchaseStore struct {
	db *Store
}

func (s *Store) Purchases() PurchaseStore {
	return &purchaseStore{db: s}
}

func (p *purchaseStore) Create(ctx context.Context, purchase *models.PurchasedTest) error {
	query := `INSERT INTO purchased_tests (id, user_id, test_id, stripe_session_id, expires_at, attempts_remaining, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := p.db.db.ExecContext(ctx, query, purchase.ID, purchase.UserID, purchase.TestID, purchase.StripeSessionID, purchase.ExpiresAt, purchase.AttemptsRemaining, purchase.CreatedAt)
	return err
}

func (p *purchaseStore) GetByID(ctx context.Context, id uuid.UUID) (*models.PurchasedTest, error) {
	var purchase models.PurchasedTest
	query := `SELECT id, user_id, test_id, stripe_session_id, expires_at, attempts_remaining, created_at FROM purchased_tests WHERE id = $1`
	err := p.db.db.GetContext(ctx, &purchase, query, id)
	if err != nil {
		return nil, err
	}
	return &purchase, nil
}

func (p *purchaseStore) ListByUserID(ctx context.Context, userID uuid.UUID) ([]*models.PurchasedTest, error) {
	var purchases []*models.PurchasedTest
	query := `SELECT id, user_id, test_id, stripe_session_id, expires_at, attempts_remaining, created_at FROM purchased_tests WHERE user_id = $1 ORDER BY created_at DESC`
	err := p.db.db.SelectContext(ctx, &purchases, query, userID)
	return purchases, err
}

func (p *purchaseStore) DecrementAttemptsRemaining(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE purchased_tests SET attempts_remaining = attempts_remaining - 1 WHERE id = $1 AND attempts_remaining > 0`
	result, err := p.db.db.ExecContext(ctx, query, id)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNoRowsAffected
	}
	return nil
}

func (p *purchaseStore) IncrementAttemptsRemaining(ctx context.Context, id uuid.UUID) error {
	query := `UPDATE purchased_tests SET attempts_remaining = attempts_remaining + 1 WHERE id = $1`
	_, err := p.db.db.ExecContext(ctx, query, id)
	return err
}
