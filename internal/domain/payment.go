package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type PaymentType string

const (
	PaymentTypeInternalTransfer PaymentType = "internal_transfer"
	PaymentTypeExternalPayout   PaymentType = "external_payout"
)

type PaymentStatus string

const (
	PaymentStatusPending    PaymentStatus = "pending"
	PaymentStatusProcessing PaymentStatus = "processing"
	PaymentStatusCompleted  PaymentStatus = "completed"
	PaymentStatusFailed     PaymentStatus = "failed"
	PaymentStatusReversed   PaymentStatus = "reversed"
)

type Payment struct {
	ID               uuid.UUID
	IdempotencyKey   string
	Type             PaymentType
	Status           PaymentStatus
	SourceAccountID  uuid.UUID
	DestAccountID    *uuid.UUID
	DestAccountNumber *string
	DestIBAN         *string
	DestSwiftBIC     *string
	DestBankName     *string
	SourceAmount     int64
	SourceCurrency   Currency
	DestAmount       int64
	DestCurrency     Currency
	ExchangeRate     *decimal.Decimal
	FeeAmount        int64
	FeeCurrency      *Currency
	Provider         *string
	ProviderRef      *string
	FailureReason    *string
	Metadata         json.RawMessage
	CreatedAt        time.Time
	UpdatedAt        time.Time
	CompletedAt      *time.Time
}
