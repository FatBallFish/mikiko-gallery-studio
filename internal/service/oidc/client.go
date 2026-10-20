// Package oidc implements the Mikiko ID (OIDC) authorization-code + PKCE
// login flow against any OpenID Connect provider (Keycloak realms). The
// client secret never leaves this backend: tokens are exchanged and verified
// server-side and only the resulting platform session is returned.
package oidc

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	oidclib "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const (
	// LoginStateTTL bounds how long an authorization round-trip may take.
	LoginStateTTL = 10 * time.Minute
	// TransactionCookieName is the HttpOnly cookie that carries the sealed
	// login transaction (state/nonce/verifier/return-to) between the
	// authorize redirect and the callback.
	TransactionCookieName = "pg_oidc_tx"

	providerCacheTTL = 10 * time.Minute
	scope            = "openid profile email"
)

// ProviderConfig describes one OIDC relying-party registration.
type ProviderConfig struct {
	IssuerURL    string
	ClientID     string
	ClientSecret string
	DisplayName  string
}

// Identity is the verified id_token identity used for account linking.
type Identity struct {
	Subject           string
	Email             string
	EmailVerified     bool
	Name              string
	PreferredUsername string
}

// LoginTransaction carries the CSRF state, nonce, PKCE verifier and the
// post-login return target across the browser round-trip.
type LoginTransaction struct {
	State     string    `json:"state"`
	Nonce     string    `json:"nonce"`
	Verifier  string    `json:"verifier"`
	ReturnTo  string    `json:"return_to"`
	ExpiresAt time.Time `json:"exp"`
}

// DiscoveryInfo reports the endpoints resolved from the issuer's discovery
// document; used by the admin connectivity test.
type DiscoveryInfo struct {
	Issuer                string
	AuthorizationEndpoint string
	TokenEndpoint         string
	UserInfoEndpoint      string
}

type cachedProvider struct {
	provider *oidclib.Provider
	expires  time.Time
}

type Client struct {
	mu         sync.Mutex
	providers  map[string]cachedProvider
	httpClient *http.Client
}

func NewClient(httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{providers: map[string]cachedProvider{}, httpClient: httpClient}
}

// Discover fetches and validates the issuer's discovery document.
func (c *Client) Discover(ctx context.Context, issuerURL string) (DiscoveryInfo, error) {
	provider, err := c.provider(ctx, issuerURL)
	if err != nil {
		return DiscoveryInfo{}, err
	}
	var endpoints struct {
		Issuer                string `json:"issuer"`
		AuthorizationEndpoint string `json:"authorization_endpoint"`
		TokenEndpoint         string `json:"token_endpoint"`
		UserInfoEndpoint      string `json:"userinfo_endpoint"`
	}
	if err := provider.Claims(&endpoints); err != nil {
		return DiscoveryInfo{}, errs.New(502, "OIDC_DISCOVERY_FAILED", "oidc discovery document is invalid")
	}
	return DiscoveryInfo{
		Issuer:                endpoints.Issuer,
		AuthorizationEndpoint: endpoints.AuthorizationEndpoint,
		TokenEndpoint:         endpoints.TokenEndpoint,
		UserInfoEndpoint:      endpoints.UserInfoEndpoint,
	}, nil
}

// BeginLogin builds the authorization redirect with PKCE(S256) and returns
// the transaction that must round-trip through the sealed cookie.
func (c *Client) BeginLogin(ctx context.Context, cfg ProviderConfig, redirectURI, returnTo string) (string, LoginTransaction, error) {
	provider, err := c.provider(ctx, cfg.IssuerURL)
	if err != nil {
		return "", LoginTransaction{}, err
	}
	state, err := randomURLToken()
	if err != nil {
		return "", LoginTransaction{}, err
	}
	nonce, err := randomURLToken()
	if err != nil {
		return "", LoginTransaction{}, err
	}
	verifier, err := randomURLToken()
	if err != nil {
		return "", LoginTransaction{}, err
	}
	authorizeConfig := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURI,
		Scopes:       []string{scope},
	}
	authorizeURL := authorizeConfig.AuthCodeURL(state,
		oauth2.SetAuthURLParam("nonce", nonce),
		oauth2.SetAuthURLParam("code_challenge", S256Challenge(verifier)),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
	return authorizeURL, LoginTransaction{
		State:     state,
		Nonce:     nonce,
		Verifier:  verifier,
		ReturnTo:  returnTo,
		ExpiresAt: time.Now().Add(LoginStateTTL),
	}, nil
}

// CompleteLogin exchanges the authorization code, verifies the id_token
// (issuer/audience/expiry/nonce) and returns the verified identity.
func (c *Client) CompleteLogin(ctx context.Context, cfg ProviderConfig, redirectURI, code string, tx LoginTransaction) (Identity, error) {
	if strings.TrimSpace(tx.State) == "" || strings.TrimSpace(tx.Verifier) == "" || strings.TrimSpace(tx.Nonce) == "" {
		return Identity{}, errs.New(400, "OIDC_STATE_INVALID", "oidc login transaction is missing or invalid")
	}
	if time.Now().After(tx.ExpiresAt) {
		return Identity{}, errs.New(400, "OIDC_STATE_EXPIRED", "oidc login transaction expired")
	}
	if strings.TrimSpace(code) == "" {
		return Identity{}, errs.New(400, "OIDC_CODE_MISSING", "authorization code is required")
	}
	ctx = oidclib.ClientContext(ctx, c.httpClient)
	provider, err := c.provider(ctx, cfg.IssuerURL)
	if err != nil {
		return Identity{}, err
	}
	exchangeConfig := oauth2.Config{
		ClientID:     cfg.ClientID,
		ClientSecret: cfg.ClientSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  redirectURI,
	}
	token, err := exchangeConfig.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", tx.Verifier))
	if err != nil {
		return Identity{}, errs.New(401, "OIDC_TOKEN_EXCHANGE_FAILED", "oidc token exchange failed")
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	if strings.TrimSpace(rawIDToken) == "" {
		return Identity{}, errs.New(401, "OIDC_ID_TOKEN_MISSING", "oidc provider did not return an id_token")
	}
	idToken, err := provider.Verifier(&oidclib.Config{ClientID: cfg.ClientID}).Verify(ctx, rawIDToken)
	if err != nil {
		return Identity{}, errs.New(401, "OIDC_ID_TOKEN_INVALID", "oidc id_token failed verification")
	}
	var claims struct {
		Nonce             string `json:"nonce"`
		Sub               string `json:"sub"`
		Email             string `json:"email"`
		EmailVerified     bool   `json:"email_verified"`
		Name              string `json:"name"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return Identity{}, errs.New(401, "OIDC_ID_TOKEN_INVALID", "oidc id_token claims are invalid")
	}
	if claims.Nonce != tx.Nonce {
		return Identity{}, errs.New(401, "OIDC_NONCE_MISMATCH", "oidc id_token nonce mismatch")
	}
	if strings.TrimSpace(claims.Sub) == "" {
		return Identity{}, errs.New(401, "OIDC_ID_TOKEN_INVALID", "oidc id_token has no subject")
	}
	return Identity{
		Subject:           claims.Sub,
		Email:             strings.TrimSpace(strings.ToLower(claims.Email)),
		EmailVerified:     claims.EmailVerified,
		Name:              strings.TrimSpace(claims.Name),
		PreferredUsername: strings.TrimSpace(claims.PreferredUsername),
	}, nil
}

func (c *Client) provider(ctx context.Context, issuerURL string) (*oidclib.Provider, error) {
	issuerURL = strings.TrimSpace(issuerURL)
	if issuerURL == "" {
		return nil, errs.BadRequest("oidc issuer is not configured")
	}
	key := issuerURL
	c.mu.Lock()
	cached, ok := c.providers[key]
	c.mu.Unlock()
	if ok && time.Now().Before(cached.expires) {
		return cached.provider, nil
	}
	provider, err := oidclib.NewProvider(oidclib.ClientContext(ctx, c.httpClient), issuerURL)
	if err != nil {
		return nil, errs.New(502, "OIDC_DISCOVERY_FAILED", fmt.Sprintf("oidc discovery failed for issuer %s", issuerURL))
	}
	c.mu.Lock()
	c.providers[key] = cachedProvider{provider: provider, expires: time.Now().Add(providerCacheTTL)}
	c.mu.Unlock()
	return provider, nil
}

// S256Challenge derives the RFC 7636 code challenge from a verifier.
func S256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func randomURLToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", errs.Internal("failed to generate oidc random token")
	}
	return base64.RawURLEncoding.EncodeToString(buffer), nil
}

// TransactionKey derives the HMAC key for sealing login transactions from the
// platform access-token secret (independent domain from JWT signing).
func TransactionKey(secret string) []byte {
	sum := sha256.Sum256([]byte("pic-gallery:oidc-tx:" + secret))
	return sum[:]
}

// SealTransaction encodes the transaction as a tamper-proof cookie value.
func SealTransaction(tx LoginTransaction, key []byte) string {
	payload, _ := json.Marshal(tx)
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	return encoded + "." + base64.RawURLEncoding.EncodeToString(hmacSum(key, encoded))
}

// OpenTransaction verifies and decodes a sealed transaction cookie value.
func OpenTransaction(sealed string, key []byte) (LoginTransaction, bool) {
	encoded, signature, ok := strings.Cut(sealed, ".")
	if !ok || encoded == "" || signature == "" {
		return LoginTransaction{}, false
	}
	if !hmac.Equal([]byte(signature), []byte(base64.RawURLEncoding.EncodeToString(hmacSum(key, encoded)))) {
		return LoginTransaction{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return LoginTransaction{}, false
	}
	var tx LoginTransaction
	if err := json.Unmarshal(payload, &tx); err != nil {
		return LoginTransaction{}, false
	}
	if time.Now().After(tx.ExpiresAt) {
		return LoginTransaction{}, false
	}
	return tx, true
}

func hmacSum(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(value))
	return mac.Sum(nil)
}
