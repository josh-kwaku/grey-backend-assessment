package fx

import (
	"context"
	"fmt"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/shopspring/decimal"
)

type Quote struct {
	FromCurrency  domain.Currency
	ToCurrency    domain.Currency
	MidMarketRate decimal.Decimal
	EffectiveRate decimal.Decimal
	SpreadPct     decimal.Decimal
}

type Conversion struct {
	SourceAmount  int64
	DestAmount    int64
	FeeAmount     int64
	ExchangeRate  decimal.Decimal
	MidMarketRate decimal.Decimal
}

type RateService struct {
	rates     map[string]decimal.Decimal
	spreadPct decimal.Decimal
}

func NewRateService(spreadPct float64) *RateService {
	return &RateService{
		spreadPct: decimal.NewFromFloat(spreadPct),
		rates: map[string]decimal.Decimal{
			"USD_EUR": decimal.NewFromFloat(0.92),
			"EUR_USD": decimal.NewFromFloat(1.087),
			"USD_GBP": decimal.NewFromFloat(0.79),
			"GBP_USD": decimal.NewFromFloat(1.266),
			"EUR_GBP": decimal.NewFromFloat(0.858),
			"GBP_EUR": decimal.NewFromFloat(1.166),
		},
	}
}

func pairKey(from, to domain.Currency) string {
	return string(from) + "_" + string(to)
}

func (s *RateService) GetRate(_ context.Context, from, to domain.Currency) (*Quote, error) {
	if !from.IsValid() || !to.IsValid() {
		return nil, fmt.Errorf("GetRate: invalid currency pair %s/%s: %w", from, to, domain.ErrInvalidCurrency)
	}

	if from == to {
		return &Quote{
			FromCurrency:  from,
			ToCurrency:    to,
			MidMarketRate: decimal.NewFromInt(1),
			EffectiveRate: decimal.NewFromInt(1),
			SpreadPct:     decimal.Zero,
		}, nil
	}

	mid, ok := s.rates[pairKey(from, to)]
	if !ok {
		return nil, fmt.Errorf("GetRate: unsupported pair %s/%s: %w", from, to, domain.ErrInvalidCurrency)
	}

	effective := mid.Mul(decimal.NewFromInt(1).Sub(s.spreadPct))

	return &Quote{
		FromCurrency:  from,
		ToCurrency:    to,
		MidMarketRate: mid,
		EffectiveRate: effective,
		SpreadPct:     s.spreadPct,
	}, nil
}

func (s *RateService) Convert(ctx context.Context, amount int64, from, to domain.Currency) (*Conversion, error) {
	if amount <= 0 {
		return nil, fmt.Errorf("Convert: %w", domain.ErrInvalidAmount)
	}

	quote, err := s.GetRate(ctx, from, to)
	if err != nil {
		return nil, fmt.Errorf("Convert: %w", err)
	}

	if from == to {
		return &Conversion{
			SourceAmount:  amount,
			DestAmount:    amount,
			FeeAmount:     0,
			ExchangeRate:  quote.EffectiveRate,
			MidMarketRate: quote.MidMarketRate,
		}, nil
	}

	src := decimal.NewFromInt(amount)

	destRaw := src.Mul(quote.EffectiveRate).Round(0)
	destAmount := destRaw.IntPart()
	if destAmount < 1 {
		destAmount = 1
	}

	midRounded := src.Mul(quote.MidMarketRate).Round(0).IntPart()
	fee := midRounded - destAmount
	if fee < 0 {
		fee = 0
	}

	return &Conversion{
		SourceAmount:  amount,
		DestAmount:    destAmount,
		FeeAmount:     fee,
		ExchangeRate:  quote.EffectiveRate,
		MidMarketRate: quote.MidMarketRate,
	}, nil
}
