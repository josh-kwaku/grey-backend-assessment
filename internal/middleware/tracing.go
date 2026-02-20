package middleware

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const traceIDHeader = "X-Request-ID"

type traceIDKey struct{}

func Tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := r.Header.Get(traceIDHeader)
		if traceID == "" {
			traceID = uuid.New().String()
		}

		w.Header().Set(traceIDHeader, traceID)
		ctx := context.WithValue(r.Context(), traceIDKey{}, traceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func TraceIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey{}).(string); ok {
		return id
	}
	return ""
}
