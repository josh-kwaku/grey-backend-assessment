package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/fx"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

type fxService interface {
	GetRate(ctx context.Context, from, to domain.Currency) (*fx.Quote, error)
}

type FXHandler struct {
	fx fxService
}

func NewFXHandler(fxSvc fxService) *FXHandler {
	return &FXHandler{fx: fxSvc}
}

type fxRateResponse struct {
	FromCurrency  string `json:"from_currency"`
	ToCurrency    string `json:"to_currency"`
	MidMarketRate string `json:"mid_market_rate"`
	EffectiveRate string `json:"effective_rate"`
	SpreadPct     string `json:"spread_pct"`
	Timestamp     string `json:"timestamp"`
}

func (h *FXHandler) GetRate(w http.ResponseWriter, r *http.Request) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	if fields := validateFXRateParams(from, to); len(fields) > 0 {
		RespondValidationError(w, fields)
		return
	}

	quote, err := h.fx.GetRate(r.Context(), domain.Currency(from), domain.Currency(to))
	if err != nil {
		logging.FromContext(r.Context()).Warn("fx rate lookup failed", "error", err)
		RespondDomainError(w, err)
		return
	}

	RespondSuccess(w, http.StatusOK, fxRateResponse{
		FromCurrency:  string(quote.FromCurrency),
		ToCurrency:    string(quote.ToCurrency),
		MidMarketRate: quote.MidMarketRate.String(),
		EffectiveRate: quote.EffectiveRate.String(),
		SpreadPct:     quote.SpreadPct.String(),
		Timestamp:     time.Now().UTC().Format(time.RFC3339),
	})
}

func validateFXRateParams(from, to string) []FieldError {
	var errs []FieldError

	if from == "" {
		errs = append(errs, FieldError{Field: "from", Message: "required"})
	} else if !domain.Currency(from).IsValid() {
		errs = append(errs, FieldError{Field: "from", Message: "must be USD, EUR, or GBP"})
	}

	if to == "" {
		errs = append(errs, FieldError{Field: "to", Message: "required"})
	} else if !domain.Currency(to).IsValid() {
		errs = append(errs, FieldError{Field: "to", Message: "must be USD, EUR, or GBP"})
	}

	return errs
}
