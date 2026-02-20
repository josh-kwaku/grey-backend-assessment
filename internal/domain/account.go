package domain

import (
	"time"

	"github.com/google/uuid"
)

type Currency string

const (
	CurrencyUSD Currency = "USD"
	CurrencyEUR Currency = "EUR"
	CurrencyGBP Currency = "GBP"
)

type AccountType string

const (
	AccountTypeUser     AccountType = "user"
	AccountTypeFXPool   AccountType = "fx_pool"
	AccountTypeOutgoing AccountType = "outgoing"
)

type AccountStatus string

const (
	AccountStatusPending AccountStatus = "pending"
	AccountStatusActive  AccountStatus = "active"
	AccountStatusFrozen  AccountStatus = "frozen"
	AccountStatusClosed  AccountStatus = "closed"
)

type Account struct {
	ID            uuid.UUID
	UserID        uuid.UUID
	Currency      Currency
	AccountType   AccountType
	Balance       int64
	Version       int64
	AccountNumber *string
	RoutingNumber *string
	IBAN          *string
	SwiftBIC      *string
	Provider      *string
	ProviderRef   *string
	Status        AccountStatus
	CreatedAt     time.Time
}
