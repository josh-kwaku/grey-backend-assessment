package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

const userColumns = `id, email, name, password_hash, unique_name, status, created_at`

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE id = $1`, id,
	)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetByID: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("GetByID: %w", err)
	}
	return u, nil
}

func (r *UserRepository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE email = $1`, email,
	)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetByEmail: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("GetByEmail: %w", err)
	}
	return u, nil
}

func (r *UserRepository) GetByUniqueName(ctx context.Context, uniqueName string) (*domain.User, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+userColumns+` FROM users WHERE unique_name = $1`, uniqueName,
	)
	u, err := scanUser(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetByUniqueName: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("GetByUniqueName: %w", err)
	}
	return u, nil
}

func scanUser(s scanner) (*domain.User, error) {
	var u domain.User
	err := s.Scan(
		&u.ID, &u.Email, &u.Name, &u.PasswordHash,
		&u.UniqueName, &u.Status, &u.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
