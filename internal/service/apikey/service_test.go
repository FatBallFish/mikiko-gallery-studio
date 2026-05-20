package apikey

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"strconv"
	"strings"
	"testing"
	"time"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
)

func TestAPIKeySecretStoresOnlyHashAndEncryptedSigningSecretAndAuthenticates(t *testing.T) {
	svc := NewService(nil)

	created, err := svc.CreateKey(context.Background(), CreateRequest{
		UserID:    42,
		Name:      "task4",
		GroupCode: "plus",
		Secret:    "sk-task4-secret",
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if created.Secret != "sk-task4-secret" {
		t.Fatalf("expected plaintext secret to be returned once on create")
	}
	if created.Key.SecretHash == "" || created.Key.SecretHash == "sk-task4-secret" {
		t.Fatalf("expected stored key to contain only a hash, got %#v", created.Key)
	}
	if created.Key.SigningSecret == "" || strings.Contains(created.Key.SigningSecret, "sk-task4-secret") {
		t.Fatalf("expected encrypted signing secret, got %q", created.Key.SigningSecret)
	}
	if decoded, ok := domainapikey.DecodeSigningSecret(created.Key.SigningSecret); ok || decoded == "sk-task4-secret" {
		t.Fatalf("expected signing secret not to use legacy reversible format, decoded=%q ok=%v", decoded, ok)
	}
	if raw, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(created.Key.SigningSecret, "enc:v1:")); err == nil && string(raw) == "sk-task4-secret" {
		t.Fatalf("expected signing secret not to be simple base64 of the raw secret")
	}

	identity, err := svc.AuthenticateNative(context.Background(), created.Key.AccessKey, "sk-task4-secret")
	if err != nil {
		t.Fatalf("AuthenticateNative: %v", err)
	}
	if identity.UserID != 42 || identity.APIKeyID == 0 || identity.GroupCode != "plus" {
		t.Fatalf("unexpected native identity %#v", identity)
	}

	bearer, err := svc.AuthenticateBearer(context.Background(), "sk-task4-secret")
	if err != nil {
		t.Fatalf("AuthenticateBearer: %v", err)
	}
	if bearer.APIKeyID != identity.APIKeyID {
		t.Fatalf("expected bearer identity to match native identity, got %#v want %#v", bearer, identity)
	}
}

func TestAPIKeyHMACUsesEncryptedSigningSecret(t *testing.T) {
	svc := NewServiceWithSigningSecretKey(nil, "test-api-key-signing-encryption-key")
	created, err := svc.CreateKey(context.Background(), CreateRequest{
		UserID:    42,
		Name:      "canonical",
		GroupCode: "plus",
		Secret:    "sk-canonical-secret",
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	emptyBodySum := sha256.Sum256(nil)
	bodySHA := hex.EncodeToString(emptyBodySum[:])
	canonical := "POST" + "/api/open/image/v1/tasks" + timestamp + bodySHA
	mac := hmac.New(sha256.New, []byte("sk-canonical-secret"))
	_, _ = mac.Write([]byte(canonical))

	identity, err := svc.AuthenticateCanonical(context.Background(), "POST", "/api/open/image/v1/tasks", timestamp, bodySHA, created.Key.AccessKey, hex.EncodeToString(mac.Sum(nil)), time.Minute)
	if err != nil {
		t.Fatalf("AuthenticateCanonical: %v", err)
	}
	if identity.UserID != 42 || identity.GroupCode != "plus" {
		t.Fatalf("unexpected identity %#v", identity)
	}
}

func TestAuthenticateRejectsMissingInvalidDisabledAndExpiredKeys(t *testing.T) {
	svc := NewService(nil)
	created, err := svc.CreateKey(context.Background(), CreateRequest{UserID: 7, Name: "reject", GroupCode: "basic", Secret: "sk-reject"})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	if _, err := svc.AuthenticateNative(context.Background(), "ak-missing", "sk-reject"); err == nil {
		t.Fatal("expected missing access key to be rejected")
	}
	if _, err := svc.AuthenticateNative(context.Background(), created.Key.AccessKey, "wrong-secret"); err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}

	if err := svc.SetStatusForTest(context.Background(), created.Key.ID, "disabled"); err != nil {
		t.Fatalf("SetStatusForTest disabled: %v", err)
	}
	if _, err := svc.AuthenticateNative(context.Background(), created.Key.AccessKey, "sk-reject"); err == nil {
		t.Fatal("expected disabled key to be rejected")
	}

	if err := svc.SetStatusForTest(context.Background(), created.Key.ID, "active"); err != nil {
		t.Fatalf("SetStatusForTest active: %v", err)
	}
	if err := svc.SetExpiresAtForTest(context.Background(), created.Key.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("SetExpiresAtForTest: %v", err)
	}
	if _, err := svc.AuthenticateBearer(context.Background(), "sk-reject"); err == nil {
		t.Fatal("expected expired bearer key to be rejected")
	}
}
