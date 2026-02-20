package payment

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/config"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/fx"
)

var SystemUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

type paymentRepo interface {
	Create(ctx context.Context, tx *sql.Tx, payment *domain.Payment) error
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	GetByIdempotencyKey(ctx context.Context, key string) (*domain.Payment, error)
}

type accountRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	GetByUserAndCurrency(ctx context.Context, userID uuid.UUID, currency domain.Currency, accountType domain.AccountType) (*domain.Account, error)
	GetForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Account, error)
	UpdateBalance(ctx context.Context, tx *sql.Tx, id uuid.UUID, newBalance int64, newVersion int64) error
}

type ledgerRepo interface {
	Create(ctx context.Context, tx *sql.Tx, entry *domain.LedgerEntry) error
}

type eventRepo interface {
	Create(ctx context.Context, tx *sql.Tx, event *domain.PaymentEvent) error
}

type userRepo interface {
	GetByUniqueName(ctx context.Context, uniqueName string) (*domain.User, error)
}

type fxService interface {
	Convert(ctx context.Context, amount int64, from, to domain.Currency) (*fx.Conversion, error)
}

type Service struct {
	payments paymentRepo
	accounts accountRepo
	ledger   ledgerRepo
	events   eventRepo
	users    userRepo
	fx       fxService
	db       *sql.DB
	config   *config.Config
}

func NewService(
	payments paymentRepo,
	accounts accountRepo,
	ledger ledgerRepo,
	events eventRepo,
	users userRepo,
	fxSvc fxService,
	db *sql.DB,
	cfg *config.Config,
) *Service {
	return &Service{
		payments: payments,
		accounts: accounts,
		ledger:   ledger,
		events:   events,
		users:    users,
		fx:       fxSvc,
		db:       db,
		config:   cfg,
	}
}

func (s *Service) GetPaymentByID(ctx context.Context, paymentID uuid.UUID) (*domain.Payment, error) {
	p, err := s.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("GetPaymentByID: %w", err)
	}
	return p, nil
}

func (s *Service) GetPaymentForUser(ctx context.Context, paymentID, userID uuid.UUID) (*domain.Payment, error) {
	p, err := s.payments.GetByID(ctx, paymentID)
	if err != nil {
		return nil, fmt.Errorf("GetPaymentForUser: %w", err)
	}

	acct, err := s.accounts.GetByID(ctx, p.SourceAccountID)
	if err != nil {
		return nil, fmt.Errorf("GetPaymentForUser: %w", err)
	}

	if acct.UserID != userID {
		return nil, fmt.Errorf("GetPaymentForUser: %w", domain.ErrNotFound)
	}

	return p, nil
}

func (s *Service) txLimitForCurrency(c domain.Currency) int64 {
	switch c {
	case domain.CurrencyUSD:
		return s.config.TxLimitUSD
	case domain.CurrencyEUR:
		return s.config.TxLimitEUR
	case domain.CurrencyGBP:
		return s.config.TxLimitGBP
	default:
		return 0
	}
}
