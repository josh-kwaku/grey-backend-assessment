package middleware

import (
	"net/http"
	"strings"

	"github.com/josh-kwaku/grey-backend-assessment/internal/auth"
	"github.com/josh-kwaku/grey-backend-assessment/internal/handler"
)

func Auth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			if header == "" {
				handler.RespondAppError(w, handler.ErrMissingToken, nil)
				return
			}

			token, found := strings.CutPrefix(header, "Bearer ")
			if !found || token == "" {
				handler.RespondAppError(w, handler.ErrInvalidToken, nil)
				return
			}

			claims, err := auth.ValidateToken(token, secret)
			if err != nil {
				handler.RespondAppError(w, handler.ErrInvalidToken, nil)
				return
			}

			ctx := auth.ContextWithUserID(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
