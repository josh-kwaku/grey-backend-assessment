package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

type APIResponse struct {
	Success bool     `json:"success"`
	Data    any      `json:"data"`
	Error   *APIError `json:"error"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type FieldError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

func RespondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		slog.Error("failed to encode response", "error", err)
	}
}

func RespondSuccess(w http.ResponseWriter, status int, data any) {
	RespondJSON(w, status, APIResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	})
}

func RespondAppError(w http.ResponseWriter, appErr *AppError, details any) {
	RespondJSON(w, appErr.Status, APIResponse{
		Success: false,
		Data:    nil,
		Error: &APIError{
			Code:    appErr.Code,
			Message: appErr.Message,
			Details: details,
		},
	})
}

func RespondValidationError(w http.ResponseWriter, fields []FieldError) {
	RespondAppError(w, ErrValidationFailed, fields)
}

func RespondDomainError(w http.ResponseWriter, err error) {
	var appErr *AppError

	switch {
	case errors.Is(err, domain.ErrNotFound):
		appErr = ErrResourceNotFound
	case errors.Is(err, domain.ErrInsufficientFunds):
		appErr = ErrInsufficientFunds
	case errors.Is(err, domain.ErrAccountFrozen):
		appErr = ErrAccountFrozen
	case errors.Is(err, domain.ErrDuplicatePayment):
		appErr = ErrDuplicatePayment
	case errors.Is(err, domain.ErrSelfTransfer):
		appErr = ErrSelfTransfer
	case errors.Is(err, domain.ErrLimitExceeded):
		appErr = ErrLimitExceeded
	case errors.Is(err, domain.ErrRecipientNotFound):
		appErr = ErrRecipientNotFound
	case errors.Is(err, domain.ErrAccountNotFound):
		appErr = ErrAccountNotFound
	case errors.Is(err, domain.ErrAccountExists):
		appErr = ErrAccountExists
	case errors.Is(err, domain.ErrInvalidCurrency):
		appErr = ErrInvalidCurrency
	case errors.Is(err, domain.ErrAccountClosed):
		appErr = ErrAccountClosed
	case errors.Is(err, domain.ErrCurrencyMismatch):
		appErr = ErrCurrencyMismatch
	case errors.Is(err, domain.ErrVersionConflict):
		appErr = ErrVersionConflict
	case errors.Is(err, domain.ErrInvalidAmount):
		appErr = ErrInvalidAmount
	case errors.Is(err, domain.ErrInvalidRequest):
		appErr = ErrInvalidRequest
	default:
		slog.Error("unhandled domain error", "error", err)
		appErr = ErrInternalError
	}

	RespondAppError(w, appErr, nil)
}
