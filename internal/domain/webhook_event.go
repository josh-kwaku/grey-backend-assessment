package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type WebhookEventStatus string

const (
	WebhookEventStatusPending    WebhookEventStatus = "pending"
	WebhookEventStatusDispatched WebhookEventStatus = "dispatched"
	WebhookEventStatusFailed     WebhookEventStatus = "failed"
)

type WebhookEventType string

const (
	WebhookEventTypePaymentCompleted WebhookEventType = "payment.completed"
	WebhookEventTypePaymentFailed    WebhookEventType = "payment.failed"
)

type WebhookEvent struct {
	ID             uuid.UUID
	IdempotencyKey string
	EventType      WebhookEventType
	Payload        json.RawMessage
	Status         WebhookEventStatus
	Attempts       int
	LastAttempt    *time.Time
	CreatedAt      time.Time
}
