package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/auth"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

type userReader interface {
	GetByEmail(ctx context.Context, email string) (*domain.User, error)
}

type AuthHandler struct {
	users     userReader
	jwtSecret string
	jwtExpiry time.Duration
}

func NewAuthHandler(users userReader, jwtSecret string, jwtExpiry time.Duration) *AuthHandler {
	return &AuthHandler{
		users:     users,
		jwtSecret: jwtSecret,
		jwtExpiry: jwtExpiry,
	}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (r loginRequest) Validate() []FieldError {
	var errs []FieldError
	if r.Email == "" {
		errs = append(errs, FieldError{Field: "email", Message: "required"})
	}
	if r.Password == "" {
		errs = append(errs, FieldError{Field: "password", Message: "required"})
	}
	return errs
}

type loginResponse struct {
	Token string  `json:"token"`
	User  userDTO `json:"user"`
}

type userDTO struct {
	ID         uuid.UUID `json:"id"`
	Email      string    `json:"email"`
	Name       string    `json:"name"`
	UniqueName *string   `json:"unique_name"`
}

func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondAppError(w, ErrInvalidRequest, nil)
		return
	}

	if fields := req.Validate(); len(fields) > 0 {
		RespondValidationError(w, fields)
		return
	}

	user, err := h.users.GetByEmail(r.Context(), req.Email)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			RespondAppError(w, ErrInvalidCredentials, nil)
			return
		}
		RespondDomainError(w, err)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		RespondAppError(w, ErrInvalidCredentials, nil)
		return
	}

	token, err := auth.GenerateToken(user.ID, user.Email, h.jwtSecret, h.jwtExpiry)
	if err != nil {
		RespondAppError(w, ErrInternalError, nil)
		return
	}

	RespondSuccess(w, http.StatusOK, loginResponse{
		Token: token,
		User: userDTO{
			ID:         user.ID,
			Email:      user.Email,
			Name:       user.Name,
			UniqueName: user.UniqueName,
		},
	})
}
