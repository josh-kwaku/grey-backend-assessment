package repository

import (
	"context"
	"database/sql"
	"fmt"
)

type scanner interface {
	Scan(dest ...any) error
}

type DB struct {
	pool *sql.DB
}

func NewDB(pool *sql.DB) *DB {
	return &DB{pool: pool}
}

func (d *DB) Conn() *sql.DB {
	return d.pool
}

func (d *DB) BeginTx(ctx context.Context, opts *sql.TxOptions) (*sql.Tx, error) {
	tx, err := d.pool.BeginTx(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("BeginTx: %w", err)
	}
	return tx, nil
}
