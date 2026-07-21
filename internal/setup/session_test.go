package setup

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestSessionClaimsAreStrictSignedAndExpire(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	clock := &manualClock{now: now}
	service, err := NewAuthService(AuthConfig{
		Token: generatedTokenForTest(t, 10), Version: 12, SessionTTL: 5 * time.Minute,
		Rand: &sequenceReader{next: 12}, Clock: clock.Read, RateLimit: defaultTestRateLimit(),
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	session, err := service.Exchange("2001:db8::1", generatedTokenForTest(t, 10))
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(session)
	if err != nil || len(decoded) != sessionPayloadSize+sessionSignatureSize {
		t.Fatalf("session format decode=%d bytes err=%v", len(decoded), err)
	}
	if err := service.VerifySession(session); err != nil {
		t.Fatalf("VerifySession returned error: %v", err)
	}

	mutated := append([]byte(nil), decoded...)
	mutated[len(mutated)-1] ^= 1
	if err := service.VerifySession(base64.RawURLEncoding.EncodeToString(mutated)); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("tampered session error = %v", err)
	}
	for _, malformed := range []string{session + "=", session[:len(session)-1], session + "A", strings.Repeat("A", len(session))} {
		if err := service.VerifySession(malformed); !errors.Is(err, ErrInvalidSession) {
			t.Errorf("malformed session error = %v", err)
		}
	}

	clock.Advance(5 * time.Minute)
	if err := service.VerifySession(session); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("expired session error = %v", err)
	}
}

func TestAuthConstructionAndClockFailuresFailClosed(t *testing.T) {
	token := generatedTokenForTest(t, 11)
	cases := []AuthConfig{
		{Token: token, Version: 0, SessionTTL: 5 * time.Minute, Rand: &sequenceReader{}, Clock: fixedClock(time.Now()), RateLimit: defaultTestRateLimit()},
		{Token: token + "=", Version: 1, SessionTTL: 5 * time.Minute, Rand: &sequenceReader{}, Clock: fixedClock(time.Now()), RateLimit: defaultTestRateLimit()},
		{Token: token, Version: 1, SessionTTL: time.Second, Rand: &sequenceReader{}, Clock: fixedClock(time.Now()), RateLimit: defaultTestRateLimit()},
		{Token: token, Version: 1, SessionTTL: 25 * time.Hour, Rand: &sequenceReader{}, Clock: fixedClock(time.Now()), RateLimit: defaultTestRateLimit()},
	}
	for index, cfg := range cases {
		if _, err := NewAuthService(cfg); !errors.Is(err, ErrInvalidConfiguration) {
			t.Errorf("case %d NewAuthService error = %v", index, err)
		}
	}

	service, err := NewAuthService(AuthConfig{
		Token: token, Version: 1, SessionTTL: 5 * time.Minute,
		Rand: &sequenceReader{}, Clock: func() (time.Time, error) { return time.Time{}, errors.New("secret clock detail") },
		RateLimit: defaultTestRateLimit(),
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	if _, err := service.Exchange("192.0.2.1", token); !errors.Is(err, ErrClock) || strings.Contains(err.Error(), "secret clock detail") {
		t.Fatalf("clock failure was not sanitized/fail-closed: %v", err)
	}

	for _, unsafeTime := range []time.Time{
		time.Unix(-100, 0),
		time.Date(9999, 12, 31, 23, 59, 30, 0, time.UTC),
	} {
		unsafeService, err := NewAuthService(AuthConfig{
			Token: token, Version: 1, SessionTTL: 5 * time.Minute,
			Rand: &sequenceReader{}, Clock: fixedClock(unsafeTime), RateLimit: defaultTestRateLimit(),
		})
		if err != nil {
			t.Fatalf("NewAuthService unsafe clock fixture: %v", err)
		}
		if _, err := unsafeService.Exchange("192.0.2.1", token); !errors.Is(err, ErrClock) {
			t.Errorf("unsafe clock %v exchange error = %v", unsafeTime, err)
		}
	}
}

func TestSetupSessionCookieAttributesForHTTPAndHTTPS(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	for _, secure := range []bool{false, true} {
		cookie, err := NewSetupSessionCookie("signed-session", now, 10*time.Minute, secure)
		if err != nil {
			t.Fatalf("NewSetupSessionCookie returned error: %v", err)
		}
		if cookie.Name != SetupSessionCookieName || cookie.Value != "signed-session" || cookie.Path != SetupSessionCookiePath {
			t.Fatalf("unexpected cookie identity: %#v", cookie)
		}
		if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Secure != secure {
			t.Fatalf("unexpected cookie security attributes: %#v", cookie)
		}
		if !cookie.Expires.Equal(now.Add(10*time.Minute)) || cookie.MaxAge != 600 {
			t.Fatalf("unexpected cookie lifetime: %#v", cookie)
		}
	}

	cleared := ClearSetupSessionCookie(false)
	if cleared.Name != SetupSessionCookieName || cleared.Path != SetupSessionCookiePath || cleared.MaxAge != -1 || !cleared.HttpOnly || cleared.SameSite != http.SameSiteStrictMode {
		t.Fatalf("unexpected clearing cookie: %#v", cleared)
	}
}

func TestRateLimitTracksNormalizedIPAndTokenFingerprint(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	clock := &manualClock{now: now}
	token := generatedTokenForTest(t, 12)
	service, err := NewAuthService(AuthConfig{
		Token: token, Version: 1, SessionTTL: 5 * time.Minute, Rand: &sequenceReader{}, Clock: clock.Read,
		RateLimit: RateLimitConfig{Window: time.Minute, IPAttempts: 2, TokenAttempts: 3, MaxEntries: 10},
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}

	for _, candidate := range []string{generatedTokenForTest(t, 13), generatedTokenForTest(t, 14)} {
		if _, err := service.Exchange("[::ffff:192.0.2.50]:4000", candidate); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("invalid token attempt error = %v", err)
		}
	}
	if _, err := service.Exchange("192.0.2.50", generatedTokenForTest(t, 15)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("normalized same-IP limit error = %v", err)
	}

	for _, ip := range []string{"192.0.2.60", "192.0.2.61", "192.0.2.62"} {
		if _, err := service.Exchange(ip, token); err != nil {
			t.Fatalf("valid token attempt from %s: %v", ip, err)
		}
	}
	if _, err := service.Exchange("192.0.2.63", token); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("same-token multi-IP limit error = %v", err)
	}

	clock.Advance(time.Minute)
	if _, err := service.Exchange("192.0.2.50", token); err != nil {
		t.Fatalf("rate limit did not recover after window: %v", err)
	}
}

func TestRateLimitCountsSuccessfulAttemptsEvictsOldestAndBoundsMemory(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	clock := &manualClock{now: now}
	token := generatedTokenForTest(t, 16)
	service, err := NewAuthService(AuthConfig{
		Token: token, Version: 1, SessionTTL: 5 * time.Minute, Rand: &sequenceReader{}, Clock: clock.Read,
		RateLimit: RateLimitConfig{Window: time.Minute, IPAttempts: 2, TokenAttempts: 10, MaxEntries: 2},
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := service.Exchange("192.0.2.70", token); err != nil {
			t.Fatalf("successful attempt %d: %v", attempt, err)
		}
	}
	if _, err := service.Exchange("192.0.2.70", token); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("successful attempts were not counted: %v", err)
	}

	if _, err := service.Exchange("192.0.2.71", generatedTokenForTest(t, 17)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("first capacity attempt: %v", err)
	}
	if _, err := service.Exchange("192.0.2.72", generatedTokenForTest(t, 18)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("capacity pressure globally denied a new fingerprint: %v", err)
	}
	if ipEntries, tokenEntries := service.rateLimitEntryCountsForTest(); ipEntries > 2 || tokenEntries > 2 {
		t.Fatalf("rate limiter exceeded bounds: ip=%d token=%d", ipEntries, tokenEntries)
	}

	clock.Advance(time.Minute)
	if _, err := service.Exchange("192.0.2.72", generatedTokenForTest(t, 18)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired entries were not cleaned: %v", err)
	}
}

func TestRateLimitUniqueFingerprintFloodCannotPermanentlyDenyAdministrator(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	clock := &manualClock{now: now}
	adminToken := generatedTokenForTest(t, 30)
	service, err := NewAuthService(AuthConfig{
		Token: adminToken, Version: 1, SessionTTL: 5 * time.Minute, Rand: &sequenceReader{}, Clock: clock.Read,
		RateLimit: RateLimitConfig{Window: time.Hour, IPAttempts: 3, TokenAttempts: 3, MaxEntries: 4},
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	for index := 1; index <= 40; index++ {
		candidate := generatedTokenForTest(t, byte(30+index))
		if _, err := service.Exchange("198.51.100."+itoaForTest((index%90)+1), candidate); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("flood attempt %d error = %v, want invalid token without global lockout", index, err)
		}
	}
	if _, err := service.Exchange("203.0.113.200", adminToken); err != nil {
		t.Fatalf("legitimate administrator remained denied after unique flood: %v", err)
	}
	if ipEntries, tokenEntries := service.rateLimitEntryCountsForTest(); ipEntries > 4 || tokenEntries > 4 {
		t.Fatalf("rate limiter exceeded bounds after flood: ip=%d token=%d", ipEntries, tokenEntries)
	}
}

func TestRateLimitExpiredCleanupIsAmortizedAndPreservesFingerprintLimits(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	clock := &manualClock{now: now}
	token := generatedTokenForTest(t, 80)
	service, err := NewAuthService(AuthConfig{
		Token: token, Version: 1, SessionTTL: 5 * time.Minute, Rand: &sequenceReader{}, Clock: clock.Read,
		RateLimit: RateLimitConfig{Window: time.Minute, IPAttempts: 2, TokenAttempts: 1000, MaxEntries: 512},
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	for index := 1; index <= 500; index++ {
		if _, err := service.Exchange("198.18.0."+itoaForTest((index%90)+1), generatedTokenForTest(t, byte(index))); err != nil && !errors.Is(err, ErrInvalidToken) && !errors.Is(err, ErrRateLimited) {
			t.Fatalf("seed attempt %d error = %v", index, err)
		}
	}
	clock.Advance(time.Minute)
	if _, err := service.Exchange("203.0.113.210", token); err != nil {
		t.Fatalf("administrator failed after expiry cleanup: %v", err)
	}
	for attempt := 0; attempt < 2; attempt++ {
		if _, err := service.Exchange("203.0.113.211", generatedTokenForTest(t, byte(110+attempt))); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("fingerprint-limit seed %d error = %v", attempt, err)
		}
	}
	if _, err := service.Exchange("203.0.113.211", generatedTokenForTest(t, 120)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("per-IP limit was lost after eviction redesign: %v", err)
	}
}

func TestRateLimitTableMaintenanceWorkIsBoundedAndAmortized(t *testing.T) {
	table := newRateLimitTable[int]()
	now := time.Unix(1_800_000_000, 0).UTC()
	const entries = 100_000
	for key := 0; key < entries; key++ {
		if work := table.increment(now, key, entries); work != 0 {
			t.Fatalf("initial insertion %d performed eviction work=%d", key, work)
		}
	}
	if work := table.expire(now, time.Hour); work != 1 {
		t.Fatalf("unexpired cleanup inspected %d entries, want one queue head", work)
	}
	if work := table.increment(now, entries, entries); work != 1 {
		t.Fatalf("capacity admission work=%d, want one oldest eviction", work)
	}
	if got := len(table.entries); got != entries {
		t.Fatalf("bounded table size=%d, want %d", got, entries)
	}
	if work := table.expire(now.Add(time.Hour), time.Hour); work != maxRateLimitExpiryStepsPerTable {
		t.Fatalf("single-request expired cleanup work=%d, want fixed budget %d", work, maxRateLimitExpiryStepsPerTable)
	}
	totalWork := maxRateLimitExpiryStepsPerTable
	for len(table.entries) > 0 {
		work := table.expire(now.Add(time.Hour), time.Hour)
		if work > maxRateLimitExpiryStepsPerTable {
			t.Fatalf("expired cleanup exceeded per-request budget: %d", work)
		}
		totalWork += work
	}
	if totalWork != entries {
		t.Fatalf("amortized cleanup work=%d, want one removal per retained entry=%d", totalWork, entries)
	}
}

type manualClock struct {
	now time.Time
}

func (clock *manualClock) Read() (time.Time, error)    { return clock.now, nil }
func (clock *manualClock) Advance(delta time.Duration) { clock.now = clock.now.Add(delta) }
