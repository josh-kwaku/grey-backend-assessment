package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
)

const testWebhookSecret = "test-secret-key"

type mockWebhookRepo struct {
	created *domain.WebhookEvent
	err     error
}

func (m *mockWebhookRepo) Create(_ context.Context, event *domain.WebhookEvent) error {
	m.created = event
	return m.err
}

func signPayload(body, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func validWebhookBody() string {
	p := webhookPayload{
		EventID:   uuid.NewString(),
		PaymentID: uuid.NewString(),
		Status:    "completed",
		Timestamp: "2026-02-20T00:00:00Z",
	}
	b, _ := json.Marshal(p)
	return string(b)
}

func TestVerifyHMAC(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		signature string
		secret    string
		want      bool
	}{
		{
			name:      "valid signature",
			body:      `{"event_id":"abc"}`,
			signature: signPayload(`{"event_id":"abc"}`, testWebhookSecret),
			secret:    testWebhookSecret,
			want:      true,
		},
		{
			name:      "wrong signature",
			body:      `{"event_id":"abc"}`,
			signature: "deadbeef",
			secret:    testWebhookSecret,
			want:      false,
		},
		{
			name:      "empty signature",
			body:      `{"event_id":"abc"}`,
			signature: "",
			secret:    testWebhookSecret,
			want:      false,
		},
		{
			name:      "wrong secret",
			body:      `{"event_id":"abc"}`,
			signature: signPayload(`{"event_id":"abc"}`, "other-secret"),
			secret:    testWebhookSecret,
			want:      false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := verifyHMAC([]byte(tc.body), tc.signature, tc.secret)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReceiveProviderWebhook(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		setupSig   func(body string) string
		repoErr    error
		wantStatus int
		wantCode   string
	}{
		{
			name:       "valid signed webhook",
			body:       validWebhookBody(),
			setupSig:   func(body string) string { return signPayload(body, testWebhookSecret) },
			wantStatus: http.StatusOK,
		},
		{
			name:       "missing signature header",
			body:       validWebhookBody(),
			setupSig:   nil,
			wantStatus: http.StatusUnauthorized,
			wantCode:   "INVALID_SIGNATURE",
		},
		{
			name:       "invalid HMAC signature",
			body:       validWebhookBody(),
			setupSig:   func(_ string) string { return "deadbeefdeadbeef" },
			wantStatus: http.StatusUnauthorized,
			wantCode:   "INVALID_SIGNATURE",
		},
		{
			name:       "empty body",
			body:       "",
			setupSig:   func(body string) string { return signPayload(body, testWebhookSecret) },
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name:       "invalid JSON body",
			body:       "not-json",
			setupSig:   func(body string) string { return signPayload(body, testWebhookSecret) },
			wantStatus: http.StatusBadRequest,
			wantCode:   "INVALID_REQUEST",
		},
		{
			name: "missing required fields",
			body: func() string {
				b, _ := json.Marshal(map[string]string{"status": "completed"})
				return string(b)
			}(),
			setupSig:   func(body string) string { return signPayload(body, testWebhookSecret) },
			wantStatus: http.StatusBadRequest,
			wantCode:   "VALIDATION_FAILED",
		},
		{
			name:       "duplicate webhook returns OK",
			body:       validWebhookBody(),
			setupSig:   func(body string) string { return signPayload(body, testWebhookSecret) },
			repoErr:    &pq.Error{Code: "23505"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "repository error returns 500",
			body:       validWebhookBody(),
			setupSig:   func(body string) string { return signPayload(body, testWebhookSecret) },
			repoErr:    fmt.Errorf("connection refused"),
			wantStatus: http.StatusInternalServerError,
			wantCode:   "INTERNAL_ERROR",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			repo := &mockWebhookRepo{err: tc.repoErr}
			h := NewWebhookHandler(repo, testWebhookSecret)

			req := httptest.NewRequest(http.MethodPost, "/webhooks/provider", strings.NewReader(tc.body))
			if tc.setupSig != nil {
				req.Header.Set("X-Webhook-Signature", tc.setupSig(tc.body))
			}
			rr := httptest.NewRecorder()

			h.ReceiveProviderWebhook(rr, req)

			assert.Equal(t, tc.wantStatus, rr.Code)

			var resp APIResponse
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))

			if tc.wantCode == "" {
				assert.True(t, resp.Success)
			} else {
				assert.False(t, resp.Success)
				require.NotNil(t, resp.Error)
				assert.Equal(t, tc.wantCode, resp.Error.Code)
			}
		})
	}
}

func TestReceiveProviderWebhook_StoresCorrectEvent(t *testing.T) {
	repo := &mockWebhookRepo{}
	h := NewWebhookHandler(repo, testWebhookSecret)

	body := validWebhookBody()
	req := httptest.NewRequest(http.MethodPost, "/webhooks/provider", strings.NewReader(body))
	req.Header.Set("X-Webhook-Signature", signPayload(body, testWebhookSecret))
	rr := httptest.NewRecorder()

	h.ReceiveProviderWebhook(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.NotNil(t, repo.created)
	assert.Equal(t, domain.WebhookEventStatusPending, repo.created.Status)
	assert.Equal(t, domain.WebhookEventTypePaymentCompleted, repo.created.EventType)
	assert.NotEqual(t, uuid.Nil, repo.created.ID)
	assert.Equal(t, json.RawMessage(body), repo.created.Payload)
}
