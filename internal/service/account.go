package service

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"github.com/josh-kwaku/grey-backend-assessment/internal/logging"
)

type accountRepo interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Account, error)
	GetByUserAndCurrency(ctx context.Context, userID uuid.UUID, currency domain.Currency, accountType domain.AccountType) (*domain.Account, error)
	GetByUserIDAndType(ctx context.Context, userID uuid.UUID, accountType domain.AccountType) ([]domain.Account, error)
	Create(ctx context.Context, account *domain.Account) error
}

type userChecker interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
}

type AccountService struct {
	accounts accountRepo
	users    userChecker
}

func NewAccountService(accounts accountRepo, users userChecker) *AccountService {
	return &AccountService{accounts: accounts, users: users}
}

func (s *AccountService) CreateAccount(ctx context.Context, userID uuid.UUID, currency domain.Currency) (*domain.Account, error) {
	log := logging.FromContext(ctx)

	if _, err := s.users.GetByID(ctx, userID); err != nil {
		return nil, fmt.Errorf("CreateAccount: %w", err)
	}

	if !currency.IsValid() {
		return nil, fmt.Errorf("CreateAccount: %w", domain.ErrInvalidCurrency)
	}

	_, err := s.accounts.GetByUserAndCurrency(ctx, userID, currency, domain.AccountTypeUser)
	if err == nil {
		return nil, fmt.Errorf("CreateAccount: %w", domain.ErrAccountExists)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return nil, fmt.Errorf("CreateAccount: check existing: %w", err)
	}

	acctNum, err := generateAccountNumber()
	if err != nil {
		return nil, fmt.Errorf("CreateAccount: %w", err)
	}
	iban := generateIBAN(currency, acctNum)

	account := &domain.Account{
		ID:            uuid.New(),
		UserID:        userID,
		Currency:      currency,
		AccountType:   domain.AccountTypeUser,
		Balance:       0,
		Version:       1,
		AccountNumber: &acctNum,
		IBAN:          &iban,
		Status:        domain.AccountStatusActive,
		CreatedAt:     time.Now().UTC(),
	}

	if err := s.accounts.Create(ctx, account); err != nil {
		return nil, fmt.Errorf("CreateAccount: %w", err)
	}

	log.Info("account created",
		"account_id", account.ID,
		"user_id", userID,
		"currency", currency,
	)

	return account, nil
}

func (s *AccountService) GetUserAccounts(ctx context.Context, userID uuid.UUID) ([]domain.Account, error) {
	accounts, err := s.accounts.GetByUserIDAndType(ctx, userID, domain.AccountTypeUser)
	if err != nil {
		return nil, fmt.Errorf("GetUserAccounts: %w", err)
	}
	return accounts, nil
}

func (s *AccountService) GetAccountByID(ctx context.Context, accountID uuid.UUID) (*domain.Account, error) {
	account, err := s.accounts.GetByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("GetAccountByID: %w", err)
	}
	return account, nil
}

func generateAccountNumber() (string, error) {
	digits := make([]byte, 10)
	for i := range digits {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("generateAccountNumber: %w", err)
		}
		digits[i] = '0' + byte(n.Int64())
	}
	return string(digits), nil
}

func generateIBAN(currency domain.Currency, acctNum string) string {
	prefix := "XX"
	switch currency {
	case domain.CurrencyGBP:
		prefix = "GB"
	case domain.CurrencyEUR:
		prefix = "DE"
	case domain.CurrencyUSD:
		prefix = "US"
	}
	return fmt.Sprintf("%s82GREY0000%s", prefix, acctNum)
}
