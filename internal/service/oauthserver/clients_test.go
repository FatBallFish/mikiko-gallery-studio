package oauthserver

import (
	"context"
	"errors"
	"net/netip"
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
