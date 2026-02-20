package payment

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

func (s *Service) executeCrossCurrencyTransfer(ctx context.Context, req InternalTransferRequest, senderID, recipientID uuid.UUID) (*domain.Payment, error) {
	conversion, err := s.fx.Convert(ctx, req.Amount, req.SourceCurrency, req.DestCurrency)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: %w", err)
	}

	fxPoolSource, err := s.getSystemAccount(ctx, domain.AccountTypeFXPool, req.SourceCurrency)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: fx pool %s: %w", req.SourceCurrency, err)
	}
	fxPoolDest, err := s.getSystemAccount(ctx, domain.AccountTypeFXPool, req.DestCurrency)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: fx pool %s: %w", req.DestCurrency, err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: begin tx: %w", err)
	}
	defer tx.Rollback()

	locked, err := lockAccountsInOrder(ctx, tx, s.accounts, senderID, fxPoolSource.ID, fxPoolDest.ID, recipientID)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: %w", err)
	}

	sender := locked[senderID]
	recipient := locked[recipientID]
	fxSrc := locked[fxPoolSource.ID]
	fxDst := locked[fxPoolDest.ID]

	if err := verifyAccountActive(sender, "sender"); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: %w", err)
	}
	if err := verifyAccountActive(recipient, "recipient"); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: %w", err)
	}

	if sender.Balance < req.Amount {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: %w", domain.ErrInsufficientFunds)
	}
	if fxDst.Balance < conversion.DestAmount {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: fx pool %s: %w", req.DestCurrency, domain.ErrInsufficientFunds)
	}

	now := time.Now().UTC()
	exchangeRate := conversion.ExchangeRate
	feeCurrency := req.DestCurrency
	p := &domain.Payment{
		ID:              uuid.New(),
		IdempotencyKey:  req.IdempotencyKey,
		Type:            domain.PaymentTypeInternalTransfer,
		Status:          domain.PaymentStatusCompleted,
		SourceAccountID: senderID,
		DestAccountID:   &recipientID,
		SourceAmount:    req.Amount,
		SourceCurrency:  req.SourceCurrency,
		DestAmount:      conversion.DestAmount,
		DestCurrency:    req.DestCurrency,
		ExchangeRate:    &exchangeRate,
		FeeAmount:       conversion.FeeAmount,
		FeeCurrency:     &feeCurrency,
		CreatedAt:       now,
		UpdatedAt:       now,
		CompletedAt:     &now,
	}

	if err := s.payments.Create(ctx, tx, p); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: create payment: %w", err)
	}

	if err := s.writeCrossCurrencyLedgerEntries(ctx, tx, p, sender, fxSrc, fxDst, recipient); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: %w", err)
	}

	if err := s.writePaymentEvent(ctx, tx, p.ID, req.SenderUserID, now); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: %w", err)
	}

	if err := s.accounts.UpdateBalance(ctx, tx, sender.ID, sender.Balance-req.Amount, sender.Version+1); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: update sender: %w", err)
	}
	if err := s.accounts.UpdateBalance(ctx, tx, fxSrc.ID, fxSrc.Balance+req.Amount, fxSrc.Version+1); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: update fx pool source: %w", err)
	}
	if err := s.accounts.UpdateBalance(ctx, tx, fxDst.ID, fxDst.Balance-conversion.DestAmount, fxDst.Version+1); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: update fx pool dest: %w", err)
	}
	if err := s.accounts.UpdateBalance(ctx, tx, recipient.ID, recipient.Balance+conversion.DestAmount, recipient.Version+1); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: update recipient: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyTransfer: commit: %w", err)
	}

	return p, nil
}

func (s *Service) getSystemAccount(ctx context.Context, accountType domain.AccountType, currency domain.Currency) (*domain.Account, error) {
	acct, err := s.accounts.GetByUserAndCurrency(ctx, SystemUserID, currency, accountType)
	if err != nil {
		return nil, fmt.Errorf("getSystemAccount: %s %s: %w", accountType, currency, err)
	}
	return acct, nil
}

func (s *Service) writeCrossCurrencyLedgerEntries(
	ctx context.Context,
	tx *sql.Tx,
	p *domain.Payment,
	sender, fxPoolSource, fxPoolDest, recipient *domain.Account,
) error {
	entries := []struct {
		account   *domain.Account
		entryType domain.EntryType
		amount    int64
		currency  domain.Currency
		newBal    int64
	}{
		{sender, domain.EntryTypeDebit, p.SourceAmount, p.SourceCurrency, sender.Balance - p.SourceAmount},
		{fxPoolSource, domain.EntryTypeCredit, p.SourceAmount, p.SourceCurrency, fxPoolSource.Balance + p.SourceAmount},
		{fxPoolDest, domain.EntryTypeDebit, p.DestAmount, p.DestCurrency, fxPoolDest.Balance - p.DestAmount},
		{recipient, domain.EntryTypeCredit, p.DestAmount, p.DestCurrency, recipient.Balance + p.DestAmount},
	}

	for _, e := range entries {
		entry := &domain.LedgerEntry{
			ID:            uuid.New(),
			PaymentID:     p.ID,
			AccountID:     e.account.ID,
			EntryType:     e.entryType,
			Amount:        e.amount,
			Currency:      e.currency,
			BalanceBefore: e.account.Balance,
			BalanceAfter:  e.newBal,
			CreatedAt:     p.CreatedAt,
		}
		if err := s.ledger.Create(ctx, tx, entry); err != nil {
			return fmt.Errorf("writeCrossCurrencyLedgerEntries: %s %s: %w", e.entryType, e.account.ID, err)
		}
	}

	return nil
}
