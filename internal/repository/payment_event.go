package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

const paymentEventColumns = `id, payment_id, event_type, actor, payload, created_at`

type PaymentEventRepository struct {
	db *sql.DB
}

func NewPaymentEventRepository(db *sql.DB) *PaymentEventRepository {
	return &PaymentEventRepository{db: db}
}

func (r *PaymentEventRepository) Create(ctx context.Context, tx *sql.Tx, event *domain.PaymentEvent) error {
	_, err := tx.ExecContext(ctx,
		`INSERT INTO payment_events (id, payment_id, event_type, actor, payload, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`,
		event.ID, event.PaymentID, event.EventType, event.Actor,
		event.Payload, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("Create: %w", err)
	}
	return nil
}

func (r *PaymentEventRepository) GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]domain.PaymentEvent, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+paymentEventColumns+` FROM payment_events
		WHERE payment_id = $1 ORDER BY created_at`, paymentID,
	)
	if err != nil {
		return nil, fmt.Errorf("GetByPaymentID: %w", err)
	}
	defer rows.Close()

	var events []domain.PaymentEvent
	for rows.Next() {
		e, err := scanPaymentEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("GetByPaymentID: scan: %w", err)
		}
		events = append(events, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetByPaymentID: rows: %w", err)
	}
	return events, nil
}

func scanPaymentEvent(s scanner) (*domain.PaymentEvent, error) {
	var e domain.PaymentEvent
	err := s.Scan(
		&e.ID, &e.PaymentID, &e.EventType, &e.Actor,
		&e.Payload, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}
