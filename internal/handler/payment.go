package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/josh-kwaku/grey-backend-assessment/internal/auth"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
	"github.com/josh-kwaku/grey-backend-assessment/internal/service/payment"
)

type paymentService interface {
	CreateInternalTransfer(ctx context.Context, req payment.InternalTransferRequest) (*domain.Payment, error)
	CreateExternalPayout(ctx context.Context, req payment.ExternalPayoutRequest) (*domain.Payment, error)
	GetPaymentForUser(ctx context.Context, paymentID, userID uuid.UUID) (*domain.Payment, error)
}

type PaymentHandler struct {
	payments paymentService
}

func NewPaymentHandler(payments paymentService) *PaymentHandler {
	return &PaymentHandler{payments: payments}
}

type createPaymentRequest struct {
	RecipientUniqueName string `json:"recipient_unique_name"`
	SourceCurrency      string `json:"source_currency"`
	DestCurrency        string `json:"dest_currency"`
	Amount              int64  `json:"amount"`
}

func (r createPaymentRequest) Validate() []FieldError {
	var errs []FieldError

	if r.RecipientUniqueName == "" {
		errs = append(errs, FieldError{Field: "recipient_unique_name", Message: "required"})
	}

	if r.SourceCurrency == "" {
		errs = append(errs, FieldError{Field: "source_currency", Message: "required"})
	} else if !domain.Currency(r.SourceCurrency).IsValid() {
		errs = append(errs, FieldError{Field: "source_currency", Message: "must be USD, EUR, or GBP"})
	}

	if r.DestCurrency == "" {
		errs = append(errs, FieldError{Field: "dest_currency", Message: "required"})
	} else if !domain.Currency(r.DestCurrency).IsValid() {
		errs = append(errs, FieldError{Field: "dest_currency", Message: "must be USD, EUR, or GBP"})
	}

	if r.Amount <= 0 {
		errs = append(errs, FieldError{Field: "amount", Message: "must be greater than 0"})
	}

	return errs
}

type createExternalPayoutRequest struct {
	SourceCurrency string `json:"source_currency"`
	DestCurrency   string `json:"dest_currency"`
	Amount         int64  `json:"amount"`
	DestIBAN       string `json:"dest_iban"`
	DestBankName   string `json:"dest_bank_name"`
}

func (r createExternalPayoutRequest) Validate() []FieldError {
	var errs []FieldError

	if r.SourceCurrency == "" {
		errs = append(errs, FieldError{Field: "source_currency", Message: "required"})
	} else if !domain.Currency(r.SourceCurrency).IsValid() {
		errs = append(errs, FieldError{Field: "source_currency", Message: "must be USD, EUR, or GBP"})
	}

	if r.DestCurrency == "" {
		errs = append(errs, FieldError{Field: "dest_currency", Message: "required"})
	} else if !domain.Currency(r.DestCurrency).IsValid() {
		errs = append(errs, FieldError{Field: "dest_currency", Message: "must be USD, EUR, or GBP"})
	}

	if r.Amount <= 0 {
		errs = append(errs, FieldError{Field: "amount", Message: "must be greater than 0"})
	}

	if r.DestIBAN == "" {
		errs = append(errs, FieldError{Field: "dest_iban", Message: "required"})
	}

	if r.DestBankName == "" {
		errs = append(errs, FieldError{Field: "dest_bank_name", Message: "required"})
	}

	return errs
}

type paymentDTO struct {
	ID              uuid.UUID        `json:"id"`
	Type            string           `json:"type"`
	Status          string           `json:"status"`
	SourceAccountID uuid.UUID        `json:"source_account_id"`
	DestAccountID   *uuid.UUID       `json:"dest_account_id"`
	SourceAmount    int64            `json:"source_amount"`
	SourceCurrency  string           `json:"source_currency"`
	DestAmount      int64            `json:"dest_amount"`
	DestCurrency    string           `json:"dest_currency"`
	ExchangeRate    *decimal.Decimal `json:"exchange_rate"`
	FeeAmount       int64            `json:"fee_amount"`
	FeeCurrency     *string          `json:"fee_currency,omitempty"`
	DestIBAN        *string          `json:"dest_iban,omitempty"`
	DestBankName    *string          `json:"dest_bank_name,omitempty"`
	CreatedAt       time.Time        `json:"created_at"`
	CompletedAt     *time.Time       `json:"completed_at,omitempty"`
}

func toPaymentDTO(p *domain.Payment) paymentDTO {
	dto := paymentDTO{
		ID:              p.ID,
		Type:            string(p.Type),
		Status:          string(p.Status),
		SourceAccountID: p.SourceAccountID,
		DestAccountID:   p.DestAccountID,
		SourceAmount:    p.SourceAmount,
		SourceCurrency:  string(p.SourceCurrency),
		DestAmount:      p.DestAmount,
		DestCurrency:    string(p.DestCurrency),
		ExchangeRate:    p.ExchangeRate,
		FeeAmount:       p.FeeAmount,
		CreatedAt:       p.CreatedAt,
		CompletedAt:     p.CompletedAt,
	}
	if p.FeeCurrency != nil {
		c := string(*p.FeeCurrency)
		dto.FeeCurrency = &c
	}
	dto.DestIBAN = p.DestIBAN
	dto.DestBankName = p.DestBankName
	return dto
}

func (h *PaymentHandler) Create(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context())

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		RespondAppError(w, ErrMissingToken, nil)
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	var req createPaymentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondAppError(w, ErrInvalidRequest, nil)
		return
	}

	if fields := req.Validate(); len(fields) > 0 {
		RespondValidationError(w, fields)
		return
	}

	p, err := h.payments.CreateInternalTransfer(r.Context(), payment.InternalTransferRequest{
		SenderUserID:        userID,
		RecipientUniqueName: req.RecipientUniqueName,
		SourceCurrency:      domain.Currency(req.SourceCurrency),
		DestCurrency:        domain.Currency(req.DestCurrency),
		Amount:              req.Amount,
		IdempotencyKey:      idempotencyKey,
	})
	if err != nil {
		log.Warn("payment creation failed", "error", err)
		RespondDomainError(w, err)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/payments/%s", p.ID))
	RespondSuccess(w, http.StatusCreated, toPaymentDTO(p))
}

func (h *PaymentHandler) CreateExternal(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context())

	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		RespondAppError(w, ErrMissingToken, nil)
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")

	var req createExternalPayoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondAppError(w, ErrInvalidRequest, nil)
		return
	}

	if fields := req.Validate(); len(fields) > 0 {
		RespondValidationError(w, fields)
		return
	}

	p, err := h.payments.CreateExternalPayout(r.Context(), payment.ExternalPayoutRequest{
		SenderUserID:   userID,
		SourceCurrency: domain.Currency(req.SourceCurrency),
		DestCurrency:   domain.Currency(req.DestCurrency),
		Amount:         req.Amount,
		DestIBAN:       req.DestIBAN,
		DestBankName:   req.DestBankName,
		IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		log.Warn("external payout creation failed", "error", err)
		RespondDomainError(w, err)
		return
	}

	w.Header().Set("Location", fmt.Sprintf("/api/v1/payments/%s", p.ID))
	RespondSuccess(w, http.StatusAccepted, toPaymentDTO(p))
}

func (h *PaymentHandler) Get(w http.ResponseWriter, r *http.Request) {
	userID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		RespondAppError(w, ErrMissingToken, nil)
		return
	}

	paymentID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		RespondAppError(w, ErrResourceNotFound, nil)
		return
	}

	p, err := h.payments.GetPaymentForUser(r.Context(), paymentID, userID)
	if err != nil {
		logging.FromContext(r.Context()).Warn("payment lookup failed", "error", err)
		RespondDomainError(w, err)
		return
	}

	RespondSuccess(w, http.StatusOK, toPaymentDTO(p))
}
