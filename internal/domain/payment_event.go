package domain

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type PaymentEventType string

const (
	PaymentEventTypeCreated    PaymentEventType = "created"
	PaymentEventTypeProcessing PaymentEventType = "processing"
	PaymentEventTypeCompleted  PaymentEventType = "completed"
	PaymentEventTypeFailed     PaymentEventType = "failed"
	PaymentEventTypeReversed   PaymentEventType = "reversed"
)

type PaymentEvent struct {
	ID        uuid.UUID
	PaymentID uuid.UUID
	EventType PaymentEventType
	Actor     string
	Payload   json.RawMessage
	CreatedAt time.Time
}
