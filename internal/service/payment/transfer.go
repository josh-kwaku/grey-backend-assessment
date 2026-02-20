package payment

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

type InternalTransferRequest struct {
	SenderUserID        uuid.UUID
	RecipientUniqueName string
	SourceCurrency      domain.Currency
	DestCurrency        domain.Currency
	Amount              int64
	IdempotencyKey      string
}

func (s *Service) CreateInternalTransfer(ctx context.Context, req InternalTransferRequest) (*domain.Payment, error) {
	log := logging.FromContext(ctx)

	senderAcct, recipientAcct, err := s.resolveTransferAccounts(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("CreateInternalTransfer: %w", err)
	}

	if err := s.validateTransfer(req, senderAcct, recipientAcct); err != nil {
		return nil, fmt.Errorf("CreateInternalTransfer: %w", err)
	}

	p, err := s.executeTransfer(ctx, req, senderAcct.ID, recipientAcct.ID)
	if err != nil {
		if errors.Is(err, domain.ErrDuplicateIdempotencyKey) {
			return nil, fmt.Errorf("CreateInternalTransfer: %w", domain.ErrDuplicatePayment)
		}
		return nil, fmt.Errorf("CreateInternalTransfer: %w", err)
	}

	log.Info("internal transfer completed",
		"payment_id", p.ID,
		"sender_account", senderAcct.ID,
		"recipient_account", recipientAcct.ID,
		"source_amount", req.Amount,
		"source_currency", req.SourceCurrency,
		"dest_amount", p.DestAmount,
		"dest_currency", req.DestCurrency,
	)

	return p, nil
}


func (s *Service) resolveTransferAccounts(ctx context.Context, req InternalTransferRequest) (*domain.Account, *domain.Account, error) {
	recipient, err := s.users.GetByUniqueName(ctx, req.RecipientUniqueName)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil, fmt.Errorf("resolveTransferAccounts: %w", domain.ErrRecipientNotFound)
		}
		return nil, nil, fmt.Errorf("resolveTransferAccounts: %w", err)
	}

	recipientAcct, err := s.accounts.GetByUserAndCurrency(ctx, recipient.ID, req.DestCurrency, domain.AccountTypeUser)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil, fmt.Errorf("resolveTransferAccounts: recipient has no %s account: %w", req.DestCurrency, domain.ErrAccountNotFound)
		}
		return nil, nil, fmt.Errorf("resolveTransferAccounts: %w", err)
	}

	senderAcct, err := s.accounts.GetByUserAndCurrency(ctx, req.SenderUserID, req.SourceCurrency, domain.AccountTypeUser)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, nil, fmt.Errorf("resolveTransferAccounts: %w", domain.ErrAccountNotFound)
		}
		return nil, nil, fmt.Errorf("resolveTransferAccounts: %w", err)
	}

	return senderAcct, recipientAcct, nil
}

func (s *Service) validateTransfer(req InternalTransferRequest, sender, recipient *domain.Account) error {
	if req.Amount <= 0 {
		return fmt.Errorf("validateTransfer: %w", domain.ErrInvalidAmount)
	}

	if sender.UserID == recipient.UserID && req.SourceCurrency == req.DestCurrency {
		return fmt.Errorf("validateTransfer: %w", domain.ErrSelfTransfer)
	}

	if sender.Status == domain.AccountStatusFrozen {
		return fmt.Errorf("validateTransfer: sender: %w", domain.ErrAccountFrozen)
	}
	if sender.Status != domain.AccountStatusActive {
		return fmt.Errorf("validateTransfer: sender: %w", domain.ErrAccountClosed)
	}

	if recipient.Status == domain.AccountStatusFrozen {
		return fmt.Errorf("validateTransfer: recipient: %w", domain.ErrAccountFrozen)
	}
	if recipient.Status != domain.AccountStatusActive {
		return fmt.Errorf("validateTransfer: recipient: %w", domain.ErrAccountClosed)
	}

	if req.Amount > s.txLimitForCurrency(req.SourceCurrency) {
		return fmt.Errorf("validateTransfer: %w", domain.ErrLimitExceeded)
	}

	return nil
}

func (s *Service) executeTransfer(ctx context.Context, req InternalTransferRequest, senderID, recipientID uuid.UUID) (*domain.Payment, error) {
	if req.SourceCurrency != req.DestCurrency {
		return s.executeCrossCurrencyTransfer(ctx, req, senderID, recipientID)
	}
	return s.executeSameCurrencyTransfer(ctx, req, senderID, recipientID)
}

func (s *Service) executeSameCurrencyTransfer(ctx context.Context, req InternalTransferRequest, senderID, recipientID uuid.UUID) (*domain.Payment, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: begin tx: %w", err)
	}
	defer tx.Rollback()

	locked, err := lockAccountsInOrder(ctx, tx, s.accounts, senderID, recipientID)
	if err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: %w", err)
	}

	sender, recipient := locked[senderID], locked[recipientID]

	if err := verifyAccountActive(sender, "sender"); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: %w", err)
	}
	if err := verifyAccountActive(recipient, "recipient"); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: %w", err)
	}

	if sender.Balance < req.Amount {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: %w", domain.ErrInsufficientFunds)
	}

	now := time.Now().UTC()
	p := &domain.Payment{
		ID:              uuid.New(),
		IdempotencyKey:  req.IdempotencyKey,
		Type:            domain.PaymentTypeInternalTransfer,
		Status:          domain.PaymentStatusCompleted,
		SourceAccountID: senderID,
		DestAccountID:   &recipientID,
		SourceAmount:    req.Amount,
		SourceCurrency:  req.SourceCurrency,
		DestAmount:      req.Amount,
		DestCurrency:    req.DestCurrency,
		CreatedAt:       now,
		UpdatedAt:       now,
		CompletedAt:     &now,
	}

	if err := s.payments.Create(ctx, tx, p); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: create payment: %w", err)
	}

	if err := s.writeLedgerEntries(ctx, tx, p, sender, recipient); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: %w", err)
	}

	if err := s.writePaymentEvent(ctx, tx, p.ID, domain.PaymentEventTypeCompleted, req.SenderUserID, now); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: %w", err)
	}

	if err := s.accounts.UpdateBalance(ctx, tx, senderID, sender.Balance-req.Amount, sender.Version+1); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: update sender: %w", err)
	}
	if err := s.accounts.UpdateBalance(ctx, tx, recipientID, recipient.Balance+req.Amount, recipient.Version+1); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: update recipient: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("executeSameCurrencyTransfer: commit: %w", err)
	}

	return p, nil
}

func verifyAccountActive(acct *domain.Account, role string) error {
	if acct.Status == domain.AccountStatusFrozen {
		return fmt.Errorf("%s: %w", role, domain.ErrAccountFrozen)
	}
	if acct.Status != domain.AccountStatusActive {
		return fmt.Errorf("%s: %w", role, domain.ErrAccountClosed)
	}
	return nil
}

func (s *Service) writeLedgerEntries(ctx context.Context, tx *sql.Tx, p *domain.Payment, sender, recipient *domain.Account) error {
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
		return fmt.Errorf("writeLedgerEntries: debit: %w", err)
	}

	credit := &domain.LedgerEntry{
		ID:            uuid.New(),
		PaymentID:     p.ID,
		AccountID:     recipient.ID,
		EntryType:     domain.EntryTypeCredit,
		Amount:        p.DestAmount,
		Currency:      p.DestCurrency,
		BalanceBefore: recipient.Balance,
		BalanceAfter:  recipient.Balance + p.DestAmount,
		CreatedAt:     p.CreatedAt,
	}
	if err := s.ledger.Create(ctx, tx, credit); err != nil {
		return fmt.Errorf("writeLedgerEntries: credit: %w", err)
	}

	return nil
}

func (s *Service) writePaymentEvent(ctx context.Context, tx *sql.Tx, paymentID uuid.UUID, eventType domain.PaymentEventType, actorUserID uuid.UUID, now time.Time) error {
	event := &domain.PaymentEvent{
		ID:        uuid.New(),
		PaymentID: paymentID,
		EventType: eventType,
		Actor:     fmt.Sprintf("user:%s", actorUserID),
		CreatedAt: now,
	}
	if err := s.events.Create(ctx, tx, event); err != nil {
		return fmt.Errorf("writePaymentEvent: %w", err)
	}
	return nil
}

func lockAccountsInOrder(ctx context.Context, tx *sql.Tx, accounts accountRepo, ids ...uuid.UUID) (map[uuid.UUID]*domain.Account, error) {
	sorted := make([]uuid.UUID, len(ids))
	copy(sorted, ids)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].String() < sorted[j].String()
	})

	result := make(map[uuid.UUID]*domain.Account, len(ids))
	for _, id := range sorted {
		acct, err := accounts.GetForUpdate(ctx, tx, id)
		if err != nil {
			return nil, fmt.Errorf("lockAccountsInOrder: %w", err)
		}
		result[id] = acct
	}
	return result, nil
}
