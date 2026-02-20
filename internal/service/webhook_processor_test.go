package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

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

func setupWebhookTest(t *testing.T, db *sql.DB) (*payment.Service, *WebhookProcessor, *repository.WebhookEventRepository) {
	t.Helper()

	paymentSvc := payment.NewService(
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

	webhookRepo := repository.NewWebhookEventRepository(db)
	processor := NewWebhookProcessor(
		webhookRepo,
		repository.NewPaymentRepository(db),
		repository.NewAccountRepository(db),
		repository.NewLedgerRepository(db),
		repository.NewPaymentEventRepository(db),
		db,
		slog.Default(),
		time.Second,
	)

	return paymentSvc, processor, webhookRepo
}

func insertWebhookEvent(t *testing.T, repo *repository.WebhookEventRepository, paymentID uuid.UUID, status, reason string) *domain.WebhookEvent {
	t.Helper()
	ctx := context.Background()

	eventType := domain.WebhookEventTypePaymentCompleted
	if status == "failed" {
		eventType = domain.WebhookEventTypePaymentFailed
	}

	payload, _ := json.Marshal(webhookCallbackPayload{
		EventID:     uuid.NewString(),
		PaymentID:   paymentID.String(),
		Status:      status,
		ProviderRef: "prov-ref-123",
		Reason:      reason,
	})
	event := &domain.WebhookEvent{
		ID:             uuid.New(),
		IdempotencyKey: uuid.NewString(),
		EventType:      eventType,
		Payload:        payload,
		Status:         domain.WebhookEventStatusPending,
		CreatedAt:      time.Now().UTC(),
	}
	require.NoError(t, repo.Create(ctx, event))
	return event
}

func getWebhookStatus(t *testing.T, db *sql.DB, id uuid.UUID) domain.WebhookEventStatus {
	t.Helper()
	var status domain.WebhookEventStatus
	err := db.QueryRow(`SELECT status FROM webhook_events WHERE id = $1`, id).Scan(&status)
	require.NoError(t, err)
	return status
}

func TestWebhookProcessor_CompletedPayout(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	paymentSvc, processor, webhookRepo := setupWebhookTest(t, db)

	sender := testutil.SeedTestUser(t, db, "sender@test.com", "Sender", "sender_wc")
	senderAcct := testutil.SeedTestAccount(t, db, sender.ID, "USD", 10000)
	outgoingBefore := testutil.GetAccountBalance(t, db, testutil.OutgoingUSDID)

	p, err := paymentSvc.CreateExternalPayout(ctx, payment.ExternalPayoutRequest{
		SenderUserID:   sender.ID,
		SourceCurrency: domain.CurrencyUSD,
		DestCurrency:   domain.CurrencyUSD,
		Amount:         5000,
		DestIBAN:       "DE89370400440532013000",
		DestBankName:   "Deutsche Bank",
		IdempotencyKey: uuid.NewString(),
	})
	require.NoError(t, err)

	webhookEvent := insertWebhookEvent(t, webhookRepo, p.ID, "completed", "")

	err = processor.processEvent(ctx, *webhookEvent)
	require.NoError(t, err)

	updated, err := repository.NewPaymentRepository(db).GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusCompleted, updated.Status)
	require.NotNil(t, updated.ProviderRef)
	assert.Equal(t, "prov-ref-123", *updated.ProviderRef)
	require.NotNil(t, updated.CompletedAt)

	assert.Equal(t, int64(5000), testutil.GetAccountBalance(t, db, senderAcct.ID))
	assert.Equal(t, outgoingBefore+5000, testutil.GetAccountBalance(t, db, testutil.OutgoingUSDID))

	assert.Equal(t, 2, testutil.CountLedgerEntries(t, db, p.ID))

	events, err := repository.NewPaymentEventRepository(db).GetByPaymentID(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, events, 2)
	assert.Equal(t, domain.PaymentEventTypeCreated, events[0].EventType)
	assert.Equal(t, domain.PaymentEventTypeCompleted, events[1].EventType)

	assert.Equal(t, domain.WebhookEventStatusDispatched, getWebhookStatus(t, db, webhookEvent.ID))
}

func TestWebhookProcessor_FailedPayout_Reversal(t *testing.T) {
	db := testutil.SetupTestDB(t)
	ctx := context.Background()
	paymentSvc, processor, webhookRepo := setupWebhookTest(t, db)

	sender := testutil.SeedTestUser(t, db, "sender@test.com", "Sender", "sender_rev")
	senderAcct := testutil.SeedTestAccount(t, db, sender.ID, "USD", 10000)
	outgoingBefore := testutil.GetAccountBalance(t, db, testutil.OutgoingUSDID)

	p, err := paymentSvc.CreateExternalPayout(ctx, payment.ExternalPayoutRequest{
		SenderUserID:   sender.ID,
		SourceCurrency: domain.CurrencyUSD,
		DestCurrency:   domain.CurrencyUSD,
		Amount:         5000,
		DestIBAN:       "DE89370400440532013000",
		DestBankName:   "Deutsche Bank",
		IdempotencyKey: uuid.NewString(),
	})
	require.NoError(t, err)
	require.Equal(t, domain.PaymentStatusPending, p.Status)

	assert.Equal(t, int64(5000), testutil.GetAccountBalance(t, db, senderAcct.ID))
	assert.Equal(t, outgoingBefore+5000, testutil.GetAccountBalance(t, db, testutil.OutgoingUSDID))

	webhookEvent := insertWebhookEvent(t, webhookRepo, p.ID, "failed", "provider_declined")

	err = processor.processEvent(ctx, *webhookEvent)
	require.NoError(t, err)

	assert.Equal(t, int64(10000), testutil.GetAccountBalance(t, db, senderAcct.ID))
	assert.Equal(t, outgoingBefore, testutil.GetAccountBalance(t, db, testutil.OutgoingUSDID))

	updated, err := repository.NewPaymentRepository(db).GetByID(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.PaymentStatusFailed, updated.Status)
	require.NotNil(t, updated.FailureReason)
	assert.Equal(t, "provider_declined", *updated.FailureReason)

	ledgerEntries, err := repository.NewLedgerRepository(db).GetByPaymentID(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, ledgerEntries, 4)

	// entries[2] = reversal debit on outgoing (funds returned from outgoing)
	reversalDebit := findLedgerEntry(ledgerEntries, testutil.OutgoingUSDID, domain.EntryTypeDebit)
	require.NotNil(t, reversalDebit)
	assert.Equal(t, int64(5000), reversalDebit.Amount)
	assert.Equal(t, outgoingBefore+5000, reversalDebit.BalanceBefore)
	assert.Equal(t, outgoingBefore, reversalDebit.BalanceAfter)

	// entries[3] = reversal credit on sender (funds restored)
	reversalCredit := findLedgerEntry(ledgerEntries, senderAcct.ID, domain.EntryTypeCredit)
	require.NotNil(t, reversalCredit)
	assert.Equal(t, int64(5000), reversalCredit.Amount)
	assert.Equal(t, int64(5000), reversalCredit.BalanceBefore)
	assert.Equal(t, int64(10000), reversalCredit.BalanceAfter)

	paymentEvents, err := repository.NewPaymentEventRepository(db).GetByPaymentID(ctx, p.ID)
	require.NoError(t, err)
	assert.Len(t, paymentEvents, 2)
	assert.Equal(t, domain.PaymentEventTypeCreated, paymentEvents[0].EventType)
	assert.Equal(t, domain.PaymentEventTypeFailed, paymentEvents[1].EventType)

	assert.Equal(t, domain.WebhookEventStatusDispatched, getWebhookStatus(t, db, webhookEvent.ID))
}

func findLedgerEntry(entries []domain.LedgerEntry, accountID uuid.UUID, entryType domain.EntryType) *domain.LedgerEntry {
	for _, e := range entries {
		if e.AccountID == accountID && e.EntryType == entryType {
			return &e
		}
	}
	return nil
}
