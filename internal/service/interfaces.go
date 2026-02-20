package service

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

type userRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
	GetByUniqueName(ctx context.Context, uniqueName string) (*domain.User, error)
}

type accountRepository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	GetByUserAndCurrency(ctx context.Context, userID uuid.UUID, currency domain.Currency, accountType domain.AccountType) (*domain.Account, error)
	GetByUserID(ctx context.Context, userID uuid.UUID) ([]domain.Account, error)
	Create(ctx context.Context, account *domain.Account) error
	GetForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Account, error)
	UpdateBalance(ctx context.Context, tx *sql.Tx, id uuid.UUID, newBalance int64, newVersion int64) error
}

type paymentRepository interface {
	Create(ctx context.Context, tx *sql.Tx, payment *domain.Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error)
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.PaymentStatus, providerRef *string, failureReason *string, completedAt *time.Time) error
}

type ledgerRepository interface {
	Create(ctx context.Context, tx *sql.Tx, entry *domain.LedgerEntry) error
	GetByAccountID(ctx context.Context, accountID uuid.UUID, limit, offset int) ([]domain.LedgerEntry, int, error)
	GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]domain.LedgerEntry, error)
}

type paymentEventRepository interface {
	Create(ctx context.Context, tx *sql.Tx, event *domain.PaymentEvent) error
	GetByPaymentID(ctx context.Context, paymentID uuid.UUID) ([]domain.PaymentEvent, error)
}

type webhookEventRepository interface {
	Create(ctx context.Context, event *domain.WebhookEvent) error
	GetPending(ctx context.Context, limit int) ([]domain.WebhookEvent, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WebhookEventStatus) error
}
