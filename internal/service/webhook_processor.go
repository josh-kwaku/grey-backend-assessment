package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

type webhookRepo interface {
	GetPending(ctx context.Context, limit int) ([]domain.WebhookEvent, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.WebhookEventStatus) error
}

type wpPaymentRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	UpdateStatus(ctx context.Context, tx *sql.Tx, id uuid.UUID, status domain.PaymentStatus, providerRef *string, failureReason *string, completedAt *time.Time) error
}

type wpAccountRepo interface {
	GetByUserAndCurrency(ctx context.Context, userID uuid.UUID, currency domain.Currency, accountType domain.AccountType) (*domain.Account, error)
	GetForUpdate(ctx context.Context, tx *sql.Tx, id uuid.UUID) (*domain.Account, error)
	UpdateBalance(ctx context.Context, tx *sql.Tx, id uuid.UUID, newBalance int64, newVersion int64) error
}

type wpLedgerRepo interface {
	Create(ctx context.Context, tx *sql.Tx, entry *domain.LedgerEntry) error
}

type wpEventRepo interface {
	Create(ctx context.Context, tx *sql.Tx, event *domain.PaymentEvent) error
}

type WebhookProcessor struct {
	webhooks webhookRepo
	payments wpPaymentRepo
	accounts wpAccountRepo
	ledger   wpLedgerRepo
	events   wpEventRepo
	db       *sql.DB
	logger   *slog.Logger
	interval time.Duration
}

func NewWebhookProcessor(
	webhooks webhookRepo,
	payments wpPaymentRepo,
	accounts wpAccountRepo,
	ledger wpLedgerRepo,
	events wpEventRepo,
	db *sql.DB,
	logger *slog.Logger,
	interval time.Duration,
) *WebhookProcessor {
	return &WebhookProcessor{
		webhooks: webhooks,
		payments: payments,
		accounts: accounts,
		ledger:   ledger,
		events:   events,
		db:       db,
		logger:   logger,
		interval: interval,
	}
}

func (p *WebhookProcessor) Start(ctx context.Context) {
	p.logger.Info("webhook processor started", "interval", p.interval)

	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			p.logger.Info("webhook processor stopped")
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *WebhookProcessor) poll(ctx context.Context) {
	events, err := p.webhooks.GetPending(ctx, 10)
	if err != nil {
		p.logger.Error("failed to fetch pending webhook events", "error", err)
		return
	}

	for _, event := range events {
		if err := p.processEvent(ctx, event); err != nil {
			p.logger.Error("failed to process webhook event",
				"webhook_event_id", event.ID,
				"error", err,
			)
		}
	}
}

type webhookCallbackPayload struct {
	EventID     string `json:"event_id"`
	PaymentID   string `json:"payment_id"`
	Status      string `json:"status"`
	ProviderRef string `json:"provider_ref,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func (p *WebhookProcessor) processEvent(ctx context.Context, event domain.WebhookEvent) error {
	var payload webhookCallbackPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		p.logger.Error("malformed webhook payload", "webhook_event_id", event.ID, "error", err)
		return p.webhooks.UpdateStatus(ctx, event.ID, domain.WebhookEventStatusFailed)
	}

	paymentID, err := uuid.Parse(payload.PaymentID)
	if err != nil {
		p.logger.Error("invalid payment_id in webhook", "webhook_event_id", event.ID, "payment_id", payload.PaymentID)
		return p.webhooks.UpdateStatus(ctx, event.ID, domain.WebhookEventStatusFailed)
	}

	payment, err := p.payments.GetByID(ctx, paymentID)
	if err != nil {
		p.logger.Warn("payment not found for webhook", "webhook_event_id", event.ID, "payment_id", paymentID)
		return p.webhooks.UpdateStatus(ctx, event.ID, domain.WebhookEventStatusFailed)
	}

	if isTerminalStatus(payment.Status) {
		p.logger.Info("payment already in terminal state, skipping",
			"webhook_event_id", event.ID,
			"payment_id", paymentID,
			"payment_status", payment.Status,
		)
		return p.webhooks.UpdateStatus(ctx, event.ID, domain.WebhookEventStatusDispatched)
	}

	switch payload.Status {
	case "completed":
		err = p.handleCompleted(ctx, payment, payload.ProviderRef)
	case "failed":
		err = p.handleFailed(ctx, payment, payload.Reason)
	default:
		p.logger.Error("unknown webhook status", "webhook_event_id", event.ID, "status", payload.Status)
		return p.webhooks.UpdateStatus(ctx, event.ID, domain.WebhookEventStatusFailed)
	}

	if err != nil {
		if errors.Is(err, domain.ErrPaymentTerminal) {
			p.logger.Info("payment transitioned to terminal during processing",
				"webhook_event_id", event.ID,
				"payment_id", paymentID,
			)
			return p.webhooks.UpdateStatus(ctx, event.ID, domain.WebhookEventStatusDispatched)
		}
		return fmt.Errorf("processEvent: %w", err)
	}

	return p.webhooks.UpdateStatus(ctx, event.ID, domain.WebhookEventStatusDispatched)
}

func (p *WebhookProcessor) handleCompleted(ctx context.Context, payment *domain.Payment, providerRef string) error {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("handleCompleted: begin tx: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	var ref *string
	if providerRef != "" {
		ref = &providerRef
	}

	if err := p.payments.UpdateStatus(ctx, tx, payment.ID, domain.PaymentStatusCompleted, ref, nil, &now); err != nil {
		return fmt.Errorf("handleCompleted: update payment: %w", err)
	}

	event := &domain.PaymentEvent{
		ID:        uuid.New(),
		PaymentID: payment.ID,
		EventType: domain.PaymentEventTypeCompleted,
		Actor:     "system",
		CreatedAt: now,
	}
	if err := p.events.Create(ctx, tx, event); err != nil {
		return fmt.Errorf("handleCompleted: create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("handleCompleted: commit: %w", err)
	}

	p.logger.Info("payment completed", "payment_id", payment.ID, "provider_ref", providerRef)
	return nil
}

func (p *WebhookProcessor) handleFailed(ctx context.Context, payment *domain.Payment, reason string) error {
	isCrossCurrency := payment.SourceCurrency != payment.DestCurrency

	accountIDs := []uuid.UUID{payment.SourceAccountID}

	var outgoingID uuid.UUID
	outgoing, err := p.getSystemAccount(ctx, domain.AccountTypeOutgoing, payment.DestCurrency)
	if err != nil {
		return fmt.Errorf("handleFailed: %w", err)
	}
	outgoingID = outgoing.ID
	accountIDs = append(accountIDs, outgoingID)

	var fxPoolSourceID, fxPoolDestID uuid.UUID
	if isCrossCurrency {
		fxSrc, err := p.getSystemAccount(ctx, domain.AccountTypeFXPool, payment.SourceCurrency)
		if err != nil {
			return fmt.Errorf("handleFailed: %w", err)
		}
		fxDst, err := p.getSystemAccount(ctx, domain.AccountTypeFXPool, payment.DestCurrency)
		if err != nil {
			return fmt.Errorf("handleFailed: %w", err)
		}
		fxPoolSourceID = fxSrc.ID
		fxPoolDestID = fxDst.ID
		accountIDs = append(accountIDs, fxPoolSourceID, fxPoolDestID)
	}

	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("handleFailed: begin tx: %w", err)
	}
	defer tx.Rollback()

	locked, err := lockAccountsInOrder(ctx, tx, p.accounts, accountIDs...)
	if err != nil {
		return fmt.Errorf("handleFailed: %w", err)
	}

	now := time.Now().UTC()
	failureReason := &reason

	if err := p.payments.UpdateStatus(ctx, tx, payment.ID, domain.PaymentStatusFailed, nil, failureReason, nil); err != nil {
		return fmt.Errorf("handleFailed: update payment: %w", err)
	}

	if isCrossCurrency {
		if err := p.writeCrossCurrencyReversal(ctx, tx, payment, locked, outgoingID, fxPoolSourceID, fxPoolDestID, now); err != nil {
			return fmt.Errorf("handleFailed: %w", err)
		}
	} else {
		if err := p.writeSameCurrencyReversal(ctx, tx, payment, locked, outgoingID, now); err != nil {
			return fmt.Errorf("handleFailed: %w", err)
		}
	}

	reasonJSON, _ := json.Marshal(map[string]string{"reason": reason})
	event := &domain.PaymentEvent{
		ID:        uuid.New(),
		PaymentID: payment.ID,
		EventType: domain.PaymentEventTypeFailed,
		Actor:     "system",
		Payload:   reasonJSON,
		CreatedAt: now,
	}
	if err := p.events.Create(ctx, tx, event); err != nil {
		return fmt.Errorf("handleFailed: create event: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("handleFailed: commit: %w", err)
	}

	p.logger.Info("payment failed, reversal complete", "payment_id", payment.ID, "reason", reason)
	return nil
}

