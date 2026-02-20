package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type IdempotencyCacheEntry struct {
	Key          string
	UserID       uuid.UUID
	RequestHash  string
	StatusCode   int
	ResponseBody []byte
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type IdempotencyRepository struct {
	db *sql.DB
}

func NewIdempotencyRepository(db *sql.DB) *IdempotencyRepository {
	return &IdempotencyRepository{db: db}
}

func (r *IdempotencyRepository) Get(ctx context.Context, key string, userID uuid.UUID) (*IdempotencyCacheEntry, error) {
	var e IdempotencyCacheEntry
	err := r.db.QueryRowContext(ctx,
		`SELECT idempotency_key, user_id, request_hash, status_code, response_body, created_at, expires_at
		FROM idempotency_cache
		WHERE idempotency_key = $1 AND user_id = $2 AND expires_at > now()`,
		key, userID,
	).Scan(&e.Key, &e.UserID, &e.RequestHash, &e.StatusCode, &e.ResponseBody, &e.CreatedAt, &e.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("Get: %w", err)
	}
	return &e, nil
}

func (r *IdempotencyRepository) Set(ctx context.Context, entry *IdempotencyCacheEntry) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO idempotency_cache (idempotency_key, user_id, request_hash, status_code, response_body, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (idempotency_key, user_id) DO NOTHING`,
		entry.Key, entry.UserID, entry.RequestHash, entry.StatusCode, entry.ResponseBody, entry.CreatedAt, entry.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("Set: %w", err)
	}
	return nil
}

func (r *IdempotencyRepository) CleanExpired(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM idempotency_cache WHERE expires_at < now()`,
	)
	if err != nil {
		return 0, fmt.Errorf("CleanExpired: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("CleanExpired: rows affected: %w", err)
	}
	return n, nil
}
