package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

type webhookEventRepository interface {
	Create(ctx context.Context, event *domain.WebhookEvent) error
}

type WebhookHandler struct {
	webhooks webhookEventRepository
	secret   string
}

func NewWebhookHandler(webhooks webhookEventRepository, secret string) *WebhookHandler {
	return &WebhookHandler{webhooks: webhooks, secret: secret}
}

type webhookPayload struct {
	EventID     string `json:"event_id"`
	PaymentID   string `json:"payment_id"`
	Status      string `json:"status"`
	ProviderRef string `json:"provider_ref,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Timestamp   string `json:"timestamp"`
}

func (p webhookPayload) validate() []FieldError {
	var errs []FieldError

	if p.EventID == "" {
		errs = append(errs, FieldError{Field: "event_id", Message: "required"})
	} else if _, err := uuid.Parse(p.EventID); err != nil {
		errs = append(errs, FieldError{Field: "event_id", Message: "must be a valid UUID"})
	}

	if p.PaymentID == "" {
		errs = append(errs, FieldError{Field: "payment_id", Message: "required"})
	} else if _, err := uuid.Parse(p.PaymentID); err != nil {
		errs = append(errs, FieldError{Field: "payment_id", Message: "must be a valid UUID"})
	}

	if p.Status == "" {
		errs = append(errs, FieldError{Field: "status", Message: "required"})
	} else if p.Status != "completed" && p.Status != "failed" {
		errs = append(errs, FieldError{Field: "status", Message: "must be completed or failed"})
	}

	return errs
}

func (p webhookPayload) eventType() domain.WebhookEventType {
	if p.Status == "completed" {
		return domain.WebhookEventTypePaymentCompleted
	}
	return domain.WebhookEventTypePaymentFailed
}

var ErrInvalidSignature = &AppError{http.StatusUnauthorized, "INVALID_SIGNATURE", "Webhook signature is invalid"}

func (h *WebhookHandler) ReceiveProviderWebhook(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context())

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		log.Error("failed to read webhook body", "error", err)
		RespondAppError(w, ErrInvalidRequest, nil)
		return
	}

	sig := r.Header.Get("X-Webhook-Signature")
	if !verifyHMAC(body, sig, h.secret) {
		log.Warn("webhook signature verification failed")
		RespondAppError(w, ErrInvalidSignature, nil)
		return
	}

	var payload webhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Warn("failed to parse webhook payload", "error", err)
		RespondAppError(w, ErrInvalidRequest, nil)
		return
	}

	if fields := payload.validate(); len(fields) > 0 {
		RespondValidationError(w, fields)
		return
	}

	event := &domain.WebhookEvent{
		ID:             uuid.New(),
		IdempotencyKey: payload.EventID,
		EventType:      payload.eventType(),
		Payload:        body,
		Status:         domain.WebhookEventStatusPending,
		CreatedAt:      time.Now().UTC(),
	}

	if err := h.webhooks.Create(r.Context(), event); err != nil {
		if isDuplicateKey(err) {
			log.Info("duplicate webhook received", "event_id", payload.EventID, "payment_id", payload.PaymentID)
			RespondSuccess(w, http.StatusOK, map[string]string{"status": "already_received"})
			return
		}
		log.Error("failed to store webhook event", "error", err)
		RespondAppError(w, ErrInternalError, nil)
		return
	}

	log.Info("webhook event stored",
		"webhook_event_id", event.ID,
		"provider_event_id", payload.EventID,
		"payment_id", payload.PaymentID,
		"event_type", event.EventType,
	)

	RespondSuccess(w, http.StatusOK, map[string]string{"status": "received"})
}

func verifyHMAC(body []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func isDuplicateKey(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) && pqErr.Code == "23505" {
		return true
	}
	return false
}
