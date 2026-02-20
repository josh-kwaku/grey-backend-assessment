package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

const ledgerColumns = `id, payment_id, account_id, entry_type, amount, currency,
	balance_before, balance_after, created_at`

type LedgerRepository struct {
	db *sql.DB
}

func NewLedgerRepository(db *sql.DB) *LedgerRepository {
	return &LedgerRepository{db: db}
}

func (r *LedgerRepository) Create(ctx context.Context, tx *sql.Tx, entry *domain.LedgerEntry) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO ledger_entries (
			id, payment_id, account_id, entry_type, amount, currency,
			balance_before, balance_after, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		entry.ID, entry.PaymentID, entry.AccountID, entry.EntryType,
		entry.Amount, entry.Currency, entry.BalanceBefore, entry.BalanceAfter,
		entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("Create: %w", err)
	}
	return nil
}

func (r *LedgerRepository) GetByAccountID(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]domain.LedgerEntry, int, error) {
	var total int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM ledger_entries WHERE account_id = $1`, accountID,
	).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("GetByAccountID: count: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT `+ledgerColumns+` FROM ledger_entries
		WHERE account_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		accountID, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("GetByAccountID: %w", err)
	}
	defer rows.Close()

	var entries []domain.LedgerEntry
	for rows.Next() {
		e, err := scanLedgerEntry(rows)
		if err != nil {
			return nil, 0, fmt.Errorf("GetByAccountID: scan: %w", err)
		}
		entries = append(entries, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("GetByAccountID: rows: %w", err)
	}
	return entries, total, nil
}

func (r *LedgerRepository) GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]domain.LedgerEntry, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+ledgerColumns+` FROM ledger_entries
		WHERE payment_id = $1 ORDER BY created_at`, paymentID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetByPaymentID: %w", err)
	}
	defer rows.Close()

	var entries []domain.LedgerEntry
	for rows.Next() {
		e, err := scanLedgerEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("GetByPaymentID: scan: %w", err)
		}
		entries = append(entries, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetByPaymentID: rows: %w", err)
	}
	return entries, nil
}

func scanLedgerEntry(s scanner) (*domain.LedgerEntry, error) {
	var e domain.LedgerEntry
	err := s.Scan(
		&e.ID, &e.PaymentID, &e.AccountID, &e.EntryType,
		&e.Amount, &e.Currency, &e.BalanceBefore, &e.BalanceAfter,
		&e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}
