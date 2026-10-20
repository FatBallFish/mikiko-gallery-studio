package oidc

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/service/oidc/oidctest"
)

func testProviderConfig(idp *oidctest.Server) ProviderConfig {
	return ProviderConfig{
		IssuerURL:    idp.URL,
		ClientID:     idp.ClientID,
		ClientSecret: idp.ClientSecret,
		DisplayName:  "Mikiko ID",
	}
}

func TestBeginLoginBuildsPKCEAuthorizeURL(t *testing.T) {
	idp := oidctest.Start(t)
	client := NewClient(nil)
	authorizeURL, tx, err := client.BeginLogin(t.Context(), testProviderConfig(idp), "http://app.example.com/auth/callback", "http://app.example.com/home")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("client_id") != idp.ClientID {
		t.Fatalf("client_id mismatch: %s", query.Get("client_id"))
	}
	if query.Get("response_type") != "code" {
		t.Fatalf("response_type must be code: %s", query.Get("response_type"))
	}
	if query.Get("redirect_uri") != "http://app.example.com/auth/callback" {
		t.Fatalf("redirect_uri mismatch: %s", query.Get("redirect_uri"))
	}
	if !strings.Contains(query.Get("scope"), "openid") {
		t.Fatalf("scope must include openid: %s", query.Get("scope"))
	}
	if query.Get("state") != tx.State || query.Get("nonce") != tx.Nonce {
		t.Fatal("state/nonce must round-trip through the transaction")
	}
	if query.Get("code_challenge_method") != "S256" {
		t.Fatalf("code_challenge_method must be S256: %s", query.Get("code_challenge_method"))
	}
	if query.Get("code_challenge") != S256Challenge(tx.Verifier) {
		t.Fatal("code_challenge must be the S256 digest of the verifier")
	}
	if tx.ReturnTo != "http://app.example.com/home" {
		t.Fatalf("return_to mismatch: %s", tx.ReturnTo)
	}
	if time.Until(tx.ExpiresAt) <= 0 || time.Until(tx.ExpiresAt) > LoginStateTTL {
		t.Fatalf("transaction expiry out of range: %v", tx.ExpiresAt)
	}
}

func TestCompleteLoginRoundTrip(t *testing.T) {
	idp := oidctest.Start(t)
	client := NewClient(nil)
	redirectURI := "http://app.example.com/auth/callback"
	authorizeURL, tx, err := client.BeginLogin(t.Context(), testProviderConfig(idp), redirectURI, "")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(authorizeURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	code := idp.IssueCode(query.Get("code_challenge"), query.Get("nonce"))

	identity, err := client.CompleteLogin(t.Context(), testProviderConfig(idp), redirectURI, code, tx)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Subject != "test-subject" || identity.Email != "oidc-user@example.com" || !identity.EmailVerified {
		t.Fatalf("unexpected identity: %#v", identity)
	}
	if identity.Name != "Mikiko Tester" || identity.PreferredUsername != "mikiko.tester" {
		t.Fatalf("unexpected profile claims: %#v", identity)
	}
}

func TestCompleteLoginRejectsWrongPKCEVerifier(t *testing.T) {
	idp := oidctest.Start(t)
	client := NewClient(nil)
	redirectURI := "http://app.example.com/auth/callback"
	_, tx, err := client.BeginLogin(t.Context(), testProviderConfig(idp), redirectURI, "")
	if err != nil {
		t.Fatal(err)
	}
	code := idp.IssueCode(S256Challenge("a-completely-different-verifier-value-123456"), tx.Nonce)
	if _, err := client.CompleteLogin(t.Context(), testProviderConfig(idp), redirectURI, code, tx); err == nil {
		t.Fatal("pkce mismatch must fail the exchange")
	}
}

func TestCompleteLoginRejectsNonceMismatch(t *testing.T) {
	idp := oidctest.Start(t)
	idp.BreakNonce()
	client := NewClient(nil)
	redirectURI := "http://app.example.com/auth/callback"
	authorizeURL, tx, err := client.BeginLogin(t.Context(), testProviderConfig(idp), redirectURI, "")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(authorizeURL)
	query := parsed.Query()
	code := idp.IssueCode(query.Get("code_challenge"), query.Get("nonce"))
	if _, err := client.CompleteLogin(t.Context(), testProviderConfig(idp), redirectURI, code, tx); err == nil {
		t.Fatal("nonce mismatch must be rejected")
	}
}

func TestCompleteLoginRejectsExpiredTransaction(t *testing.T) {
	idp := oidctest.Start(t)
	client := NewClient(nil)
	_, tx, err := client.BeginLogin(t.Context(), testProviderConfig(idp), "http://app.example.com/auth/callback", "")
	if err != nil {
		t.Fatal(err)
	}
	tx.ExpiresAt = time.Now().Add(-time.Second)
	if _, err := client.CompleteLogin(t.Context(), testProviderConfig(idp), "http://app.example.com/auth/callback", "code", tx); err == nil {
		t.Fatal("expired transaction must be rejected")
	}
}

func TestSealOpenTransactionRoundTrip(t *testing.T) {
	key := TransactionKey("access-token-secret")
	tx := LoginTransaction{State: "s", Nonce: "n", Verifier: "v", ReturnTo: "http://app.example.com/x", ExpiresAt: time.Now().Add(time.Minute)}
	if opened, ok := OpenTransaction(SealTransaction(tx, key), key); !ok || opened.State != "s" || opened.Nonce != "n" || opened.Verifier != "v" || opened.ReturnTo != tx.ReturnTo {
		t.Fatalf("round-trip failed: %#v ok=%v", opened, ok)
	}
	if _, ok := OpenTransaction(SealTransaction(tx, key), TransactionKey("other-secret")); ok {
		t.Fatal("wrong key must not open the transaction")
	}
	if _, ok := OpenTransaction("tampered."+"AAAA", key); ok {
		t.Fatal("tampered payload must not open")
	}
	expired := tx
	expired.ExpiresAt = time.Now().Add(-time.Second)
	if _, ok := OpenTransaction(SealTransaction(expired, key), key); ok {
		t.Fatal("expired transaction must not open")
	}
}
