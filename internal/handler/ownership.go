package handler

import (
	"net/http"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/auth"
)

func ownerFromPath(r *http.Request) (uuid.UUID, *AppError) {
	authUserID, ok := auth.UserIDFromContext(r.Context())
	if !ok {
		return uuid.Nil, ErrMissingToken
	}

	userID, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		return uuid.Nil, ErrResourceNotFound
	}

	if userID != authUserID {
		return uuid.Nil, ErrResourceNotFound
	}

	return userID, nil
}
