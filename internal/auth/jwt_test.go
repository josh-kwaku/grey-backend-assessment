package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testSecret = "test-jwt-secret"

func TestGenerateAndValidateToken(t *testing.T) {
	userID := uuid.New()
	email := "user@test.com"

	token, err := GenerateToken(userID, email, testSecret, 24*time.Hour)
	require.NoError(t, err)
	require.NotEmpty(t, token)

	claims, err := ValidateToken(token, testSecret)
	require.NoError(t, err)
	assert.Equal(t, userID, claims.UserID)
	assert.Equal(t, email, claims.Email)
}

func TestValidateToken(t *testing.T) {
	userID := uuid.New()
	email := "user@test.com"

	validToken, err := GenerateToken(userID, email, testSecret, 24*time.Hour)
	require.NoError(t, err)

	expiredToken, err := GenerateToken(userID, email, testSecret, -1*time.Hour)
	require.NoError(t, err)

	tests := []struct {
		name      string
		token     string
		secret    string
		wantErrIs error
	}{
		{
			name:      "expired token",
			token:     expiredToken,
			secret:    testSecret,
			wantErrIs: jwt.ErrTokenExpired,
		},
		{
			name:      "wrong secret",
			token:     validToken,
			secret:    "wrong-secret",
			wantErrIs: jwt.ErrTokenSignatureInvalid,
		},
		{
			name:      "malformed token",
			token:     "not.a.valid.jwt",
			secret:    testSecret,
			wantErrIs: jwt.ErrTokenMalformed,
		},
		{
			name:      "empty token",
			token:     "",
			secret:    testSecret,
			wantErrIs: jwt.ErrTokenMalformed,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ValidateToken(tc.token, tc.secret)
			require.Error(t, err)
			assert.ErrorIs(t, err, tc.wantErrIs)
		})
	}
}

func TestValidateToken_RejectsNonHMAC(t *testing.T) {
	// Algorithm confusion: a token signed with "none" should be rejected
	claims := tokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
		UserID: uuid.NewString(),
		Email:  "user@test.com",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claims)
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	require.NoError(t, err)

	_, err = ValidateToken(signed, testSecret)
	require.Error(t, err)
}
