package domain

import (
	"time"

	"github.com/google/uuid"
)

type EntryType string

const (
	EntryTypeDebit  EntryType = "debit"
	EntryTypeCredit EntryType = "credit"
)

type LedgerEntry struct {
	ID            uuid.UUID
	PaymentID     uuid.UUID
	AccountID     uuid.UUID
	EntryType     EntryType
	Amount        int64
	Currency      Currency
	BalanceBefore int64
	BalanceAfter  int64
	CreatedAt     time.Time
}
