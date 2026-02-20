package payment

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

func (s *Service) executeCrossCurrencyExternalPayout(ctx context.Context, req ExternalPayoutRequest, senderID uuid.UUID) (*domain.Payment, error) {
	conversion, err := s.fx.Convert(ctx, req.Amount, req.SourceCurrency, req.DestCurrency)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: %w", err)
	}

	fxPoolSource, err := s.getSystemAccount(ctx, domain.AccountTypeFXPool, req.SourceCurrency)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: fx pool %s: %w", req.SourceCurrency, err)
	}
	fxPoolDest, err := s.getSystemAccount(ctx, domain.AccountTypeFXPool, req.DestCurrency)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: fx pool %s: %w", req.DestCurrency, err)
	}
	outgoing, err := s.getSystemAccount(ctx, domain.AccountTypeOutgoing, req.DestCurrency)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: outgoing %s: %w", req.DestCurrency, err)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: begin tx: %w", err)
	}
	defer tx.Rollback()

	locked, err := lockAccountsInOrder(ctx, tx, s.accounts, senderID, fxPoolSource.ID, fxPoolDest.ID, outgoing.ID)
	if err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: %w", err)
	}

	sender := locked[senderID]
	fxSrc := locked[fxPoolSource.ID]
	fxDst := locked[fxPoolDest.ID]
	outgoingAcct := locked[outgoing.ID]

	if err := verifyAccountActive(sender, "sender"); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: %w", err)
	}
	if sender.Balance < req.Amount {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: %w", domain.ErrInsufficientFunds)
	}
	if fxDst.Balance < conversion.DestAmount {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: fx pool %s: %w", req.DestCurrency, domain.ErrInsufficientFunds)
	}

	now := time.Now().UTC()
	exchangeRate := conversion.ExchangeRate
	feeCurrency := req.DestCurrency
	p := buildExternalPayment(req, senderID, conversion.DestAmount, &exchangeRate, &feeCurrency, now)
	p.FeeAmount = conversion.FeeAmount

	if err := s.payments.Create(ctx, tx, p); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: create payment: %w", err)
	}

	if err := s.writeCrossCurrencyExternalLedgerEntries(ctx, tx, p, sender, fxSrc, fxDst, outgoingAcct); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: %w", err)
	}

	if err := s.writePaymentEvent(ctx, tx, p.ID, domain.PaymentEventTypeCreated, req.SenderUserID, now); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: %w", err)
	}

	if err := s.accounts.UpdateBalance(ctx, tx, sender.ID, sender.Balance-req.Amount, sender.Version+1); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: update sender: %w", err)
	}
	if err := s.accounts.UpdateBalance(ctx, tx, fxSrc.ID, fxSrc.Balance+req.Amount, fxSrc.Version+1); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: update fx source: %w", err)
	}
	if err := s.accounts.UpdateBalance(ctx, tx, fxDst.ID, fxDst.Balance-conversion.DestAmount, fxDst.Version+1); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: update fx dest: %w", err)
	}
	if err := s.accounts.UpdateBalance(ctx, tx, outgoingAcct.ID, outgoingAcct.Balance+conversion.DestAmount, outgoingAcct.Version+1); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: update outgoing: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("executeCrossCurrencyExternalPayout: commit: %w", err)
	}

	return p, nil
}

func (s *Service) writeCrossCurrencyExternalLedgerEntries(
	ctx context.Context,
	tx *sql.Tx,
	p *domain.Payment,
	sender, fxPoolSource, fxPoolDest, outgoing *domain.Account,
) error {
	entries := []struct {
		account   *domain.Account
		entryType domain.EntryType
		amount    int64
		currency  domain.Currency
	}{
		{sender, domain.EntryTypeDebit, p.SourceAmount, p.SourceCurrency},
		{fxPoolSource, domain.EntryTypeCredit, p.SourceAmount, p.SourceCurrency},
		{fxPoolDest, domain.EntryTypeDebit, p.DestAmount, p.DestCurrency},
		{outgoing, domain.EntryTypeCredit, p.DestAmount, p.DestCurrency},
	}

	for _, e := range entries {
		var newBal int64
		if e.entryType == domain.EntryTypeDebit {
			newBal = e.account.Balance - e.amount
		} else {
			newBal = e.account.Balance + e.amount
		}

		entry := &domain.LedgerEntry{
			ID:            uuid.New(),
			PaymentID:     p.ID,
			AccountID:     e.account.ID,
			EntryType:     e.entryType,
			Amount:        e.amount,
			Currency:      e.currency,
			BalanceBefore: e.account.Balance,
			BalanceAfter:  newBal,
			CreatedAt:     p.CreatedAt,
		}
		if err := s.ledger.Create(ctx, tx, entry); err != nil {
			return fmt.Errorf("writeCrossCurrencyExternalLedgerEntries: %s %s: %w", e.entryType, e.account.ID, err)
		}
	}

	return nil
}
