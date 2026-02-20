package payment_test

import (
	"context"
	"database/sql"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josh-kwaku/grey-backend-assessment/internal/config"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/fx"
	"github.com/josh-kwaku/grey-backend-assessment/internal/repository"
	"github.com/josh-kwaku/grey-backend-assessment/internal/service/payment"
	"github.com/josh-kwaku/grey-backend-assessment/internal/testutil"
)

func setupPaymentService(t *testing.T, db *sql.DB) *payment.Service {
	t.Helper()
	return payment.NewService(
		repository.NewPaymentRepository(db),
		repository.NewAccountRepository(db),
		repository.NewLedgerRepository(db),
		repository.NewPaymentEventRepository(db),
		repository.NewUserRepository(db),
		fx.NewRateService(0.005),
		nil,
		db,
		&config.Config{
			TxLimitUSD: 10_000_000,
			TxLimitEUR: 9_000_000,
			TxLimitGBP: 8_000_000,
		},
	)
}

func TestSameCurrencyTransfer_HappyPath(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := setupPaymentService(t, db)
	ctx := context.Background()

	sender := testutil.SeedTestUser(t, db, "sender@test.com", "Sender", "sender_hp")
	recipient := testutil.SeedTestUser(t, db, "recipient@test.com", "Recipient", "recipient_hp")
	senderAcct := testutil.SeedTestAccount(t, db, sender.ID, "USD", 10000)
	recipientAcct := testutil.SeedTestAccount(t, db, recipient.ID, "USD", 5000)

	p, err := svc.CreateInternalTransfer(ctx, payment.InternalTransferRequest{
		SenderUserID:        sender.ID,
		RecipientUniqueName: "recipient_hp",
		SourceCurrency:      domain.CurrencyUSD,
		DestCurrency:        domain.CurrencyUSD,
		Amount:              3000,
		IdempotencyKey:      uuid.NewString(),
	})

	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusCompleted, p.Status)
	assert.Equal(t, domain.PaymentTypeInternalTransfer, p.Type)
	assert.Equal(t, int64(3000), p.SourceAmount)
	assert.Equal(t, int64(3000), p.DestAmount)
	assert.NotNil(t, p.CompletedAt)

	assert.Equal(t, int64(7000), testutil.GetAccountBalance(t, db, senderAcct.ID))
	assert.Equal(t, int64(8000), testutil.GetAccountBalance(t, db, recipientAcct.ID))

	assert.Equal(t, 2, testutil.CountLedgerEntries(t, db, p.ID))

	entries := getLedgerEntries(t, db, p.ID)
	debit := findEntry(entries, domain.EntryTypeDebit)
	credit := findEntry(entries, domain.EntryTypeCredit)

	require.NotNil(t, debit)
	assert.Equal(t, int64(10000), debit.BalanceBefore)
	assert.Equal(t, int64(7000), debit.BalanceAfter)
	assert.Equal(t, senderAcct.ID, debit.AccountID)

	require.NotNil(t, credit)
	assert.Equal(t, int64(5000), credit.BalanceBefore)
	assert.Equal(t, int64(8000), credit.BalanceAfter)
	assert.Equal(t, recipientAcct.ID, credit.AccountID)

	events := getPaymentEvents(t, db, p.ID)
	assert.Len(t, events, 1)
	assert.Equal(t, domain.PaymentEventTypeCompleted, events[0].EventType)
}

func TestSameCurrencyTransfer_InsufficientFunds(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := setupPaymentService(t, db)
	ctx := context.Background()

	sender := testutil.SeedTestUser(t, db, "sender@test.com", "Sender", "sender_if")
	recipient := testutil.SeedTestUser(t, db, "recipient@test.com", "Recipient", "recipient_if")
	senderAcct := testutil.SeedTestAccount(t, db, sender.ID, "USD", 1000)
	recipientAcct := testutil.SeedTestAccount(t, db, recipient.ID, "USD", 5000)

	_, err := svc.CreateInternalTransfer(ctx, payment.InternalTransferRequest{
		SenderUserID:        sender.ID,
		RecipientUniqueName: "recipient_if",
		SourceCurrency:      domain.CurrencyUSD,
		DestCurrency:        domain.CurrencyUSD,
		Amount:              5000,
		IdempotencyKey:      uuid.NewString(),
	})

	require.ErrorIs(t, err, domain.ErrInsufficientFunds)
	assert.Equal(t, int64(1000), testutil.GetAccountBalance(t, db, senderAcct.ID))
	assert.Equal(t, int64(5000), testutil.GetAccountBalance(t, db, recipientAcct.ID))

	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM ledger_entries WHERE account_id = $1`, senderAcct.ID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestSameCurrencyTransfer_ConcurrentOverdraft(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := setupPaymentService(t, db)
	ctx := context.Background()

	sender := testutil.SeedTestUser(t, db, "sender@test.com", "Sender", "sender_co")
	recipient := testutil.SeedTestUser(t, db, "recipient@test.com", "Recipient", "recipient_co")
	senderAcct := testutil.SeedTestAccount(t, db, sender.ID, "USD", 10000)
	testutil.SeedTestAccount(t, db, recipient.ID, "USD", 0)

	var wg sync.WaitGroup
	results := make(chan error, 2)

	for i := range 2 {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_, err := svc.CreateInternalTransfer(ctx, payment.InternalTransferRequest{
				SenderUserID:        sender.ID,
				RecipientUniqueName: "recipient_co",
				SourceCurrency:      domain.CurrencyUSD,
				DestCurrency:        domain.CurrencyUSD,
				Amount:              7000,
				IdempotencyKey:      uuid.NewString(),
			})
			results <- err
		}(i)
	}

	wg.Wait()
	close(results)

	var successes, failures int
	for err := range results {
		if err == nil {
			successes++
		} else {
			assert.ErrorIs(t, err, domain.ErrInsufficientFunds)
			failures++
		}
	}

	assert.Equal(t, 1, successes, "exactly one transfer should succeed")
	assert.Equal(t, 1, failures, "exactly one transfer should fail")

	balance := testutil.GetAccountBalance(t, db, senderAcct.ID)
	assert.Equal(t, int64(3000), balance, "balance must be 3000, not negative")
}

func TestCrossCurrencyTransfer_HappyPath(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := setupPaymentService(t, db)
	ctx := context.Background()

	sender := testutil.SeedTestUser(t, db, "sender@test.com", "Sender", "sender_fx")
	recipient := testutil.SeedTestUser(t, db, "recipient@test.com", "Recipient", "recipient_fx")
	senderAcct := testutil.SeedTestAccount(t, db, sender.ID, "USD", 10000)
	recipientAcct := testutil.SeedTestAccount(t, db, recipient.ID, "EUR", 5000)

	fxPoolUSDBefore := testutil.GetAccountBalance(t, db, testutil.FXPoolUSDID)
	fxPoolEURBefore := testutil.GetAccountBalance(t, db, testutil.FXPoolEURID)

	p, err := svc.CreateInternalTransfer(ctx, payment.InternalTransferRequest{
		SenderUserID:        sender.ID,
		RecipientUniqueName: "recipient_fx",
		SourceCurrency:      domain.CurrencyUSD,
		DestCurrency:        domain.CurrencyEUR,
		Amount:              10000,
		IdempotencyKey:      uuid.NewString(),
	})

	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusCompleted, p.Status)
	assert.NotNil(t, p.ExchangeRate)
	assert.True(t, p.FeeAmount > 0)
	assert.Equal(t, int64(10000), p.SourceAmount)
	assert.Equal(t, int64(9154), p.DestAmount)

	assert.Equal(t, int64(0), testutil.GetAccountBalance(t, db, senderAcct.ID))
	assert.Equal(t, int64(5000+9154), testutil.GetAccountBalance(t, db, recipientAcct.ID))
	assert.Equal(t, fxPoolUSDBefore+10000, testutil.GetAccountBalance(t, db, testutil.FXPoolUSDID))
	assert.Equal(t, fxPoolEURBefore-9154, testutil.GetAccountBalance(t, db, testutil.FXPoolEURID))

	entries := getLedgerEntries(t, db, p.ID)
	assert.Len(t, entries, 4)

	senderDebit := findEntryByAccount(entries, senderAcct.ID, domain.EntryTypeDebit)
	fxUSDCredit := findEntryByAccount(entries, testutil.FXPoolUSDID, domain.EntryTypeCredit)
	fxEURDebit := findEntryByAccount(entries, testutil.FXPoolEURID, domain.EntryTypeDebit)
	recipientCredit := findEntryByAccount(entries, recipientAcct.ID, domain.EntryTypeCredit)

	require.NotNil(t, senderDebit)
	assert.Equal(t, int64(10000), senderDebit.BalanceBefore)
	assert.Equal(t, int64(0), senderDebit.BalanceAfter)

	require.NotNil(t, fxUSDCredit)
	assert.Equal(t, fxPoolUSDBefore, fxUSDCredit.BalanceBefore)
	assert.Equal(t, fxPoolUSDBefore+10000, fxUSDCredit.BalanceAfter)

	require.NotNil(t, fxEURDebit)
	assert.Equal(t, fxPoolEURBefore, fxEURDebit.BalanceBefore)
	assert.Equal(t, fxPoolEURBefore-9154, fxEURDebit.BalanceAfter)

	require.NotNil(t, recipientCredit)
	assert.Equal(t, int64(5000), recipientCredit.BalanceBefore)
	assert.Equal(t, int64(5000+9154), recipientCredit.BalanceAfter)

	events := getPaymentEvents(t, db, p.ID)
	assert.Len(t, events, 1)
	assert.Equal(t, domain.PaymentEventTypeCompleted, events[0].EventType)
}

func TestCrossCurrencyTransfer_SelfConversion(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := setupPaymentService(t, db)
	ctx := context.Background()

	user := testutil.SeedTestUser(t, db, "user@test.com", "User", "user_sc")
	usdAcct := testutil.SeedTestAccount(t, db, user.ID, "USD", 10000)
	eurAcct := testutil.SeedTestAccount(t, db, user.ID, "EUR", 5000)

	fxPoolUSDBefore := testutil.GetAccountBalance(t, db, testutil.FXPoolUSDID)
	fxPoolEURBefore := testutil.GetAccountBalance(t, db, testutil.FXPoolEURID)

	p, err := svc.CreateInternalTransfer(ctx, payment.InternalTransferRequest{
		SenderUserID:        user.ID,
		RecipientUniqueName: "user_sc",
		SourceCurrency:      domain.CurrencyUSD,
		DestCurrency:        domain.CurrencyEUR,
		Amount:              5000,
		IdempotencyKey:      uuid.NewString(),
	})

	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusCompleted, p.Status)
	assert.Equal(t, domain.PaymentTypeInternalTransfer, p.Type)
	assert.Equal(t, int64(5000), p.SourceAmount)
	assert.Equal(t, int64(4577), p.DestAmount)
	assert.NotNil(t, p.ExchangeRate)
	assert.True(t, p.FeeAmount > 0)

	assert.Equal(t, int64(5000), testutil.GetAccountBalance(t, db, usdAcct.ID))
	assert.Equal(t, int64(5000+4577), testutil.GetAccountBalance(t, db, eurAcct.ID))
	assert.Equal(t, fxPoolUSDBefore+5000, testutil.GetAccountBalance(t, db, testutil.FXPoolUSDID))
	assert.Equal(t, fxPoolEURBefore-4577, testutil.GetAccountBalance(t, db, testutil.FXPoolEURID))

	assert.Equal(t, 4, testutil.CountLedgerEntries(t, db, p.ID))
}

func TestExternalPayout_HappyPath(t *testing.T) {
	db := testutil.SetupTestDB(t)
	svc := setupPaymentService(t, db)
	ctx := context.Background()

	sender := testutil.SeedTestUser(t, db, "sender@test.com", "Sender", "sender_ep")
	senderAcct := testutil.SeedTestAccount(t, db, sender.ID, "USD", 10000)

	outgoingBefore := testutil.GetAccountBalance(t, db, testutil.OutgoingUSDID)

	p, err := svc.CreateExternalPayout(ctx, payment.ExternalPayoutRequest{
		SenderUserID:   sender.ID,
		SourceCurrency: domain.CurrencyUSD,
		DestCurrency:   domain.CurrencyUSD,
		Amount:         5000,
		DestIBAN:       "DE89370400440532013000",
		DestBankName:   "Deutsche Bank",
		IdempotencyKey: uuid.NewString(),
	})

	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusPending, p.Status)
	assert.Equal(t, domain.PaymentTypeExternalPayout, p.Type)
	assert.Equal(t, int64(5000), p.SourceAmount)
	assert.Equal(t, int64(5000), p.DestAmount)
	assert.Nil(t, p.CompletedAt)

	assert.Equal(t, int64(5000), testutil.GetAccountBalance(t, db, senderAcct.ID))
	assert.Equal(t, outgoingBefore+5000, testutil.GetAccountBalance(t, db, testutil.OutgoingUSDID))

	assert.Equal(t, 2, testutil.CountLedgerEntries(t, db, p.ID))

	entries := getLedgerEntries(t, db, p.ID)
	debit := findEntryByAccount(entries, senderAcct.ID, domain.EntryTypeDebit)
	credit := findEntryByAccount(entries, testutil.OutgoingUSDID, domain.EntryTypeCredit)

	require.NotNil(t, debit)
	assert.Equal(t, int64(10000), debit.BalanceBefore)
	assert.Equal(t, int64(5000), debit.BalanceAfter)

	require.NotNil(t, credit)
	assert.Equal(t, outgoingBefore, credit.BalanceBefore)
	assert.Equal(t, outgoingBefore+5000, credit.BalanceAfter)

	events := getPaymentEvents(t, db, p.ID)
	assert.Len(t, events, 1)
	assert.Equal(t, domain.PaymentEventTypeCreated, events[0].EventType)
}

func getLedgerEntries(t *testing.T, db *sql.DB, paymentID uuid.UUID) []domain.LedgerEntry {
	t.Helper()
	repo := repository.NewLedgerRepository(db)
	entries, err := repo.GetByPaymentID(context.Background(), paymentID)
	require.NoError(t, err)
	return entries
}

func getPaymentEvents(t *testing.T, db *sql.DB, paymentID uuid.UUID) []domain.PaymentEvent {
	t.Helper()
	repo := repository.NewPaymentEventRepository(db)
	events, err := repo.GetByPaymentID(context.Background(), paymentID)
	require.NoError(t, err)
	return events
}

func findEntry(entries []domain.LedgerEntry, entryType domain.EntryType) *domain.LedgerEntry {
	for _, e := range entries {
		if e.EntryType == entryType {
			return &e
		}
	}
	return nil
}

func findEntryByAccount(entries []domain.LedgerEntry, accountID uuid.UUID, entryType domain.EntryType) *domain.LedgerEntry {
	for _, e := range entries {
		if e.AccountID == accountID && e.EntryType == entryType {
			return &e
		}
	}
	return nil
}
