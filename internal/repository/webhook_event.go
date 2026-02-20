package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

const webhookEventColumns = `id, idempotency_key, event_type, payload, status,
	attempts, last_attempt, created_at`

type WebhookEventRepository struct {
	db *sql.DB
}

func NewWebhookEventRepository(db *sql.DB) *WebhookEventRepository {
	return &WebhookEventRepository{db: db}
}

func (r *WebhookEventRepository) Create(ctx context.Context, event *domain.WebhookEvent) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO webhook_events (
			id, idempotency_key, event_type, payload, status, attempts, last_attempt, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		event.ID, event.IdempotencyKey, event.EventType, event.Payload,
		event.Status, event.Attempts, event.LastAttempt, event.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("Create: %w", err)
	}
	return nil
}

func (r *WebhookEventRepository) GetPending(ctx context.Context, limit int) ([]domain.WebhookEvent, error) {
	// FOR UPDATE SKIP LOCKED prevents multiple processors from claiming the same event
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+webhookEventColumns+` FROM webhook_events
		WHERE status = $1 ORDER BY created_at LIMIT $2 FOR UPDATE SKIP LOCKED`,
		domain.WebhookEventStatusPending, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("GetPending: %w", err)
	}
	defer rows.Close()

	var events []domain.WebhookEvent
	for rows.Next() {
		e, err := scanWebhookEvent(rows)
		if err != nil {
			return nil, fmt.Errorf("GetPending: scan: %w", err)
		}
		events = append(events, *e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetPending: rows: %w", err)
	}
	return events, nil
}

func (r *WebhookEventRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WebhookEventStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE webhook_events SET status = $1, attempts = attempts + 1, last_attempt = now()
		WHERE id = $2`,
		status, id,
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

func scanWebhookEvent(s scanner) (*domain.WebhookEvent, error) {
	var e domain.WebhookEvent
	err := s.Scan(
		&e.ID, &e.IdempotencyKey, &e.EventType, &e.Payload,
		&e.Status, &e.Attempts, &e.LastAttempt, &e.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}
