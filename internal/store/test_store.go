package store

import (
	"context"

	"github.com/google/uuid"
	"painkiller-shell/internal/models"
)

type TestWithProduct struct {
	models.Test
	ProductTitle       string  `db:"product_title"`
	ProductDescription string  `db:"product_description"`
	ProductStripePriceID *string `db:"product_stripe_price_id"`
	ProductIsFree      bool    `db:"product_is_free"`
}

type TestStore interface {
	Create(ctx context.Context, test *models.Test) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Test, error)
	GetWithProduct(ctx context.Context, id uuid.UUID) (*TestWithProduct, error)
	List(ctx context.Context) ([]*models.Test, error)
	ListWithProduct(ctx context.Context) ([]*TestWithProduct, error)
	Update(ctx context.Context, test *models.Test) error
	Delete(ctx context.Context, id uuid.UUID) error
	HasPurchases(ctx context.Context, testID uuid.UUID) (bool, error)
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

func (t *testStore) GetWithProduct(ctx context.Context, id uuid.UUID) (*TestWithProduct, error) {
	var result TestWithProduct
	query := `SELECT t.id, t.product_id, t.scenario_version_id, t.duration_minutes, t.access_window_hours, t.attempts_allowed, t.created_at,
		p.title AS product_title, p.description AS product_description, p.stripe_price_id AS product_stripe_price_id, p.is_free AS product_is_free
		FROM tests t JOIN products p ON t.product_id = p.id
		WHERE t.id = $1`
	err := t.db.db.GetContext(ctx, &result, query, id)
	if err != nil {
		return nil, err
	}
	return &result, nil
}

func (t *testStore) List(ctx context.Context) ([]*models.Test, error) {
	var tests []*models.Test
	query := `SELECT id, product_id, scenario_version_id, duration_minutes, access_window_hours, attempts_allowed, created_at FROM tests ORDER BY created_at`
	err := t.db.db.SelectContext(ctx, &tests, query)
	return tests, err
}

func (t *testStore) ListWithProduct(ctx context.Context) ([]*TestWithProduct, error) {
	var results []*TestWithProduct
	query := `SELECT t.id, t.product_id, t.scenario_version_id, t.duration_minutes, t.access_window_hours, t.attempts_allowed, t.created_at,
		p.title AS product_title, p.description AS product_description, p.stripe_price_id AS product_stripe_price_id, p.is_free AS product_is_free
		FROM tests t JOIN products p ON t.product_id = p.id
		ORDER BY t.created_at`
	err := t.db.db.SelectContext(ctx, &results, query)
	return results, err
}

func (t *testStore) Update(ctx context.Context, test *models.Test) error {
	query := `UPDATE tests SET duration_minutes = $1, access_window_hours = $2, attempts_allowed = $3 WHERE id = $4`
	_, err := t.db.db.ExecContext(ctx, query, test.DurationMinutes, test.AccessWindowHours, test.AttemptsAllowed, test.ID)
	return err
}

func (t *testStore) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := t.db.db.ExecContext(ctx, `DELETE FROM tests WHERE id = $1`, id)
	return err
}

func (t *testStore) HasPurchases(ctx context.Context, testID uuid.UUID) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM purchased_tests WHERE test_id = $1`
	err := t.db.db.GetContext(ctx, &count, query, testID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
