package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/josh-kwaku/grey-backend-assessment/internal/auth"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/health") {
			next.ServeHTTP(w, r)
			return
		}

		start := time.Now()

		attrs := []any{"request_id", TraceIDFromContext(r.Context())}
		if userID, ok := auth.UserIDFromContext(r.Context()); ok {
			attrs = append(attrs, "user_id", userID)
		}

		logger := slog.Default().With(attrs...)
		ctx := logging.WithLogger(r.Context(), logger)
		r = r.WithContext(ctx)

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		logger.Info("request completed",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}
