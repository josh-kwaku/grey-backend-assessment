package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

type userGetter interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

type UserHandler struct {
	users userGetter
}

func NewUserHandler(users userGetter) *UserHandler {
	return &UserHandler{users: users}
}

func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	userID, appErr := ownerFromPath(r)
	if appErr != nil {
		RespondAppError(w, appErr, nil)
		return
	}

	user, err := h.users.GetByID(r.Context(), userID)
	if err != nil {
		logging.FromContext(r.Context()).Error("failed to get user", "error", err)
		RespondDomainError(w, err)
		return
	}

	RespondSuccess(w, http.StatusOK, userDTO{
		ID:         user.ID,
		Email:      user.Email,
		Name:       user.Name,
		UniqueName: user.UniqueName,
	})
}
