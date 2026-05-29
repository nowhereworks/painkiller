package store

import (
	"context"

	"github.com/google/uuid"
	"painkiller-shell/internal/models"
)

type TestStore interface {
	Create(ctx context.Context, test *models.Test) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Test, error)
	List(ctx context.Context) ([]*models.Test, error)
}

type testStore struct {
	db *Store
}

func (s *Store) Tests() TestStore {
	return &testStore{db: s}
}

func (t *testStore) Create(ctx context.Context, test *models.Test) error {
	query := `INSERT INTO tests (id, product_id, scenario_version_id, duration_minutes, access_window_hours, attempts_allowed, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := t.db.db.ExecContext(ctx, query, test.ID, test.ProductID, test.ScenarioVersionID, test.DurationMinutes, test.AccessWindowHours, test.AttemptsAllowed, test.CreatedAt)
	return err
}

func (t *testStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Test, error) {
	var test models.Test
	query := `SELECT id, product_id, scenario_version_id, duration_minutes, access_window_hours, attempts_allowed, created_at FROM tests WHERE id = $1`
	err := t.db.db.GetContext(ctx, &test, query, id)
	if err != nil {
		return nil, err
	}
	return &test, nil
}

func (t *testStore) List(ctx context.Context) ([]*models.Test, error) {
	var tests []*models.Test
	query := `SELECT id, product_id, scenario_version_id, duration_minutes, access_window_hours, attempts_allowed, created_at FROM tests ORDER BY created_at`
	err := t.db.db.SelectContext(ctx, &tests, query)
	return tests, err
}
