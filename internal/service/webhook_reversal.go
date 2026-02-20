package service

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/service/payment"
)

func (p *WebhookProcessor) writeSameCurrencyReversal(
	ctx context.Context,
	tx *sql.Tx,
	pmt *domain.Payment,
	locked map[uuid.UUID]*domain.Account,
	outgoingID uuid.UUID,
	now time.Time,
) error {
	sender := locked[pmt.SourceAccountID]
	outgoing := locked[outgoingID]

	entries := []reversalEntry{
		{outgoing, domain.EntryTypeDebit, pmt.DestAmount, pmt.DestCurrency},
		{sender, domain.EntryTypeCredit, pmt.SourceAmount, pmt.SourceCurrency},
	}

	return p.writeReversalEntries(ctx, tx, pmt.ID, entries, now)
}

func (p *WebhookProcessor) writeCrossCurrencyReversal(
	ctx context.Context,
	tx *sql.Tx,
	pmt *domain.Payment,
	locked map[uuid.UUID]*domain.Account,
	outgoingID, fxPoolSourceID, fxPoolDestID uuid.UUID,
	now time.Time,
) error {
	sender := locked[pmt.SourceAccountID]
	outgoing := locked[outgoingID]
	fxPoolSource := locked[fxPoolSourceID]
	fxPoolDest := locked[fxPoolDestID]

	// Reverse the original 4 entries:
	// Original: debit sender, credit FX source, debit FX dest, credit outgoing
	// Reversal: debit outgoing, credit FX dest, debit FX source, credit sender
	entries := []reversalEntry{
		{outgoing, domain.EntryTypeDebit, pmt.DestAmount, pmt.DestCurrency},
		{fxPoolDest, domain.EntryTypeCredit, pmt.DestAmount, pmt.DestCurrency},
		{fxPoolSource, domain.EntryTypeDebit, pmt.SourceAmount, pmt.SourceCurrency},
		{sender, domain.EntryTypeCredit, pmt.SourceAmount, pmt.SourceCurrency},
	}

	return p.writeReversalEntries(ctx, tx, pmt.ID, entries, now)
}

type reversalEntry struct {
	account   *domain.Account
	entryType domain.EntryType
	amount    int64
	currency  domain.Currency
}

func (p *WebhookProcessor) writeReversalEntries(
	ctx context.Context,
	tx *sql.Tx,
	paymentID uuid.UUID,
	entries []reversalEntry,
	now time.Time,
) error {
	for _, e := range entries {
		var newBalance int64
		if e.entryType == domain.EntryTypeDebit {
			newBalance = e.account.Balance - e.amount
		} else {
			newBalance = e.account.Balance + e.amount
		}

		entry := &domain.LedgerEntry{
			ID:            uuid.New(),
			PaymentID:     paymentID,
			AccountID:     e.account.ID,
			EntryType:     e.entryType,
			Amount:        e.amount,
			Currency:      e.currency,
			BalanceBefore: e.account.Balance,
			BalanceAfter:  newBalance,
			CreatedAt:     now,
		}
		if err := p.ledger.Create(ctx, tx, entry); err != nil {
			return fmt.Errorf("writeReversalEntries: %s %s: %w", e.entryType, e.account.ID, err)
		}

		if err := p.accounts.UpdateBalance(ctx, tx, e.account.ID, newBalance, e.account.Version+1); err != nil {
			return fmt.Errorf("writeReversalEntries: update %s: %w", e.account.ID, err)
		}
	}

	return nil
}

func (p *WebhookProcessor) getSystemAccount(ctx context.Context, accountType domain.AccountType, currency domain.Currency) (*domain.Account, error) {
	acct, err := p.accounts.GetByUserAndCurrency(ctx, payment.SystemUserID, currency, accountType)
	if err != nil {
		return nil, fmt.Errorf("getSystemAccount: %s %s: %w", accountType, currency, err)
	}
	return acct, nil
}

func isTerminalStatus(s domain.PaymentStatus) bool {
	switch s {
	case domain.PaymentStatusCompleted, domain.PaymentStatusFailed, domain.PaymentStatusReversed:
		return true
	default:
		return false
	}
}

func lockAccountsInOrder(ctx context.Context, tx *sql.Tx, accounts wpAccountRepo, ids ...uuid.UUID) (map[uuid.UUID]*domain.Account, error) {
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
