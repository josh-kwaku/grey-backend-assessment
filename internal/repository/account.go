package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

const accountColumns = `id, user_id, currency, account_type, balance, version,
	account_number, routing_number, iban, swift_bic, provider, provider_ref,
	status, created_at`

type AccountRepository struct {
	db *sql.DB
}

func NewAccountRepository(db *sql.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

func (r *AccountRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE id = $1`, id,
	)
	a, err := scanAccount(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetByID: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("GetByID: %w", err)
	}
	return a, nil
}

func (r *AccountRepository) GetByUserAndCurrency(ctx context.Context, userID uuid.UUID, currency domain.Currency, accountType domain.AccountType) (*domain.Account, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts
		WHERE user_id = $1 AND currency = $2 AND account_type = $3`,
		userID, currency, accountType,
	)
	a, err := scanAccount(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetByUserAndCurrency: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("GetByUserAndCurrency: %w", err)
	}
	return a, nil
}

func (r *AccountRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Account, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE user_id = $1 ORDER BY created_at`, userID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetByUserID: %w", err)
	}
	defer rows.Close()

	var accounts []domain.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, fmt.Errorf("GetByUserID: scan: %w", err)
		}
		accounts = append(accounts, *a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetByUserID: rows: %w", err)
	}
	return accounts, nil
}

func (r *AccountRepository) Create(ctx context.Context, account *domain.Account) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO accounts (
			id, user_id, currency, account_type, balance, version,
			account_number, routing_number, iban, swift_bic, provider, provider_ref,
			status, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`,
		account.ID, account.UserID, account.Currency, account.AccountType,
		account.Balance, account.Version,
		account.AccountNumber, account.RoutingNumber, account.IBAN, account.SwiftBIC,
		account.Provider, account.ProviderRef,
		account.Status, account.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("Create: %w", err)
	}
	return nil
}

func (r *AccountRepository) GetForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Account, error) {
	row := tx.QueryRowContext(ctx,
		`SELECT `+accountColumns+` FROM accounts WHERE id = $1 FOR UPDATE`, id,
	)
	a, err := scanAccount(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetForUpdate: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("GetForUpdate: %w", err)
	}
	return a, nil
}

func (r *AccountRepository) UpdateBalance(ctx context.Context, tx *sql.Tx, id uuid.UUID, newBalance int64, newVersion int64) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE accounts SET balance = $1, version = $2 WHERE id = $3 AND version = $4`,
		newBalance, newVersion, id, newVersion-1,
	)
	if err != nil {
		return fmt.Errorf("UpdateBalance: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateBalance: rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("UpdateBalance: %w", domain.ErrVersionConflict)
	}
	return nil
}

func scanAccount(s scanner) (*domain.Account, error) {
	var a domain.Account
	err := s.Scan(
		&a.ID, &a.UserID, &a.Currency, &a.AccountType,
		&a.Balance, &a.Version,
		&a.AccountNumber, &a.RoutingNumber, &a.IBAN, &a.SwiftBIC,
		&a.Provider, &a.ProviderRef,
		&a.Status, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}
