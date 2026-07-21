package setup

import (
	"encoding/base64"
	"errors"
	"math"
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

func TestRateLimitConfigurationRejectsUnsafeBounds(t *testing.T) {
	token := generatedTokenForTest(t, 129)
	valid := defaultTestRateLimit()
	cases := []RateLimitConfig{
		{Window: time.Minute, IPAttempts: 1, TokenAttempts: 1, MaxEntries: minimumRateLimitMemoryCells},
		func() RateLimitConfig { cfg := valid; cfg.GlobalWindow = 0; return cfg }(),
		func() RateLimitConfig { cfg := valid; cfg.GlobalWindow = 2 * time.Hour; return cfg }(),
		func() RateLimitConfig { cfg := valid; cfg.GlobalAttempts = 0; return cfg }(),
		func() RateLimitConfig { cfg := valid; cfg.MaxEntries = minimumRateLimitMemoryCells - 1; return cfg }(),
	}
	if uint64(^uint(0)) > math.MaxUint32 {
		tooMany := int(uint64(math.MaxUint32) + 1)
		for _, field := range []string{"ip", "token", "global"} {
			cfg := valid
			switch field {
			case "ip":
				cfg.IPAttempts = tooMany
			case "token":
				cfg.TokenAttempts = tooMany
			case "global":
				cfg.GlobalAttempts = tooMany
			}
			cases = append(cases, cfg)
		}
	}
	for index, rateLimit := range cases {
		_, err := NewAuthService(AuthConfig{
			Token: token, Version: 1, SessionTTL: 5 * time.Minute,
			Rand: &sequenceReader{}, Clock: fixedClock(time.Unix(1_800_000_000, 0)), RateLimit: rateLimit,
		})
		if !errors.Is(err, ErrInvalidConfiguration) {
			t.Errorf("case %d NewAuthService error = %v", index, err)
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
		RateLimit: testRateLimitConfig(time.Minute, 2, 3, 256),
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

func TestRateLimitCountsSuccessfulAttemptsAndBoundsMemory(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	clock := &manualClock{now: now}
	token := generatedTokenForTest(t, 16)
	config := testRateLimitConfig(time.Minute, 2, 10, 64)
	service, err := NewAuthService(AuthConfig{
		Token: token, Version: 1, SessionTTL: 5 * time.Minute, Rand: &sequenceReader{}, Clock: clock.Read,
		RateLimit: config,
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
	memoryCells := service.rateLimitMemoryCellsForTest()
	if memoryCells > config.MaxEntries {
		t.Fatalf("rate limiter exceeded memory budget: cells=%d budget=%d", memoryCells, config.MaxEntries)
	}
	if ipCells, tokenCells := service.rateLimitEntryCountsForTest(); ipCells > memoryCells || tokenCells > memoryCells {
		t.Fatalf("active sketch cells exceeded allocation: ip=%d token=%d total=%d", ipCells, tokenCells, memoryCells)
	}

	clock.Advance(time.Minute)
	if _, err := service.Exchange("192.0.2.72", generatedTokenForTest(t, 18)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired entries were not cleaned: %v", err)
	}
}

func TestRateLimitUniqueFingerprintFloodCannotEraseLimitedKeyAndRecoversByEpoch(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	clock := &manualClock{now: now}
	adminToken := generatedTokenForTest(t, 30)
	service, err := NewAuthService(AuthConfig{
		Token: adminToken, Version: 1, SessionTTL: 5 * time.Minute, Rand: &sequenceReader{}, Clock: clock.Read,
		RateLimit: testRateLimitConfig(time.Hour, 3, 1000, 128),
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	limitedIP := "203.0.113.199"
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := service.Exchange(limitedIP, generatedTokenForTest(t, byte(90+attempt))); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("limited-key seed %d error = %v", attempt, err)
		}
	}
	if _, err := service.Exchange(limitedIP, generatedTokenForTest(t, 99)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("limited key was not initially limited: %v", err)
	}
	for index := 1; index <= 400; index++ {
		candidate := generatedTokenForTest(t, byte(30+index))
		_, err := service.Exchange("198.51.100."+itoaForTest((index%90)+1), candidate)
		if !errors.Is(err, ErrInvalidToken) && !errors.Is(err, ErrRateLimited) {
			t.Fatalf("flood attempt %d error = %v", index, err)
		}
	}
	if _, err := service.Exchange(limitedIP, generatedTokenForTest(t, 100)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("unrelated flood erased limited-key count: %v", err)
	}
	clock.Advance(time.Hour)
	if _, err := service.Exchange("203.0.113.200", adminToken); err != nil {
		t.Fatalf("rate limiter did not recover after retained epochs expired: %v", err)
	}
}

func TestRateLimitExpiredCleanupIsAmortizedAndPreservesFingerprintLimits(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	clock := &manualClock{now: now}
	token := generatedTokenForTest(t, 80)
	service, err := NewAuthService(AuthConfig{
		Token: token, Version: 1, SessionTTL: 5 * time.Minute, Rand: &sequenceReader{}, Clock: clock.Read,
		RateLimit: testRateLimitConfig(time.Minute, 2, 1000, 512),
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

func TestRateLimitSketchHasFixedMemoryKeyedHashesAndConservativeCollisions(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	config := testRateLimitConfig(time.Minute, 1, 100, minimumRateLimitMemoryCells)
	service, err := NewAuthService(AuthConfig{
		Token: generatedTokenForTest(t, 121), Version: 1, SessionTTL: 5 * time.Minute,
		Rand: &sequenceReader{}, Clock: fixedClock(now), RateLimit: config,
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	if cells := service.rateLimitMemoryCellsForTest(); cells > config.MaxEntries {
		t.Fatalf("sketch allocated %d cells, budget=%d", cells, config.MaxEntries)
	}
	if _, err := service.Exchange("192.0.2.1", generatedTokenForTest(t, 122)); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("first collision seed error = %v", err)
	}
	if _, err := service.Exchange("192.0.2.2", generatedTokenForTest(t, 123)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("width-one collision undercounted instead of conservatively limiting: %v", err)
	}

	wideConfig := testRateLimitConfig(time.Minute, 100, 100, 1024)
	wideService, err := NewAuthService(AuthConfig{
		Token: generatedTokenForTest(t, 131), Version: 1, SessionTTL: 5 * time.Minute,
		Rand: &sequenceReader{}, Clock: fixedClock(now), RateLimit: wideConfig,
	})
	if err != nil {
		t.Fatalf("NewAuthService for keyed positions returned error: %v", err)
	}
	firstKey := wideService.rateLimitPositionsForTest("ip", []byte("192.0.2.1"))
	wideService.signingKey[0] ^= 0x80
	secondKey := wideService.rateLimitPositionsForTest("ip", []byte("192.0.2.1"))
	if firstKey == secondKey {
		t.Fatal("keyed rate-limit positions did not change with signing key")
	}
}

func TestRateLimitGlobalBudgetIsNonEvictableAndRecovers(t *testing.T) {
	now := time.Unix(1_800_000_000, 0).UTC()
	clock := &manualClock{now: now}
	token := generatedTokenForTest(t, 124)
	config := testRateLimitConfig(time.Minute, 100, 100, 1024)
	config.GlobalWindow = 10 * time.Second
	config.GlobalAttempts = 3
	service, err := NewAuthService(AuthConfig{
		Token: token, Version: 1, SessionTTL: 5 * time.Minute,
		Rand: &sequenceReader{}, Clock: clock.Read, RateLimit: config,
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := service.Exchange("198.51.100."+itoaForTest(attempt+1), generatedTokenForTest(t, byte(125+attempt))); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("global seed %d error = %v", attempt, err)
		}
	}
	if _, err := service.Exchange("198.51.100.4", token); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("global attempt budget did not limit total work: %v", err)
	}
	clock.Advance(10 * time.Second)
	if _, err := service.Exchange("198.51.100.4", token); err != nil {
		t.Fatalf("global attempt budget did not recover: %v", err)
	}
}

type manualClock struct {
	now time.Time
}

func (clock *manualClock) Read() (time.Time, error)    { return clock.now, nil }
func (clock *manualClock) Advance(delta time.Duration) { clock.now = clock.now.Add(delta) }
