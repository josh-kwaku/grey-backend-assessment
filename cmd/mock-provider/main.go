package main

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

type processRequest struct {
	PaymentID   string `json:"payment_id"`
	Amount      int64  `json:"amount"`
	Currency    string `json:"currency"`
	DestIBAN    string `json:"dest_iban"`
	DestBankName string `json:"dest_bank_name"`
	CallbackURL string `json:"callback_url"`
}

type callbackPayload struct {
	EventID     string `json:"event_id"`
	PaymentID   string `json:"payment_id"`
	Status      string `json:"status"`
	ProviderRef string `json:"provider_ref,omitempty"`
	Reason      string `json:"reason,omitempty"`
	Timestamp   string `json:"timestamp"`
}

func main() {
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}
	logging.Init("mock-provider", logLevel, os.Getenv("APP_ENV"))

	secret := os.Getenv("WEBHOOK_SECRET")
	if secret == "" {
		slog.Error("WEBHOOK_SECRET is required")
		os.Exit(1)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	var wg sync.WaitGroup

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
			slog.Error("failed to write health response", "error", err)
		}
	})

	mux.HandleFunc("POST /process", func(w http.ResponseWriter, r *http.Request) {
		var req processRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if req.PaymentID == "" || req.CallbackURL == "" {
			http.Error(w, "payment_id and callback_url are required", http.StatusBadRequest)
			return
		}

		slog.Info("received payment request",
			"payment_id", req.PaymentID,
			"amount", req.Amount,
			"currency", req.Currency,
		)

		wg.Add(1)
		go func() {
			defer wg.Done()
			processPayment(client, secret, req)
		}()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(map[string]string{"status": "accepted"}); err != nil {
			slog.Error("failed to write process response", "error", err)
		}
	})

	srv := &http.Server{Addr: ":8081", Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		slog.Info("mock provider started", "addr", ":8081")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down mock provider")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	wg.Wait()
	slog.Info("mock provider stopped")
}

func processPayment(client *http.Client, secret string, req processRequest) {
	// Simulate processing delay: 1-3 seconds
	delay := time.Duration(1+rand.Intn(3)) * time.Second
	time.Sleep(delay)

	payload := callbackPayload{
		EventID:   uuid.New().String(),
		PaymentID: req.PaymentID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	// 80% success, 20% failure
	if rand.Intn(100) < 80 {
		payload.Status = "completed"
		payload.ProviderRef = fmt.Sprintf("mock_ref_%d", rand.Int63())
	} else {
		payload.Status = "failed"
		payload.Reason = "Insufficient funds at destination bank"
	}

	body, err := json.Marshal(payload)
	if err != nil {
		slog.Error("failed to marshal callback payload", "error", err, "payment_id", req.PaymentID)
		return
	}

	sig := computeHMAC(body, secret)

	httpReq, err := http.NewRequest(http.MethodPost, req.CallbackURL, bytes.NewReader(body))
	if err != nil {
		slog.Error("failed to create callback request", "error", err, "payment_id", req.PaymentID)
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("X-Webhook-Signature", sig)

	resp, err := client.Do(httpReq)
	if err != nil {
		slog.Error("failed to send callback", "error", err, "payment_id", req.PaymentID)
		return
	}
	defer resp.Body.Close()

	slog.Info("callback sent",
		"payment_id", req.PaymentID,
		"status", payload.Status,
		"callback_status_code", resp.StatusCode,
	)
}

func computeHMAC(data []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(data)
	return hex.EncodeToString(mac.Sum(nil))
}
