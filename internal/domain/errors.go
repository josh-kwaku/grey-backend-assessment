package domain

import "errors"

var (
	ErrNotFound                 = errors.New("not found")
	ErrInsufficientFunds        = errors.New("insufficient funds")
	ErrAccountFrozen            = errors.New("account frozen")
	ErrDuplicatePayment         = errors.New("duplicate payment")
	ErrSelfTransfer             = errors.New("cannot transfer to same account")
	ErrInvalidCurrency          = errors.New("invalid currency")
	ErrInvalidAmount            = errors.New("amount must be greater than zero")
	ErrRecipientNotFound        = errors.New("recipient not found")
	ErrAccountNotFound          = errors.New("account not found")
	ErrLimitExceeded            = errors.New("transaction limit exceeded")
	ErrAccountExists            = errors.New("account already exists for this currency")
	ErrAccountClosed            = errors.New("account closed")
	ErrCurrencyMismatch         = errors.New("currency mismatch")
	ErrVersionConflict          = errors.New("optimistic lock conflict")
	ErrInvalidRequest           = errors.New("invalid request")
	ErrPaymentTerminal          = errors.New("payment already in terminal state")
	ErrDuplicateIdempotencyKey  = errors.New("duplicate idempotency key")
)
