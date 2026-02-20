package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

type accountService interface {
	CreateAccount(ctx context.Context, userID uuid.UUID, currency domain.Currency) (*domain.Account, error)
	GetUserAccounts(ctx context.Context, userID uuid.UUID) ([]domain.Account, error)
}

type AccountHandler struct {
	accounts accountService
}

func NewAccountHandler(accounts accountService) *AccountHandler {
	return &AccountHandler{accounts: accounts}
}

type createAccountRequest struct {
	Currency string `json:"currency"`
}

func (r createAccountRequest) Validate() []FieldError {
	var errs []FieldError
	if r.Currency == "" {
		errs = append(errs, FieldError{Field: "currency", Message: "required"})
	} else if !domain.Currency(r.Currency).IsValid() {
		errs = append(errs, FieldError{Field: "currency", Message: "must be USD, EUR, or GBP"})
	}
	return errs
}

type accountDTO struct {
	ID            uuid.UUID `json:"id"`
	UserID        uuid.UUID `json:"user_id"`
	Currency      string    `json:"currency"`
	Balance       int64     `json:"balance"`
	AccountNumber *string   `json:"account_number"`
	IBAN          *string   `json:"iban"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
}

func toAccountDTO(a *domain.Account) accountDTO {
	return accountDTO{
		ID:            a.ID,
		UserID:        a.UserID,
		Currency:      string(a.Currency),
		Balance:       a.Balance,
		AccountNumber: a.AccountNumber,
		IBAN:          a.IBAN,
		Status:        string(a.Status),
		CreatedAt:     a.CreatedAt,
	}
}

func (h *AccountHandler) Create(w http.ResponseWriter, r *http.Request) {
	userID, appErr := ownerFromPath(r)
	if appErr != nil {
		RespondAppError(w, appErr, nil)
		return
	}

	var req createAccountRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondAppError(w, ErrInvalidRequest, nil)
		return
	}

	if fields := req.Validate(); len(fields) > 0 {
		RespondValidationError(w, fields)
		return
	}

	account, err := h.accounts.CreateAccount(r.Context(), userID, domain.Currency(req.Currency))
	if err != nil {
		logging.FromContext(r.Context()).Error("failed to create account", "error", err)
		RespondDomainError(w, err)
		return
	}

	RespondSuccess(w, http.StatusCreated, toAccountDTO(account))
}

func (h *AccountHandler) List(w http.ResponseWriter, r *http.Request) {
	userID, appErr := ownerFromPath(r)
	if appErr != nil {
		RespondAppError(w, appErr, nil)
		return
	}

	accounts, err := h.accounts.GetUserAccounts(r.Context(), userID)
	if err != nil {
		logging.FromContext(r.Context()).Error("failed to list accounts", "error", err)
		RespondDomainError(w, err)
		return
	}

	dtos := make([]accountDTO, len(accounts))
	for i := range accounts {
		dtos[i] = toAccountDTO(&accounts[i])
	}

	RespondSuccess(w, http.StatusOK, dtos)
}
