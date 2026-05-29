package store

import (
	"context"

	"github.com/google/uuid"
	"painkiller-shell/internal/models"
)

type ProductStore interface {
	Create(ctx context.Context, product *models.Product) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.Product, error)
	GetByStripePriceID(ctx context.Context, stripePriceID string) (*models.Product, error)
	List(ctx context.Context) ([]*models.Product, error)
}

type productStore struct {
	db *Store
}

func (s *Store) Products() ProductStore {
	return &productStore{db: s}
}

func (p *productStore) Create(ctx context.Context, product *models.Product) error {
	query := `INSERT INTO products (id, stripe_price_id, title, description, created_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := p.db.db.ExecContext(ctx, query, product.ID, product.StripePriceID, product.Title, product.Description, product.CreatedAt)
	return err
}

func (p *productStore) GetByID(ctx context.Context, id uuid.UUID) (*models.Product, error) {
	var product models.Product
	query := `SELECT id, stripe_price_id, title, description, created_at FROM products WHERE id = $1`
	err := p.db.db.GetContext(ctx, &product, query, id)
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func (p *productStore) GetByStripePriceID(ctx context.Context, stripePriceID string) (*models.Product, error) {
	var product models.Product
	query := `SELECT id, stripe_price_id, title, description, created_at FROM products WHERE stripe_price_id = $1`
	err := p.db.db.GetContext(ctx, &product, query, stripePriceID)
	if err != nil {
		return nil, err
	}
	return &product, nil
}

func (p *productStore) List(ctx context.Context) ([]*models.Product, error) {
	var products []*models.Product
	query := `SELECT id, stripe_price_id, title, description, created_at FROM products ORDER BY created_at`
	err := p.db.db.SelectContext(ctx, &products, query)
	return products, err
}
