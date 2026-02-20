package fx

import (
	"context"
	"testing"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetRate(t *testing.T) {
	svc := NewRateService(0.005)
	ctx := context.Background()

	tests := []struct {
		name          string
		from          domain.Currency
		to            domain.Currency
		wantMid       string
		wantEffective string
		wantSpread    string
		wantErr       error
	}{
		{
			name:          "USD to EUR",
			from:          domain.CurrencyUSD,
			to:            domain.CurrencyEUR,
			wantMid:       "0.92",
			wantEffective: "0.9154",
			wantSpread:    "0.005",
		},
		{
			name:          "EUR to USD",
			from:          domain.CurrencyEUR,
			to:            domain.CurrencyUSD,
			wantMid:       "1.087",
			wantEffective: "1.081565",
			wantSpread:    "0.005",
		},
		{
			name:          "same currency",
			from:          domain.CurrencyUSD,
			to:            domain.CurrencyUSD,
			wantMid:       "1",
			wantEffective: "1",
			wantSpread:    "0",
		},
		{
			name:    "invalid currency",
			from:    domain.CurrencyUSD,
			to:      domain.Currency("XYZ"),
			wantErr: domain.ErrInvalidCurrency,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			quote, err := svc.GetRate(ctx, tc.from, tc.to)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.from, quote.FromCurrency)
			assert.Equal(t, tc.to, quote.ToCurrency)
			assert.True(t, quote.MidMarketRate.Equal(decimal.RequireFromString(tc.wantMid)),
				"mid: got %s, want %s", quote.MidMarketRate, tc.wantMid)
			assert.True(t, quote.EffectiveRate.Equal(decimal.RequireFromString(tc.wantEffective)),
				"effective: got %s, want %s", quote.EffectiveRate, tc.wantEffective)
			assert.True(t, quote.SpreadPct.Equal(decimal.RequireFromString(tc.wantSpread)),
				"spread: got %s, want %s", quote.SpreadPct, tc.wantSpread)
		})
	}
}

func TestConvert(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		spreadPct  float64
		amount     int64
		from       domain.Currency
		to         domain.Currency
		wantDest   int64
		wantFee    int64
		wantErr    error
	}{
		{
			name:      "10000 USD to EUR with 0.5% spread",
			spreadPct: 0.005,
			amount:    10000,
			from:      domain.CurrencyUSD,
			to:        domain.CurrencyEUR,
			wantDest:  9154,
			wantFee:   46,
		},
		{
			name:      "same currency passthrough",
			spreadPct: 0.005,
			amount:    5000,
			from:      domain.CurrencyUSD,
			to:        domain.CurrencyUSD,
			wantDest:  5000,
			wantFee:   0,
		},
		{
			name:      "1 cent minimum enforced",
			spreadPct: 0.005,
			amount:    1,
			from:      domain.CurrencyUSD,
			to:        domain.CurrencyEUR,
			wantDest:  1,
			wantFee:   0,
		},
		{
			name:      "zero spread means effective equals mid",
			spreadPct: 0,
			amount:    10000,
			from:      domain.CurrencyUSD,
			to:        domain.CurrencyEUR,
			wantDest:  9200,
			wantFee:   0,
		},
		{
			name:      "zero amount",
			spreadPct: 0.005,
			amount:    0,
			from:      domain.CurrencyUSD,
			to:        domain.CurrencyEUR,
			wantErr:   domain.ErrInvalidAmount,
		},
		{
			name:      "negative amount",
			spreadPct: 0.005,
			amount:    -100,
			from:      domain.CurrencyUSD,
			to:        domain.CurrencyEUR,
			wantErr:   domain.ErrInvalidAmount,
		},
		{
			name:      "invalid currency pair",
			spreadPct: 0.005,
			amount:    1000,
			from:      domain.CurrencyUSD,
			to:        domain.Currency("XYZ"),
			wantErr:   domain.ErrInvalidCurrency,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := NewRateService(tc.spreadPct)
			conv, err := svc.Convert(ctx, tc.amount, tc.from, tc.to)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tc.amount, conv.SourceAmount)
			assert.Equal(t, tc.wantDest, conv.DestAmount)
			assert.Equal(t, tc.wantFee, conv.FeeAmount)
			assert.True(t, conv.DestAmount >= 1, "dest amount must be >= 1")
		})
	}
}
