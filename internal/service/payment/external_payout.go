package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

type ExternalPayoutRequest struct {
	SenderUserID   uuid.UUID
	SourceCurrency domain.Currency
	DestCurrency   domain.Currency
	Amount         int64
	DestIBAN       string
	DestBankName   string
	IdempotencyKey string
}

func (s *Service) CreateExternalPayout(ctx context.Context, req ExternalPayoutRequest) (*domain.Payment, error) {
	log := logging.FromContext(ctx)

	senderAcct, err := s.accounts.GetByUserAndCurrency(ctx, req.SenderUserID, req.SourceCurrency, domain.AccountTypeUser)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, fmt.Errorf("CreateExternalPayout: %w", domain.ErrAccountNotFound)
		}
		return nil, fmt.Errorf("CreateExternalPayout: %w", err)
	}

	existing, err := s.checkExternalPayoutIdempotency(ctx, req, senderAcct.ID)
	if err != nil {
		return nil, fmt.Errorf("CreateExternalPayout: %w", err)
	}
	if existing != nil {
		log.Info("idempotent replay", "payment_id", existing.ID, "idempotency_key", req.IdempotencyKey)
		return existing, nil
	}

	if err := s.validateExternalPayout(req, senderAcct); err != nil {
		return nil, fmt.Errorf("CreateExternalPayout: %w", err)
	}

	p, err := s.executeExternalPayout(ctx, req, senderAcct.ID)
	if err != nil {
		if errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
			existing, idempErr := s.checkExternalPayoutIdempotency(ctx, req, senderAcct.ID)
			if idempErr != nil {
				return nil, fmt.Errorf("CreateExternalPayout: %w", idempErr)
			}
			if existing != nil {
				log.Info("idempotent replay (race)", "payment_id", existing.ID, "idempotency_key", req.IdempotencyKey)
				return existing, nil
			}
			return nil, fmt.Errorf("CreateExternalPayout: %w", domain.ErrDuplicatePayment)
		}
		return nil, fmt.Errorf("CreateExternalPayout: %w", err)
	}

	s.submitToProvider(ctx, p)

	log.Info("external payout created",
		"payment_id", p.ID,
		"sender_account", senderAcct.ID,
		"source_amount", req.Amount,
		"source_currency", req.SourceCurrency,
		"dest_amount", p.DestAmount,
		"dest_currency", req.DestCurrency,
	)

	return p, nil
}

func (s *Service) checkExternalPayoutIdempotency(ctx context.Context, req ExternalPayoutRequest, senderAcctID uuid.UUID) (*domain.Payment, error) {
	existing, err := s.payments.GetByIdempotencyKey(ctx, req.IdempotencyKey)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("checkExternalPayoutIdempotency: %w", err)
	}

	if existing.SourceAccountID == senderAcctID &&
		existing.SourceAmount == req.Amount &&
		existing.SourceCurrency == req.SourceCurrency &&
		existing.DestCurrency == req.DestCurrency &&
		existing.Type == domain.PaymentTypeExternalPayout {
		return existing, nil
	}

	return nil, fmt.Errorf("checkExternalPayoutIdempotency: %w", domain.ErrDuplicatePayment)
}

func (s *Service) validateExternalPayout(req ExternalPayoutRequest, sender *domain.Account) error {
	if req.Amount <= 0 {
		return fmt.Errorf("validateExternalPayout: %w", domain.ErrInvalidAmount)
	}

	if req.DestIBAN == "" {
		return fmt.Errorf("validateExternalPayout: dest IBAN required: %w", domain.ErrInvalidRequest)
	}
	if req.DestBankName == "" {
		return fmt.Errorf("validateExternalPayout: dest bank name required: %w", domain.ErrInvalidRequest)
	}

	if sender.Status == domain.AccountStatusFrozen {
		return fmt.Errorf("validateExternalPayout: %w", domain.ErrAccountFrozen)
	}
	if sender.Status != domain.AccountStatusActive {
		return fmt.Errorf("validateExternalPayout: %w", domain.ErrAccountClosed)
	}

	if req.Amount > s.txLimitForCurrency(req.SourceCurrency) {
		return fmt.Errorf("validateExternalPayout: %w", domain.ErrLimitExceeded)
	}

	return nil
}

func (s *Service) executeExternalPayout(ctx context.Context, req ExternalPayoutRequest, senderID uuid.UUID) (*domain.Payment, error) {
	if req.SourceCurrency != req.DestCurrency {
		return s.executeCrossCurrencyExternalPayout(ctx, req, senderID)
	}
	return s.executeSameCurrencyExternalPayout(ctx, req, senderID)
}

func (s *Service) executeSameCurrencyExternalPayout(ctx context.Context, req ExternalPayoutRequest, senderID uuid.UUID) (*domain.Payment, error) {
	outgoing, err := s.getSystemAccount(ctx, domain.AccountTypeOutgoing, req.DestCurrency)
	if err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: %w", err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: begin tx: %w", err)
	}
	defer tx.Rollback()

	locked, err := lockAccountsInOrder(ctx, tx, s.accounts, senderID, outgoing.ID)
	if err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: %w", err)
	}

	sender := locked[senderID]
	outgoingAcct := locked[outgoing.ID]

	if err := verifyAccountActive(sender, "sender"); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: %w", err)
	}
	if sender.Balance < req.Amount {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: %w", domain.ErrInsufficientFunds)
	}

	now := time.Now().UTC()
	p := buildExternalPayment(req, senderID, req.Amount, nil, nil, now)

	if err := s.payments.Create(ctx, tx, p); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: create payment: %w", err)
	}

	if err := s.writeExternalLedgerEntries(ctx, tx, p, sender, outgoingAcct); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: %w", err)
	}

	if err := s.writePaymentEvent(ctx, tx, p.ID, domain.PaymentEventTypeCreated, req.SenderUserID, now); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: %w", err)
	}

	if err := s.accounts.UpdateBalance(ctx, tx, sender.ID, sender.Balance-req.Amount, sender.Version+1); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: update sender: %w", err)
	}
	if err := s.accounts.UpdateBalance(ctx, tx, outgoingAcct.ID, outgoingAcct.Balance+req.Amount, outgoingAcct.Version+1); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: update outgoing: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyExternalPayout: commit: %w", err)
	}

	return p, nil
}

func buildExternalPayment(req ExternalPayoutRequest, senderID uuid.UUID, destAmount int64, exchangeRate *decimal.Decimal, feeCurrency *domain.Currency, now time.Time) *domain.Payment {
	return &domain.Payment{
		ID:              uuid.New(),
		IdempotencyKey:  req.IdempotencyKey,
		Type:            domain.PaymentTypeExternalPayout,
		Status:          domain.PaymentStatusPending,
		SourceAccountID: senderID,
		DestIBAN:        &req.DestIBAN,
		DestBankName:    &req.DestBankName,
		SourceAmount:    req.Amount,
		SourceCurrency:  req.SourceCurrency,
		DestAmount:      destAmount,
		DestCurrency:    req.DestCurrency,
		ExchangeRate:    exchangeRate,
		FeeCurrency:     feeCurrency,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func (s *Service) writeExternalLedgerEntries(ctx context.Context, tx *sql.Tx, p *domain.Payment, sender, outgoing *domain.Account) error {
	debit := &domain.LedgerEntry{
		ID:            uuid.New(),
		PaymentID:     p.ID,
		AccountID:     sender.ID,
		EntryType:     domain.EntryTypeDebit,
		Amount:        p.SourceAmount,
		Currency:      p.SourceCurrency,
		BalanceBefore: sender.Balance,
		BalanceAfter:  sender.Balance - p.SourceAmount,
		CreatedAt:     p.CreatedAt,
	}
	if err := s.ledger.Create(ctx, tx, debit); err != nil {
		return fmt.Errorf("writeExternalLedgerEntries: debit: %w", err)
	}

	credit := &domain.LedgerEntry{
		ID:            uuid.New(),
		PaymentID:     p.ID,
		AccountID:     outgoing.ID,
		EntryType:     domain.EntryTypeCredit,
		Amount:        p.DestAmount,
		Currency:      p.DestCurrency,
		BalanceBefore: outgoing.Balance,
		BalanceAfter:  outgoing.Balance + p.DestAmount,
		CreatedAt:     p.CreatedAt,
	}
	if err := s.ledger.Create(ctx, tx, credit); err != nil {
		return fmt.Errorf("writeExternalLedgerEntries: credit: %w", err)
	}

	return nil
}

func (s *Service) submitToProvider(ctx context.Context, p *domain.Payment) {
	if s.provider == nil {
		return
	}

	log := logging.FromContext(ctx)
	err := s.provider.SubmitPayment(ctx, ProviderRequest{
		PaymentID:    p.ID,
		Amount:       p.DestAmount,
		Currency:     p.DestCurrency,
		DestIBAN:     stringVal(p.DestIBAN),
		DestBankName: stringVal(p.DestBankName),
	})
	if err != nil {
		log.Warn("failed to submit to provider, payment stays pending",
			"payment_id", p.ID,
			"error", err,
		)
	}
}

func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
