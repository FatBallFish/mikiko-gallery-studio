package oauthserver

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type rateStore struct {
	mu   sync.Mutex
	rows map[string]Record
}

func (m *rateStore) Get(_ context.Context, id string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[id]
	if !ok {
		return r, ErrNotFound
	}
	return r, nil
}
func (m *rateStore) Create(_ context.Context, r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.rows[r.ID]; ok {
		return ErrConflict
	}
	m.rows[r.ID] = r
	return nil
}
func (m *rateStore) CompareSwap(_ context.Context, r Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.rows[r.ID].Version != r.Version {
		return ErrConflict
	}
	r.Version++
	m.rows[r.ID] = r
	return nil
}
func (m *rateStore) List(context.Context, string, int64) ([]Record, error) { return nil, nil }

func TestRegistrationRateWindowRecoversAcrossServices(t *testing.T) {
	ctx := context.Background()
	store := &rateStore{rows: map[string]Record{}}
	// Old lifetime exhaustion must not disable registration after upgrade.
	store.rows["registration-limit"] = document("registration-limit", "limit", 0, map[string]int{"Count": 100, "Next": 9999999999})
	s := &Service{store: store}
	restarted := &Service{store: store}
	now := time.Unix(1000, 0)
	for window := 0; window < 12; window++ {
		for i := 0; i < registrationBurst; i++ {
			if err := s.reserveRegistrationAt(ctx, now); err != nil {
				t.Fatal(err)
			}
		}
		err := restarted.reserveRegistrationAt(ctx, now.Add(250*time.Millisecond))
		var rate *RateLimitError
		if !errors.As(err, &rate) || rate.RetryAfter != 59750*time.Millisecond {
			t.Fatalf("rate=%+v err=%v", rate, err)
		}
		now = now.Add(registrationWindow)
	}
	if len(store.rows) != 2 {
		t.Fatalf("unbounded limiter records: %d", len(store.rows))
	}
}

func TestRegistrationRateConcurrentBound(t *testing.T) {
	store := &rateStore{rows: map[string]Record{}}
	s := &Service{store: store}
	now := time.Unix(1000, 0)
	var wg sync.WaitGroup
	results := make(chan error, 40)
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.reserveRegistrationAt(context.Background(), now) }()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else {
			var rate *RateLimitError
			if !errors.As(err, &rate) && !errors.Is(err, ErrConflict) {
				t.Fatal(err)
			}
		}
	}
	if accepted != registrationBurst {
		t.Fatalf("accepted=%d", accepted)
	}
}

func TestOAuthConsentExpiryIsDistinct(t *testing.T) {
	ctx := context.Background()
	store := &rateStore{rows: map[string]Record{}}
	s := &Service{store: store, cfg: Config{User: func(context.Context, int64) (User, error) { return User{Active: true}, nil }}}
	a := Authorization{ID: "expired", UserID: 7, CSRFHash: hash("csrf"), Expires: time.Now().Add(-time.Second).Unix()}
	if err := store.Create(ctx, document("request:expired", "request", 7, a)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Consent(ctx, "expired", 7); !errors.Is(err, ErrAuthorizationExpired) {
		t.Fatalf("consent: %v", err)
	}
	if _, err := s.Decide(ctx, "expired", 7, "csrf", true); !errors.Is(err, ErrAuthorizationExpired) {
		t.Fatalf("decision: %v", err)
	}
	if _, _, err := s.Consent(ctx, "missing", 7); errors.Is(err, ErrAuthorizationExpired) || !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing mislabeled: %v", err)
	}
}

func TestOAuthMetadataAddressPolicy(t *testing.T) {
	for _, raw := range []string{"127.0.0.1", "::1", "10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.169.254", "100.100.100.200", "198.18.0.1", "192.0.2.1", "2001:db8::1", "64:ff9b::a00:1", "::ffff:127.0.0.1"} {
		if publicAddress(netip.MustParseAddr(raw)) {
			t.Errorf("accepted nonpublic address %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "2606:4700:4700::1111"} {
		if !publicAddress(netip.MustParseAddr(raw)) {
			t.Errorf("rejected public address %s", raw)
		}
	}
}
func TestOAuthLoopbackMetadataAuthorization(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "[::1]"} {
		t.Run(host, func(t *testing.T) {
			ctx := context.Background()
			store := &rateStore{rows: map[string]Record{}}
			s, err := New(store, Config{PublicAPIURL: "https://studio.example", User: func(context.Context, int64) (User, error) { return User{ID: 7, Active: true}, nil }})
			if err != nil {
				t.Fatal(err)
			}
			// Exercise the real metadata-cache resolution path without network or
			// user-provided URLs, state, PKCE material or credentials.
			const clientID = "https://client.example/native/client.json"
			registered := "http://" + host + "/callback/test-native?aud=studio"
			actual := "http://" + host + ":49152/callback/test-native?aud=studio"
			metadata, err := json.Marshal(map[string]any{"client_id": clientID, "client_name": "Native test", "application_type": "native", "redirect_uris": []string{registered}, "token_endpoint_auth_method": "none"})
			if err != nil {
				t.Fatal(err)
			}
			var c Client
			if err := json.Unmarshal(metadata, &c); err != nil || validateClient(c) != nil {
				t.Fatalf("metadata: %v", err)
			}
			if err := store.Create(ctx, document("client:"+clientID, "client", 0, cachedClient{Client: c, Expires: time.Now().Add(time.Minute).Unix()})); err != nil {
				t.Fatal(err)
			}
			verifier := strings.Repeat("t", 43)
			sum := sha256.Sum256([]byte(verifier))
			q := url.Values{"client_id": {clientID}, "redirect_uri": {actual}, "response_type": {"code"}, "resource": {s.Resource()}, "scope": {"account:read"}, "code_challenge_method": {"S256"}, "code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "state": {"synthetic-state"}}
			for _, bad := range []string{
				strings.Replace(actual, "/callback/test-native", "/callback/other", 1),
				strings.Replace(actual, host, "localhost", 1),
				strings.Replace(actual, "aud=studio", "aud=other", 1),
				strings.Replace(actual, host, "192.0.2.1", 1),
				actual + "#fragment",
				strings.Replace(actual, "://", "://user@", 1),
				strings.Replace(actual, ":49152", ":65536", 1),
			} {
				q.Set("redirect_uri", bad)
				if _, err := s.Begin(ctx, q); !errors.Is(err, ErrInvalid) {
					t.Fatalf("accepted callback %s: %v", bad, err)
				}
			}
			q.Set("redirect_uri", actual)
			id, err := s.Begin(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			a, csrf, err := s.Consent(ctx, id, 7)
			if err != nil || a.RedirectURI != actual {
				t.Fatalf("actual callback not preserved: %v", err)
			}
			callback, err := s.Decide(ctx, id, 7, csrf, true)
			if err != nil {
				t.Fatal(err)
			}
			u, err := url.Parse(callback)
			if err != nil || u.Host != host+":49152" || u.Query().Get("state") != "synthetic-state" || u.Query().Get("aud") != "studio" {
				t.Fatalf("callback binding: %s %v", callback, err)
			}
			form := url.Values{"grant_type": {"authorization_code"}, "client_id": {clientID}, "resource": {s.Resource()}, "code": {u.Query().Get("code")}, "code_verifier": {verifier}}
			for _, bad := range []string{registered, strings.Replace(actual, ":49152", ":49153", 1)} {
				form.Set("redirect_uri", bad)
				if _, err := s.Token(ctx, form); !errors.Is(err, ErrInvalid) {
					t.Fatalf("token accepted changed callback %s: %v", bad, err)
				}
			}
			form.Set("redirect_uri", actual)
			token, err := s.Token(ctx, form)
			if err != nil {
				t.Fatal(err)
			}
			if p, err := s.ValidateAccess(ctx, token.AccessToken); err != nil || p.ClientID != clientID || p.UserID != 7 {
				t.Fatalf("access principal: %+v %v", p, err)
			}
		})
	}
}

func TestOAuthRedirectPortExceptionBoundaries(t *testing.T) {
	for _, tc := range []struct {
		registered, requested string
		want                  bool
	}{
		{"http://127.0.0.1/cb", "http://127.0.0.1:49152/cb", true},
		{"http://[::1]/cb", "http://[::1]:49152/cb", true},
		{"http://127.0.0.1:1234/cb", "http://127.0.0.1:49152/cb", true},
		{"https://client.example/cb?x=1", "https://client.example/cb?x=1", true},
		{"http://localhost:1234/cb", "http://localhost:1234/cb", true},
		{"http://localhost/cb", "http://localhost:49152/cb", false},
		{"http://127.0.0.1/cb", "http://127.0.0.2:49152/cb", false},
		{"http://127.0.0.1/cb", "http://[::1]:49152/cb", false},
		{"https://127.0.0.1/cb", "https://127.0.0.1:49152/cb", false},
		{"https://client.example/cb", "https://client.example:443/cb", false},
		{"http://127.0.0.1/cb", "http://127.0.0.1:49152/cb?", false},
		{"http://127.0.0.1/cb?a=1&b=2", "http://127.0.0.1:49152/cb?b=2&a=1", false},
		{"http://127.0.0.1/cb", "http://127.0.0.1:49152/%63b", false},
		{"http://127.0.0.1/cb", "http://127.0.0.1:49152/a/../cb", false},
		{"http://remote.example/cb", "http://remote.example:49152/cb", false},
	} {
		if got := redirectURIMatches(tc.registered, tc.requested); got != tc.want {
			t.Errorf("match(%q, %q) = %v, want %v", tc.registered, tc.requested, got, tc.want)
		}
	}
	for _, bad := range []string{
		"http://127.0.0.1:0/cb", "http://127.0.0.1:65536/cb", "http://127.0.0.1:/cb", "http://127.0.0.1:abc/cb", "http://127.0.0.1:-1/cb",
		"http://[::1]:65536/cb", "http://[::1]:/cb", "http://::1/cb",
		"https://client.example:65536/cb", "http://user@127.0.0.1/cb", "http://127.0.0.1/cb#", "http://127.0.0.1/cb#fragment",
	} {
		if validateClient(Client{Name: "test", RedirectURIs: []string{bad}}) == nil || redirectURIMatches(bad, bad) {
			t.Errorf("accepted invalid callback %q", bad)
		}
	}
}

func TestOAuthRejectUnsafeClientMetadata(t *testing.T) {
	for _, redirect := range []string{"https://good.example/cb#fragment", "http://remote.example/cb", "https://user:pass@example.com/cb", "javascript:alert(1)", "https:///cb"} {
		if validateClient(Client{Name: "App", RedirectURIs: []string{redirect}}) == nil {
			t.Errorf("accepted %s", redirect)
		}
	}
	if validateClient(Client{Name: "App", RedirectURIs: []string{"https://example.com/cb"}, TokenEndpointAuthMethod: "client_secret_basic"}) == nil {
		t.Fatal("accepted unsupported auth method")
	}
}
