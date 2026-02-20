package repository

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type PoolConfig struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetimeS int
	ConnMaxIdleTimeS int
}

func NewPostgresDB(ctx context.Context, databaseURL string, pool PoolConfig) (*sql.DB, error) {
	db, err := sql.Open("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("NewPostgresDB: open: %w", err)
	}

	db.SetMaxOpenConns(pool.MaxOpenConns)
	db.SetMaxIdleConns(pool.MaxIdleConns)
	db.SetConnMaxLifetime(time.Duration(pool.ConnMaxLifetimeS) * time.Second)
	db.SetConnMaxIdleTime(time.Duration(pool.ConnMaxIdleTimeS) * time.Second)

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("NewPostgresDB: ping: %w", err)
	}

	return db, nil
}
