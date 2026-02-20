package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
	"github.com/josh-kwaku/grey-backend-assessment/internal/service/payment"
)

type ProviderClient struct {
	baseURL     string
	callbackURL string
	httpClient  *http.Client
}

func NewProviderClient(baseURL, callbackURL string) *ProviderClient {
	return &ProviderClient{
		baseURL:     baseURL,
		callbackURL: callbackURL,
		httpClient: &http.Client{
			Timeout: 5 * time.Second,
		},
	}
}

type providerPayload struct {
	PaymentID    string `json:"payment_id"`
	Amount       int64  `json:"amount"`
	Currency     string `json:"currency"`
	DestIBAN     string `json:"dest_iban"`
	DestBankName string `json:"dest_bank_name"`
	CallbackURL  string `json:"callback_url"`
}

func (c *ProviderClient) SubmitPayment(ctx context.Context, req payment.ProviderRequest) error {
	log := logging.FromContext(ctx)

	payload := providerPayload{
		PaymentID:    req.PaymentID.String(),
		Amount:       req.Amount,
		Currency:     string(req.Currency),
		DestIBAN:     req.DestIBAN,
		DestBankName: req.DestBankName,
		CallbackURL:  c.callbackURL,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("SubmitPayment: marshal: %w", err)
	}

	url := c.baseURL + "/process"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("SubmitPayment: build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	start := time.Now()
	log.Info("provider request sent", "provider", "mock_provider", "payment_id", req.PaymentID)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("SubmitPayment: send: %w", err)
	}
	defer resp.Body.Close()

	log.Info("provider response received",
		"status", resp.StatusCode,
		"duration_ms", time.Since(start).Milliseconds(),
	)

	if resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("SubmitPayment: unexpected status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
