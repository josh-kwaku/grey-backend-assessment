package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/josh-kwaku/grey-backend-assessment/internal/handler"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log := logging.FromContext(r.Context())
				log.Error("panic recovered", "error", err, "stack", string(debug.Stack()))
				handler.RespondAppError(w, handler.ErrInternalError, nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
