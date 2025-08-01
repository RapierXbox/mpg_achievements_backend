package utils

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"unicode"

	"golang.org/x/crypto/argon2"
)

const (
	saltLength  = 16
	timeCost    = 1         // Argon2 time parameter
	memoryCost  = 64 * 1024 // 64mb is a good balance between performance and security again brute force attack (compute time up to 100ms, but depends on hardware)
	parallelism = 4         // 4 threads
	keyLength   = 32        // 256-bit hash output
)

// HashToken creates a secure hash of a token using Argon2id
func HashToken(token, pepper string) string {
	// generate cryptographically secure salt
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		panic("crypto rand failed: " + err.Error())
	}

	// combine token with server side pepper
	data := []byte(token + pepper)

	// generate Argon2id hash
	hash := argon2.IDKey(
		data,
		salt,
		timeCost,
		memoryCost,
		parallelism,
		keyLength,
	)

	// format: salt$hash
	return fmt.Sprintf("%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
}

// VerifyToken compares a token against a stored hash
func VerifyToken(token, pepper, storedHash string) bool {
	// decode stored components
	components := make([][]byte, 2)
	for i, part := range strings.SplitN(storedHash, "$", 2) {
		decoded, err := base64.RawStdEncoding.DecodeString(part)
		if err != nil {
			return false
		}
		components[i] = decoded
	}

	salt := components[0]
	storedHashBytes := components[1]

	// recompute hash with same parameters
	data := []byte(token + pepper)
	computedHash := argon2.IDKey(
		data,
		salt,
		timeCost,
		memoryCost,
		parallelism,
		keyLength,
	)

	// constant time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare(storedHashBytes, computedHash) == 1
}

// HashPassword creates a secure hash of a password using Argon2id
func HashPassword(password, pepper string) (string, error) {
	// generate cryptographically secure salt
	salt := make([]byte, saltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("salt generation failed: %w", err)
	}

	// combine password with server-side pepper
	peppered := password + pepper

	// generate Argon2id hash
	hash := argon2.IDKey(
		[]byte(peppered),
		salt,
		timeCost,
		memoryCost,
		parallelism,
		keyLength,
	)

	// encode to string: salt$hash
	return fmt.Sprintf(
		"%s$%s",
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	), nil
}

// CheckPasswordHash verifies a password against a stored hash
func CheckPasswordHash(password, pepper, storedHash string) bool {
	// split stored hash into components
	parts := strings.Split(storedHash, "$")
	if len(parts) != 2 {
		return false
	}

	// decode salt
	salt, err := base64.RawStdEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}

	// decode stored hash
	storedHashBytes, err := base64.RawStdEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}

	// recompute hash with same parameters
	peppered := password + pepper
	computedHash := argon2.IDKey(
		[]byte(peppered),
		salt,
		timeCost,
		memoryCost,
		parallelism,
		keyLength,
	)

	// constant time comparison to prevent timing attacks
	return subtle.ConstantTimeCompare(storedHashBytes, computedHash) == 1
}

// ValidateEmail checks if an email address is properly formatted
func ValidateEmail(email string) bool {
	// simplified regex for basic validation
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}

func ValidatePassword(password string) bool {
	if len(password) < 8 { // check for lenght
		return false
	}

	var hasUpper, hasLower, hasNumber bool

	for _, c := range password { // check for big small and digits
		switch {
		case unicode.IsUpper(c):
			hasUpper = true
		case unicode.IsLower(c):
			hasLower = true
		case unicode.IsDigit(c):
			hasNumber = true
		}
	}

	return hasUpper && hasLower && hasNumber
}
