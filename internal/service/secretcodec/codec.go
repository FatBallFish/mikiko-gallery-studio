package secretcodec

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Codec struct {
	keyMaterial string
}

func New(keyMaterial string) *Codec {
	return &Codec{keyMaterial: strings.TrimSpace(keyMaterial)}
}

func (c *Codec) EncryptJSON(value map[string]any) (map[string]any, error) {
	value = normalizeMap(value)
	if len(value) == 0 {
		return map[string]any{}, nil
	}
	if c == nil || c.keyMaterial == "" {
		return value, nil
	}
	plain, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal secure config secret: %w", err)
	}
	aead, err := c.aead()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate secure config nonce: %w", err)
	}
	ciphertext := aead.Seal(nonce, nonce, plain, nil)
	return map[string]any{"ciphertext": "v1:" + base64.RawURLEncoding.EncodeToString(ciphertext)}, nil
}

func (c *Codec) DecryptJSON(envelope map[string]any) (map[string]any, error) {
	envelope = normalizeMap(envelope)
	ciphertext, _ := envelope["ciphertext"].(string)
	ciphertext = strings.TrimSpace(ciphertext)
	if ciphertext == "" {
		return envelope, nil
	}
	if c == nil || c.keyMaterial == "" {
		return map[string]any{}, nil
	}
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(ciphertext, "v1:"))
	if err != nil || !strings.HasPrefix(ciphertext, "v1:") {
		return map[string]any{}, nil
	}
	aead, err := c.aead()
	if err != nil {
		return nil, err
	}
	if len(payload) < aead.NonceSize() {
		return map[string]any{}, nil
	}
	nonce := payload[:aead.NonceSize()]
	sealed := payload[aead.NonceSize():]
	plain, err := aead.Open(nil, nonce, sealed, nil)
	if err != nil {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(plain, &decoded); err != nil {
		return map[string]any{}, nil
	}
	return normalizeMap(decoded), nil
}

func Fingerprint(values map[string]any, secretFields []string) string {
	parts := make([]string, 0, len(secretFields))
	for _, key := range secretFields {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if value, ok := values[key]; ok && strings.TrimSpace(fmt.Sprint(value)) != "" {
			parts = append(parts, fmt.Sprintf("%s=%v", key, value))
		}
	}
	sort.Strings(parts)
	if len(parts) == 0 {
		return ""
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\n")))
	return fmt.Sprintf("sha256:%x", sum[:8])
}

func (c *Codec) aead() (cipher.AEAD, error) {
	sum := sha256.Sum256([]byte(c.keyMaterial))
	block, err := aes.NewCipher(sum[:])
	if err != nil {
		return nil, fmt.Errorf("create secure config cipher: %w", err)
	}
	return cipher.NewGCM(block)
}

func normalizeMap(value map[string]any) map[string]any {
	if value == nil {
		return map[string]any{}
	}
	normalized := make(map[string]any, len(value))
	for key, raw := range value {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		normalized[key] = raw
	}
	return normalized
}
