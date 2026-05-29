package store

import (
	"context"

	"github.com/google/uuid"
	"painkiller-shell/internal/models"
)

type UserStore interface {
	Create(ctx context.Context, user *models.User) error
	GetByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetByEmail(ctx context.Context, email string) (*models.User, error)
}

type userStore struct {
	db *Store
}

func (s *Store) Users() UserStore {
	return &userStore{db: s}
}

func (u *userStore) Create(ctx context.Context, user *models.User) error {
	query := `INSERT INTO users (id, email, password_hash, is_admin, created_at)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := u.db.db.ExecContext(ctx, query, user.ID, user.Email, user.PasswordHash, user.IsAdmin, user.CreatedAt)
	return err
}

func (u *userStore) GetByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	var user models.User
	query := `SELECT id, email, password_hash, is_admin, created_at FROM users WHERE id = $1`
	err := u.db.db.GetContext(ctx, &user, query, id)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (u *userStore) GetByEmail(ctx context.Context, email string) (*models.User, error) {
	var user models.User
	query := `SELECT id, email, password_hash, is_admin, created_at FROM users WHERE email = $1`
	err := u.db.db.GetContext(ctx, &user, query, email)
	if err != nil {
		return nil, err
	}
	return &user, nil
}
