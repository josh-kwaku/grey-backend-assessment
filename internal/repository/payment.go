package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

const paymentColumns = `id, idempotency_key, type, status, source_account_id,
	dest_account_id, dest_account_number, dest_iban, dest_swift_bic, dest_bank_name,
	source_amount, source_currency, dest_amount, dest_currency, exchange_rate,
	fee_amount, fee_currency, provider, provider_ref, failure_reason, metadata,
	created_at, updated_at, completed_at`

type PaymentRepository struct {
	db *sql.DB
}

func NewPaymentRepository(db *sql.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Create(ctx context.Context, tx *sql.Tx, payment *domain.Payment) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO payments (
			id, idempotency_key, type, status, source_account_id,
			dest_account_id, dest_account_number, dest_iban, dest_swift_bic, dest_bank_name,
			source_amount, source_currency, dest_amount, dest_currency, exchange_rate,
			fee_amount, fee_currency, provider, provider_ref, failure_reason, metadata,
			created_at, updated_at, completed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
			$11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21,
			$22, $23, $24
		)`,
		payment.ID, payment.IdempotencyKey, payment.Type, payment.Status, payment.SourceAccountID,
		payment.DestAccountID, payment.DestAccountNumber, payment.DestIBAN, payment.DestSwiftBIC, payment.DestBankName,
		payment.SourceAmount, payment.SourceCurrency, payment.DestAmount, payment.DestCurrency, payment.ExchangeRate,
		payment.FeeAmount, payment.FeeCurrency, payment.Provider, payment.ProviderRef, payment.FailureReason, payment.Metadata,
		payment.CreatedAt, payment.UpdatedAt, payment.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("Create: %w", err)
	}
	return nil
}

func (r *PaymentRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE id = $1`, id,
	)
	p, err := scanPayment(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetByID: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("GetByID: %w", err)
	}
	return p, nil
}

func (r *PaymentRepository) GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+paymentColumns+` FROM payments WHERE idempotency_key = $1`, key,
	)
	p, err := scanPayment(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("GetByIdempotencyKey: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("GetByIdempotencyKey: %w", err)
	}
	return p, nil
}

func (r *PaymentRepository) UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.PaymentStatus, failureReason *string, completedAt *time.Time) error {
	res, err := tx.ExecContext(ctx,
		`UPDATE payments SET status = $1, failure_reason = $2, completed_at = $3, updated_at = now()
		WHERE id = $4`,
		status, failureReason, completedAt, id,
	)
	if err != nil {
		return fmt.Errorf("UpdateStatus: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("UpdateStatus: rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("UpdateStatus: %w", domain.ErrNotFound)
	}
	return nil
}

func scanPayment(s scanner) (*domain.Payment, error) {
	var p domain.Payment
	var destAccountID uuid.NullUUID
	var exchangeRate decimal.NullDecimal
	var feeCurrency *string
	var metadata *[]byte

	err := s.Scan(
		&p.ID, &p.IdempotencyKey, &p.Type, &p.Status, &p.SourceAccountID,
		&destAccountID, &p.DestAccountNumber, &p.DestIBAN, &p.DestSwiftBIC, &p.DestBankName,
		&p.SourceAmount, &p.SourceCurrency, &p.DestAmount, &p.DestCurrency, &exchangeRate,
		&p.FeeAmount, &feeCurrency, &p.Provider, &p.ProviderRef, &p.FailureReason, &metadata,
		&p.CreatedAt, &p.UpdatedAt, &p.CompletedAt,
	)
	if err != nil {
		return nil, err
	}

	if destAccountID.Valid {
		p.DestAccountID = &destAccountID.UUID
	}
	if exchangeRate.Valid {
		p.ExchangeRate = &exchangeRate.Decimal
	}
	if feeCurrency != nil {
		c := domain.Currency(*feeCurrency)
		p.FeeCurrency = &c
	}
	if metadata != nil {
		p.Metadata = *metadata
	}

	return &p, nil
}
