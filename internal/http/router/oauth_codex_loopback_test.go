package router

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/service/oauthserver"
	_ "github.com/mattn/go-sqlite3"
)

func TestOAuthCodexLoopbackAuthorizeHTTP(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:"+filepath.Join(t.TempDir(), "oauth-loopback.db")+"?_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}

	// Pre-register synthetic Codex-shaped metadata: a URL client ID and
	// portless loopback callbacks. No remote metadata or credentials are used.
	const clientID = "https://client.example/oauth/native/test-client/client.json"
	const path = "/callback/test-client"
	service, err := oauthserver.New(entstore.NewOAuthStore(client), oauthserver.Config{
		PublicAPIURL: "https://studio.example", WebURL: "https://web.example",
		Clients: []oauthserver.Client{{
			ID: clientID, Name: "Synthetic Codex", TokenEndpointAuthMethod: "none",
			RedirectURIs: []string{"http://127.0.0.1" + path, "http://[::1]" + path},
			GrantTypes:   []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"},
		}},
		User: func(context.Context, int64) (oauthserver.User, error) {
			return oauthserver.User{ID: 7, Active: true}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	oauth := handlers.NewOAuth(service, nil)
	oauthMux := http.NewServeMux()
	oauthMux.HandleFunc("/oauth/authorize", oauth.Authorize)
	api := handlers.NewAPI(config.Config{}, nil, nil)
	api.SetDeveloperTransports(nil, oauthMux, nil, nil)
	handler := NewWithAPI(api)

	// Reserve an OS-selected port without making any callback connection.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	port := listener.Addr().(*net.TCPAddr).Port
	verifier := strings.Repeat("s", 43)
	sum := sha256.Sum256([]byte(verifier))
	for _, host := range []string{"127.0.0.1", "::1"} {
		actual := (&url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(port)), Path: path}).String()
		for _, tc := range []struct {
			name, redirect string
			status         int
		}{
			{"dynamic_port", actual, http.StatusSeeOther},
			{"changed_path", actual + "/other", http.StatusBadRequest},
			{"changed_query", actual + "?extra=1", http.StatusBadRequest},
		} {
			t.Run(host+"/"+tc.name, func(t *testing.T) {
				q := url.Values{
					"client_id": {clientID}, "redirect_uri": {tc.redirect},
					"response_type": {"code"}, "resource": {service.Resource()},
					"scope": {"account:read models:read"}, "state": {"synthetic-http-state"},
					"code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])},
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oauth/authorize?"+q.Encode(), nil))
				if rec.Code != tc.status {
					t.Fatalf("status=%d want=%d body=%s", rec.Code, tc.status, rec.Body.String())
				}
				if tc.status == http.StatusSeeOther {
					location, err := url.Parse(rec.Header().Get("Location"))
					if err != nil {
						t.Fatal(err)
					}
					if location.Scheme != "https" || location.Host != "web.example" || location.Path != "/" || location.Fragment != "oauth-consent" || location.Query().Get("oauth_request") == "" {
						t.Fatalf("unexpected consent redirect %q", location)
					}
				} else {
					var body struct {
						Error string `json:"error"`
					}
					if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body.Error != "invalid_request" {
						t.Fatalf("unexpected error response %s: %v", rec.Body.String(), err)
					}
					if rec.Header().Get("Location") != "" {
						t.Fatal("invalid callback received redirect")
					}
				}
			})
		}
	}
}
