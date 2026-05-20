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
	StatusDeleted  = "deleted"
)

type APIKey struct {
	ID               int64      `json:"id"`
	UserID           int64      `json:"-"`
	AccessKey        string     `json:"access_key"`
	SecretHash       string     `json:"-"`
	SigningSecret    string     `json:"-"`
	Name             string     `json:"name"`
	Status           string     `json:"status"`
	GroupCode        string     `json:"group_code"`
	TotalQuotaPoints *string    `json:"total_quota_points,omitempty"`
	DailyQuotaPoints *string    `json:"daily_quota_points,omitempty"`
	RPMLimit         *int       `json:"rpm_limit,omitempty"`
	ExpiresAt        *time.Time `json:"expires_at,omitempty"`
	LastUsedAt       *time.Time `json:"last_used_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
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

func EncodeSigningSecret(secret string) string {
	if strings.TrimSpace(secret) == "" {
		return ""
	}
	return "plain:v1:" + base64.RawURLEncoding.EncodeToString([]byte(secret))
}

func DecodeSigningSecret(material string) (string, bool) {
	const prefix = "plain:v1:"
	if !strings.HasPrefix(material, prefix) {
		return "", false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(material, prefix))
	if err != nil || len(decoded) == 0 {
		return "", false
	}
	return string(decoded), true
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
	ciphertext := aead.Seal(nil, nonce, []byte(secret), nil)
	return "enc:v1:" + base64.RawURLEncoding.EncodeToString(nonce) + ":" + base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

func DecryptSigningSecret(material, keyMaterial string) (string, bool) {
	const prefix = "enc:v1:"
	if !strings.HasPrefix(material, prefix) {
		return "", false
	}
	parts := strings.Split(strings.TrimPrefix(material, prefix), ":")
	if len(parts) != 2 {
		return "", false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", false
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", false
	}
	aead, err := signingSecretAEAD(keyMaterial)
	if err != nil || len(nonce) != aead.NonceSize() {
		return "", false
	}
	plaintext, err := aead.Open(nil, nonce, ciphertext, nil)
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
