package handler

import "net/http"

type AppError struct {
	Status  int
	Code    string
	Message string
}

func (e *AppError) Error() string { return e.Message }

var (
	ErrMissingToken       = &AppError{http.StatusUnauthorized, "MISSING_TOKEN", "Authorization header required"}
	ErrInvalidToken       = &AppError{http.StatusUnauthorized, "INVALID_TOKEN", "Token is invalid or expired"}
	ErrInvalidCredentials = &AppError{http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password"}
	ErrInvalidRequest     = &AppError{http.StatusBadRequest, "INVALID_REQUEST", "Invalid request body"}
	ErrValidationFailed   = &AppError{http.StatusBadRequest, "VALIDATION_FAILED", "Validation failed"}
	ErrResourceNotFound   = &AppError{http.StatusNotFound, "RESOURCE_NOT_FOUND", "Resource not found"}
	ErrInternalError      = &AppError{http.StatusInternalServerError, "INTERNAL_ERROR", "An unexpected error occurred"}

	ErrInsufficientFunds = &AppError{http.StatusUnprocessableEntity, "INSUFFICIENT_FUNDS", "Insufficient funds"}
	ErrAccountFrozen     = &AppError{http.StatusUnprocessableEntity, "ACCOUNT_FROZEN", "Account is frozen"}
	ErrDuplicatePayment  = &AppError{http.StatusConflict, "DUPLICATE_PAYMENT", "Duplicate payment"}
	ErrSelfTransfer      = &AppError{http.StatusUnprocessableEntity, "SELF_TRANSFER_NOT_ALLOWED", "Cannot transfer to the same account"}
	ErrLimitExceeded     = &AppError{http.StatusUnprocessableEntity, "TRANSACTION_LIMIT_EXCEEDED", "Transaction limit exceeded"}
	ErrRecipientNotFound = &AppError{http.StatusUnprocessableEntity, "RECIPIENT_NOT_FOUND", "Recipient not found"}
	ErrAccountNotFound   = &AppError{http.StatusUnprocessableEntity, "ACCOUNT_NOT_FOUND", "Account not found"}
	ErrAccountExists     = &AppError{http.StatusConflict, "ACCOUNT_ALREADY_EXISTS", "Account already exists for this currency"}
	ErrInvalidCurrency   = &AppError{http.StatusBadRequest, "INVALID_CURRENCY", "Invalid currency"}
	ErrAccountClosed     = &AppError{http.StatusUnprocessableEntity, "ACCOUNT_CLOSED", "Account is closed"}
	ErrCurrencyMismatch  = &AppError{http.StatusUnprocessableEntity, "CURRENCY_MISMATCH", "Currency mismatch"}
	ErrVersionConflict          = &AppError{http.StatusConflict, "VERSION_CONFLICT", "Resource was modified concurrently, please retry"}
	ErrMissingIdempotencyKey    = &AppError{http.StatusBadRequest, "MISSING_IDEMPOTENCY_KEY", "Idempotency-Key header is required"}
	ErrIdempotencyConflict      = &AppError{http.StatusConflict, "IDEMPOTENCY_CONFLICT", "Idempotency key already used with a different request"}
	ErrInvalidAmount            = &AppError{http.StatusBadRequest, "INVALID_AMOUNT", "Amount must be greater than zero"}
)
