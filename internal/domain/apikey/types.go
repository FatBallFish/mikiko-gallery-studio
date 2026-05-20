package apikey

import (
	"crypto/sha256"
	"encoding/base64"
	"time"
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
)

type APIKey struct {
	ID         int64
	UserID     int64
	AccessKey  string
	SecretHash string
	Name       string
	Status     string
	GroupCode  string
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type Identity struct {
	APIKeyID  int64
	UserID    int64
	AccessKey string
	GroupCode string
}

func HashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
