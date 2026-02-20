package testutil

import (
	"database/sql"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/josh-kwaku/grey-backend-assessment/internal/domain"
	"golang.org/x/crypto/bcrypt"
)

var (
	SystemUserID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

	FXPoolUSDID   = uuid.MustParse("00000000-0000-0000-0001-000000000001")
	FXPoolEURID   = uuid.MustParse("00000000-0000-0000-0001-000000000002")
	FXPoolGBPID   = uuid.MustParse("00000000-0000-0000-0001-000000000003")
	OutgoingUSDID = uuid.MustParse("00000000-0000-0000-0002-000000000001")
	OutgoingEURID = uuid.MustParse("00000000-0000-0000-0002-000000000002")
	OutgoingGBPID = uuid.MustParse("00000000-0000-0000-0002-000000000003")
)

const fxPoolInitialBalance int64 = 1_000_000_000

func SeedSystemUser(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	_, err = db.Exec(
		`INSERT INTO users (id, email, name, password_hash, unique_name, status)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (id) DO NOTHING`,
		SystemUserID, "system@grey.internal", "System", string(hash), "system", "active",
	)
	if err != nil {
		t.Fatalf("seed system user: %v", err)
	}
	return SystemUserID
}

func SeedSystemAccounts(t *testing.T, db *sql.DB, systemUserID uuid.UUID) {
	t.Helper()

	systemAccounts := []struct {
		id          uuid.UUID
		accountType string
		currency    string
		balance     int64
	}{
		{FXPoolUSDID, "fx_pool", "USD", fxPoolInitialBalance},
		{FXPoolEURID, "fx_pool", "EUR", fxPoolInitialBalance},
		{FXPoolGBPID, "fx_pool", "GBP", fxPoolInitialBalance},
		{OutgoingUSDID, "outgoing", "USD", 0},
		{OutgoingEURID, "outgoing", "EUR", 0},
		{OutgoingGBPID, "outgoing", "GBP", 0},
	}

	for _, a := range systemAccounts {
		_, err := db.Exec(
			`INSERT INTO accounts (id, user_id, currency, account_type, balance, status)
			 VALUES ($1, $2, $3, $4, $5, 'active')
			 ON CONFLICT (id) DO NOTHING`,
			a.id, systemUserID, a.currency, a.accountType, a.balance,
		)
		if err != nil {
			t.Fatalf("seed %s %s: %v", a.accountType, a.currency, err)
		}
	}
}

func SeedTestUser(t *testing.T, db *sql.DB, email, name, uniqueName string) *domain.User {
	t.Helper()

	hash, err := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		Name:         name,
		PasswordHash: string(hash),
		UniqueName:   &uniqueName,
		Status:       domain.UserStatusActive,
		CreatedAt:    time.Now().UTC(),
	}

	_, err = db.Exec(
		`INSERT INTO users (id, email, name, password_hash, unique_name, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		u.ID, u.Email, u.Name, u.PasswordHash, u.UniqueName, u.Status, u.CreatedAt,
	)
	if err != nil {
		t.Fatalf("seed test user %s: %v", email, err)
	}
	return u
}

func SeedTestAccount(t *testing.T, db *sql.DB, userID uuid.UUID, currency string, balance int64) *domain.Account {
	t.Helper()

	a := &domain.Account{
		ID:          uuid.New(),
		UserID:      userID,
		Currency:    domain.Currency(currency),
		AccountType: domain.AccountTypeUser,
		Balance:     balance,
		Version:     0,
		Status:      domain.AccountStatusActive,
		CreatedAt:   time.Now().UTC(),
	}

	_, err := db.Exec(
		`INSERT INTO accounts (id, user_id, currency, account_type, balance, version, status, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		a.ID, a.UserID, a.Currency, a.AccountType, a.Balance, a.Version, a.Status, a.CreatedAt,
	)
	if err != nil {
		t.Fatalf("seed test account %s/%s: %v", userID, currency, err)
	}
	return a
}

func GetAccountBalance(t *testing.T, db *sql.DB, accountID uuid.UUID) int64 {
	t.Helper()

	var balance int64
	err := db.QueryRow(`SELECT balance FROM accounts WHERE id = $1`, accountID).Scan(&balance)
	if err != nil {
		t.Fatalf("get account balance %s: %v", accountID, err)
	}
	return balance
}

func CountLedgerEntries(t *testing.T, db *sql.DB, paymentID uuid.UUID) int {
	t.Helper()

	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM ledger_entries WHERE payment_id = $1`, paymentID).Scan(&count)
	if err != nil {
		t.Fatalf("count ledger entries for payment %s: %v", paymentID, err)
	}
	return count
}
