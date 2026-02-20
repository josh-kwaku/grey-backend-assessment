package payment

import (
	"testing"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/config"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/stretchr/testify/require"
)

func newServiceWithConfig() *Service {
	return &Service{
		config: &config.Config{
			TxLimitUSD: 10_000_000,
			TxLimitEUR: 9_000_000,
			TxLimitGBP: 8_000_000,
		},
	}
}

func activeAccount(userID uuid.UUID, currency domain.Currency) *domain.Account {
	return &domain.Account{
		ID:       uuid.New(),
		UserID:   userID,
		Currency: currency,
		Status:   domain.AccountStatusActive,
	}
}

func TestValidateTransfer(t *testing.T) {
	svc := newServiceWithConfig()
	userA := uuid.New()
	userB := uuid.New()

	tests := []struct {
		name      string
		req       InternalTransferRequest
		sender    *domain.Account
		recipient *domain.Account
		wantErr   error
	}{
		{
			name:      "valid same-currency transfer",
			req:       InternalTransferRequest{Amount: 5000, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender:    activeAccount(userA, domain.CurrencyUSD),
			recipient: activeAccount(userB, domain.CurrencyUSD),
		},
		{
			name:      "amount zero",
			req:       InternalTransferRequest{Amount: 0, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender:    activeAccount(userA, domain.CurrencyUSD),
			recipient: activeAccount(userB, domain.CurrencyUSD),
			wantErr:   domain.ErrInvalidAmount,
		},
		{
			name:      "amount negative",
			req:       InternalTransferRequest{Amount: -100, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender:    activeAccount(userA, domain.CurrencyUSD),
			recipient: activeAccount(userB, domain.CurrencyUSD),
			wantErr:   domain.ErrInvalidAmount,
		},
		{
			name:      "exceeds USD limit",
			req:       InternalTransferRequest{Amount: 10_000_001, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender:    activeAccount(userA, domain.CurrencyUSD),
			recipient: activeAccount(userB, domain.CurrencyUSD),
			wantErr:   domain.ErrLimitExceeded,
		},
		{
			name:      "at USD limit is allowed",
			req:       InternalTransferRequest{Amount: 10_000_000, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender:    activeAccount(userA, domain.CurrencyUSD),
			recipient: activeAccount(userB, domain.CurrencyUSD),
		},
		{
			name:      "self-transfer same user same currency",
			req:       InternalTransferRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender:    activeAccount(userA, domain.CurrencyUSD),
			recipient: activeAccount(userA, domain.CurrencyUSD),
			wantErr:   domain.ErrSelfTransfer,
		},
		{
			name:      "self-conversion same user diff currency is allowed",
			req:       InternalTransferRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyEUR},
			sender:    activeAccount(userA, domain.CurrencyUSD),
			recipient: activeAccount(userA, domain.CurrencyEUR),
		},
		{
			// txLimitForCurrency returns 0 for unknown currencies, so any positive amount exceeds the limit
			name:      "unknown currency exceeds limit",
			req:       InternalTransferRequest{Amount: 1, SourceCurrency: domain.Currency("XYZ"), DestCurrency: domain.Currency("XYZ")},
			sender:    activeAccount(userA, domain.Currency("XYZ")),
			recipient: activeAccount(userB, domain.Currency("XYZ")),
			wantErr:   domain.ErrLimitExceeded,
		},
		{
			name: "sender frozen",
			req:  InternalTransferRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender: func() *domain.Account {
				a := activeAccount(userA, domain.CurrencyUSD)
				a.Status = domain.AccountStatusFrozen
				return a
			}(),
			recipient: activeAccount(userB, domain.CurrencyUSD),
			wantErr:   domain.ErrAccountFrozen,
		},
		{
			name: "sender closed",
			req:  InternalTransferRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender: func() *domain.Account {
				a := activeAccount(userA, domain.CurrencyUSD)
				a.Status = domain.AccountStatusClosed
				return a
			}(),
			recipient: activeAccount(userB, domain.CurrencyUSD),
			wantErr:   domain.ErrAccountClosed,
		},
		{
			name:   "recipient frozen",
			req:    InternalTransferRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender: activeAccount(userA, domain.CurrencyUSD),
			recipient: func() *domain.Account {
				a := activeAccount(userB, domain.CurrencyUSD)
				a.Status = domain.AccountStatusFrozen
				return a
			}(),
			wantErr: domain.ErrAccountFrozen,
		},
		{
			name:   "recipient closed",
			req:    InternalTransferRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestCurrency: domain.CurrencyUSD},
			sender: activeAccount(userA, domain.CurrencyUSD),
			recipient: func() *domain.Account {
				a := activeAccount(userB, domain.CurrencyUSD)
				a.Status = domain.AccountStatusClosed
				return a
			}(),
			wantErr: domain.ErrAccountClosed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.validateTransfer(tc.req, tc.sender, tc.recipient)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestValidateExternalPayout(t *testing.T) {
	svc := newServiceWithConfig()
	userA := uuid.New()

	tests := []struct {
		name    string
		req     ExternalPayoutRequest
		sender  *domain.Account
		wantErr error
	}{
		{
			name:   "valid payout",
			req:    ExternalPayoutRequest{Amount: 5000, SourceCurrency: domain.CurrencyUSD, DestIBAN: "DE89370400440532013000", DestBankName: "Deutsche Bank"},
			sender: activeAccount(userA, domain.CurrencyUSD),
		},
		{
			name:    "amount zero",
			req:     ExternalPayoutRequest{Amount: 0, SourceCurrency: domain.CurrencyUSD, DestIBAN: "DE89370400440532013000", DestBankName: "Deutsche Bank"},
			sender:  activeAccount(userA, domain.CurrencyUSD),
			wantErr: domain.ErrInvalidAmount,
		},
		{
			name:    "missing IBAN",
			req:     ExternalPayoutRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestIBAN: "", DestBankName: "Deutsche Bank"},
			sender:  activeAccount(userA, domain.CurrencyUSD),
			wantErr: domain.ErrInvalidRequest,
		},
		{
			name:    "missing bank name",
			req:     ExternalPayoutRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestIBAN: "DE89370400440532013000", DestBankName: ""},
			sender:  activeAccount(userA, domain.CurrencyUSD),
			wantErr: domain.ErrInvalidRequest,
		},
		{
			name:    "exceeds USD limit",
			req:     ExternalPayoutRequest{Amount: 10_000_001, SourceCurrency: domain.CurrencyUSD, DestIBAN: "DE89370400440532013000", DestBankName: "Deutsche Bank"},
			sender:  activeAccount(userA, domain.CurrencyUSD),
			wantErr: domain.ErrLimitExceeded,
		},
		{
			name:   "at USD limit is allowed",
			req:    ExternalPayoutRequest{Amount: 10_000_000, SourceCurrency: domain.CurrencyUSD, DestIBAN: "DE89370400440532013000", DestBankName: "Deutsche Bank"},
			sender: activeAccount(userA, domain.CurrencyUSD),
		},
		{
			name:    "amount negative",
			req:     ExternalPayoutRequest{Amount: -100, SourceCurrency: domain.CurrencyUSD, DestIBAN: "DE89370400440532013000", DestBankName: "Deutsche Bank"},
			sender:  activeAccount(userA, domain.CurrencyUSD),
			wantErr: domain.ErrInvalidAmount,
		},
		{
			name: "sender frozen",
			req:  ExternalPayoutRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestIBAN: "DE89370400440532013000", DestBankName: "Deutsche Bank"},
			sender: func() *domain.Account {
				a := activeAccount(userA, domain.CurrencyUSD)
				a.Status = domain.AccountStatusFrozen
				return a
			}(),
			wantErr: domain.ErrAccountFrozen,
		},
		{
			name: "sender closed",
			req:  ExternalPayoutRequest{Amount: 1000, SourceCurrency: domain.CurrencyUSD, DestIBAN: "DE89370400440532013000", DestBankName: "Deutsche Bank"},
			sender: func() *domain.Account {
				a := activeAccount(userA, domain.CurrencyUSD)
				a.Status = domain.AccountStatusClosed
				return a
			}(),
			wantErr: domain.ErrAccountClosed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := svc.validateExternalPayout(tc.req, tc.sender)

			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
		})
	}
}

