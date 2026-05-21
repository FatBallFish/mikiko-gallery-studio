package apikey

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	StatusActive   = "active"
	StatusDisabled = "disabled"
	StatusRevoked  = "revoked"
)

type APIKey struct {
	ID                   int64      `json:"id"`
	UserID               int64      `json:"-"`
	AccessKey            string     `json:"access_key"`
	SecretHash           string     `json:"-"`
	SecretCiphertext     string     `json:"-"`
	SigningSecret        string     `json:"-"`
	Name                 string     `json:"name"`
	Status               string     `json:"status"`
	GroupCode            string     `json:"group_code"`
	TotalQuotaPoints     *string    `json:"total_quota_points,omitempty"`
	DailyQuotaPoints     *string    `json:"daily_quota_points,omitempty"`
	TotalQuotaUsedPoints string     `json:"total_quota_used_points"`
	DailyQuotaUsedPoints string     `json:"daily_quota_used_points"`
	QuotaUsageDay        *string    `json:"quota_usage_day,omitempty"`
	RPMLimit             *int       `json:"rpm_limit,omitempty"`
	RPMWindowStartedAt   *time.Time `json:"rpm_window_started_at,omitempty"`
	RPMWindowCount       int        `json:"rpm_window_count"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	LastUsedAt           *time.Time `json:"last_used_at,omitempty"`
	DeletedAt            *time.Time `json:"-"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
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

func EncryptSigningSecret(secret, keyMaterial string) (string, error) {
	if strings.TrimSpace(secret) == "" {
		return "", nil
	}
	aead, err := signingSecretAEAD(keyMaterial)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate signing secret nonce: %w", err)
	}
	ciphertext := aead.Seal(nonce, nonce, []byte(secret), nil)
	return "v1:" + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func DecryptSigningSecret(material, keyMaterial string) (string, bool) {
	encoded := strings.TrimSpace(material)
	if !strings.HasPrefix(encoded, "v1:") {
		return "", false
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(encoded, "v1:"))
	if err != nil {
		return "", false
	}
	aead, err := signingSecretAEAD(keyMaterial)
	if err != nil || len(payload) < aead.NonceSize() {
		return "", false
	}
	nonce := payload[:aead.NonceSize()]
	sealed := payload[aead.NonceSize():]
	plaintext, err := aead.Open(nil, nonce, sealed, nil)
	if err != nil || len(plaintext) == 0 {
		return "", false
	}
	return string(plaintext), true
}

func signingSecretAEAD(keyMaterial string) (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(strings.TrimSpace(keyMaterial)))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("create signing secret cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
