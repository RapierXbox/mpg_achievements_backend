package utils

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GenerateTokenPair creates access and refresh tokens
func GenerateTokenPair(userID string, secret []byte, accessTTL, refreshTTL time.Duration) (string, string, error) {
	// access token claims
	accessClaims := jwt.MapClaims{
		"sub": userID, // subject (user ID)
		"exp": time.Now().Add(accessTTL).Unix(),
		"iat": time.Now().Unix(),
		"typ": "access",
	}

	// create and sign access token
	accessToken := jwt.NewWithClaims(jwt.SigningMethodHS256, accessClaims)
	accessSigned, err := accessToken.SignedString(secret)
	if err != nil {
		return "", "", err
	}

	// refresh token claims
	refreshClaims := jwt.MapClaims{
		"sub": userID,
		"exp": time.Now().Add(refreshTTL).Unix(),
		"iat": time.Now().Unix(),
		"typ": "refresh",
	}

	// create and sign refresh token
	refreshToken := jwt.NewWithClaims(jwt.SigningMethodHS256, refreshClaims)
	refreshSigned, err := refreshToken.SignedString(secret)

	return accessSigned, refreshSigned, err
}

// ParseToken validates and parses a JWT token
func ParseToken(tokenString string, secret []byte) (jwt.MapClaims, error) {
	// parse token with validation
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		// validate signing algorithm
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, jwt.ErrSignatureInvalid
		}
		return secret, nil
	})

	if err != nil {
		return nil, err
	}

	// verify token validity
	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	// extract and return claims
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return claims, nil
	}

	return nil, jwt.ErrTokenInvalidClaims
}

// ValidateToken checks if a token is valid and of specific type
func ValidateToken(tokenString, expectedType string, secret []byte) (jwt.MapClaims, error) {
	claims, err := ParseToken(tokenString, secret)
	if err != nil {
		return nil, err
	}

	// check token type
	if actualType, ok := claims["typ"].(string); !ok || actualType != expectedType {
		return nil, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}
