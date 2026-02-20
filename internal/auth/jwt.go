package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID uuid.UUID
	Email  string
}

type tokenClaims struct {
	jwt.RegisteredClaims
	UserID string `json:"user_id"`
	Email  string `json:"email"`
}

func GenerateToken(userID uuid.UUID, email string, secret string, expiry time.Duration) (string, error) {
	now := time.Now()
	claims := tokenClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(expiry)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID: userID.String(),
		Email:  email,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("GenerateToken: %w", err)
	}
	return signed, nil
}

func ValidateToken(tokenString string, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &tokenClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, fmt.Errorf("ValidateToken: %w", err)
	}

	tc, ok := token.Claims.(*tokenClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("ValidateToken: invalid token claims")
	}

	userID, err := uuid.Parse(tc.UserID)
	if err != nil {
		return nil, fmt.Errorf("ValidateToken: invalid user_id in token: %w", err)
	}

	return &Claims{
		UserID: userID,
		Email:  tc.Email,
	}, nil
}
