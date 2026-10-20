// Package oidctest provides an in-process fake OpenID Connect provider for
// exercising the authorization-code + PKCE login flow end to end without a
// real Keycloak.
package oidctest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	jwt "github.com/golang-jwt/jwt/v5"
)

// Identity describes the claims stamped into issued id_tokens.
type Identity struct {
	Subject           string
	Email             string
	EmailVerified     bool
	Name              string
	PreferredUsername string
}

type codeBinding struct {
	challenge string
	nonce     string
}

// Server is a minimal OIDC provider: discovery, JWKS and a token endpoint that
// enforces client credentials and PKCE(S256) against the challenge captured
// when the code was issued.
type Server struct {
	*httptest.Server

	ClientID     string
	ClientSecret string

	mu           sync.Mutex
	key          *rsa.PrivateKey
	kid          string
	codes        map[string]codeBinding
	identity     Identity
	breakNonce   bool
	ignorePKCE   bool
}

func Start(t testing.TB) *Server {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{
		ClientID:     "mikiko_studio",
		ClientSecret: "test-secret",
		key:          key,
		kid:          "test-key-1",
		codes:        map[string]codeBinding{},
		identity:     Identity{Subject: "test-subject", Email: "oidc-user@example.com", EmailVerified: true, Name: "Mikiko Tester", PreferredUsername: "mikiko.tester"},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"issuer":                 server.URL,
			"authorization_endpoint": server.URL + "/authorize",
			"token_endpoint":         server.URL + "/token",
			"userinfo_endpoint":      server.URL + "/userinfo",
			"jwks_uri":               server.URL + "/keys",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA", "use": "sig", "alg": "RS256", "kid": server.kid,
				"n": base64.RawURLEncoding.EncodeToString(server.key.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}),
			}},
		})
	})
	mux.HandleFunc("POST /token", server.handleToken)
	server.Server = httptest.NewServer(mux)
	t.Cleanup(server.Server.Close)
	return server
}

func (s *Server) handleToken(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "invalid form", http.StatusBadRequest)
		return
	}
	if r.PostFormValue("client_id") != s.ClientID || r.PostFormValue("client_secret") != s.ClientSecret {
		http.Error(w, "invalid client", http.StatusUnauthorized)
		return
	}
	if r.PostFormValue("grant_type") != "authorization_code" {
		http.Error(w, "unsupported grant", http.StatusBadRequest)
		return
	}
	code := r.PostFormValue("code")
	s.mu.Lock()
	binding, ok := s.codes[code]
	identity := s.identity
	breakNonce := s.breakNonce
	ignorePKCE := s.ignorePKCE
	s.mu.Unlock()
	if !ok {
		http.Error(w, "unknown code", http.StatusBadRequest)
		return
	}
	verifier := r.PostFormValue("code_verifier")
	if !ignorePKCE && s256(verifier) != binding.challenge {
		http.Error(w, "pkce challenge mismatch", http.StatusBadRequest)
		return
	}
	now := time.Now()
	claims := jwt.MapClaims{
		"iss":                s.URL,
		"aud":                s.ClientID,
		"sub":                identity.Subject,
		"email":              identity.Email,
		"email_verified":     identity.EmailVerified,
		"name":               identity.Name,
		"preferred_username": identity.PreferredUsername,
		"iat":                now.Unix(),
		"exp":                now.Add(5 * time.Minute).Unix(),
	}
	if breakNonce {
		claims["nonce"] = "tampered-nonce"
	} else {
		claims["nonce"] = binding.nonce
	}
	idToken, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(s.key)
	if err != nil {
		http.Error(w, "sign failed", http.StatusInternalServerError)
		return
	}
	s.mu.Lock()
	delete(s.codes, code)
	s.mu.Unlock()
	writeJSON(w, map[string]any{
		"access_token": "fake-access-token",
		"token_type":   "Bearer",
		"expires_in":   300,
		"id_token":     idToken,
	})
}

// IssueCode registers an authorization code bound to the PKCE challenge and
// nonce captured from the client's authorize redirect.
func (s *Server) IssueCode(challenge, nonce string) string {
	code := fmt.Sprintf("code-%d-%s", time.Now().UnixNano(), nonce)
	s.mu.Lock()
	s.codes[code] = codeBinding{challenge: challenge, nonce: nonce}
	s.mu.Unlock()
	return code
}

func (s *Server) SetIdentity(identity Identity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.identity = identity
}

// BreakNonce makes subsequently issued id_tokens carry a wrong nonce.
func (s *Server) BreakNonce() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.breakNonce = true
}

func (s *Server) IgnorePKCE() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ignorePKCE = true
}

func s256(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func writeJSON(w http.ResponseWriter, payload map[string]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(payload)
}
