package middleware

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/auth"
	"github.com/josh-kwaku/grey-backend-assessment/internal/handler"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
	"github.com/josh-kwaku/grey-backend-assessment/internal/repository"
)

type idempotencyRepository interface {
	Get(ctx context.Context, key string, userID uuid.UUID) (*repository.IdempotencyCacheEntry, error)
	Set(ctx context.Context, entry *repository.IdempotencyCacheEntry) error
}

const idempotencyTTL = 24 * time.Hour

func Idempotency(repo idempotencyRepository) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
				next.ServeHTTP(w, r)
				return
			}

			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				handler.RespondAppError(w, handler.ErrMissingIdempotencyKey, nil)
				return
			}

			userID, ok := auth.UserIDFromContext(r.Context())
			if !ok {
				handler.RespondAppError(w, handler.ErrMissingToken, nil)
				return
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				handler.RespondAppError(w, handler.ErrInvalidRequest, nil)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))

			reqHash := computeHash(r.Method, r.URL.Path, body)

			cached, err := repo.Get(r.Context(), key, userID)
			if err != nil {
				log := logging.FromContext(r.Context())
				log.Error("idempotency cache lookup failed", "error", err, "idempotency_key", key)
				handler.RespondAppError(w, handler.ErrInternalError, nil)
				return
			}

			if cached != nil {
				if cached.RequestHash != reqHash {
					handler.RespondAppError(w, handler.ErrIdempotencyConflict, nil)
					return
				}

				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("X-Idempotent-Replayed", "true")
				w.WriteHeader(cached.StatusCode)
				if _, err := w.Write(cached.ResponseBody); err != nil {
					log := logging.FromContext(r.Context())
					log.Error("failed to write idempotent replay", "error", err, "idempotency_key", key)
				}
				return
			}

			rec := &responseRecorder{ResponseWriter: w, body: &bytes.Buffer{}, statusCode: http.StatusOK}
			next.ServeHTTP(rec, r)

			entry := &repository.IdempotencyCacheEntry{
				Key:          key,
				UserID:       userID,
				RequestHash:  reqHash,
				StatusCode:   rec.statusCode,
				ResponseBody: rec.body.Bytes(),
				CreatedAt:    time.Now().UTC(),
				ExpiresAt:    time.Now().UTC().Add(idempotencyTTL),
			}
			if err := repo.Set(r.Context(), entry); err != nil {
				log := logging.FromContext(r.Context())
				log.Error("idempotency cache store failed", "error", err, "idempotency_key", key)
			}
		})
	}
}

func computeHash(method, path string, body []byte) string {
	h := sha256.New()
	h.Write([]byte(method))
	h.Write([]byte(path))
	h.Write(body)
	return fmt.Sprintf("%x", h.Sum(nil))
}

type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	body       *bytes.Buffer
}

func (r *responseRecorder) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
