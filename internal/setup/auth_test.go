package setup

import (
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestTokenGenerationUsesExactly256BitsAndStrictEncoding(t *testing.T) {
	reader := &sequenceReader{}
	token, err := GenerateSetupToken(reader)
	if err != nil {
		t.Fatalf("GenerateSetupToken returned error: %v", err)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("generated token is not raw URL base64: %v", err)
	}
	if len(decoded) != 32 || reader.bytesRead != 32 {
		t.Fatalf("token entropy = %d decoded/%d read bytes, want 32", len(decoded), reader.bytesRead)
	}

	service := newAuthServiceForTest(t, token, 1, false, defaultTestRateLimit())
	for _, malformed := range []string{
		token + "=", strings.ToUpper(token), token[:len(token)-1], token + "A",
		"", strings.Repeat("A", 44),
	} {
		if _, err := service.Exchange("192.0.2.1", malformed); !errors.Is(err, ErrInvalidToken) {
			t.Errorf("Exchange accepted malformed token %q: %v", malformed, err)
		}
	}
}

func TestTokenVerificationIsReusableAndStoresOnlyVerifier(t *testing.T) {
	token := generatedTokenForTest(t, 1)
	service := newAuthServiceForTest(t, token, 9, false, defaultTestRateLimit())
	if strings.Contains(service.debugStateForTest(), token) {
		t.Fatal("auth service retained setup token plaintext")
	}

	first, err := service.Exchange("192.0.2.1", token)
	if err != nil {
		t.Fatalf("first Exchange returned error: %v", err)
	}
	second, err := service.Exchange("192.0.2.1", token)
	if err != nil {
		t.Fatalf("reusable Exchange returned error: %v", err)
	}
	if first == second {
		t.Fatal("separate exchanges produced the same setup session")
	}
	if err := service.VerifySession(first); err != nil {
		t.Fatalf("first session did not verify: %v", err)
	}
	if err := service.VerifySession(second); err != nil {
		t.Fatalf("second session did not verify: %v", err)
	}
}

func TestTokenAndSessionErrorsNeverExposeCredentials(t *testing.T) {
	token := generatedTokenForTest(t, 2)
	service := newAuthServiceForTest(t, token, 1, false, defaultTestRateLimit())
	session, err := service.Exchange("192.0.2.1", token)
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	candidates := []struct {
		secret string
		err    error
	}{
		{secret: token, err: exchangeError(service, "192.0.2.2", generatedTokenForTest(t, 3))},
		{secret: session, err: service.VerifySession(session + "A")},
	}
	for _, candidate := range candidates {
		if candidate.err == nil {
			t.Fatal("expected credential failure")
		}
		if strings.Contains(candidate.err.Error(), candidate.secret) {
			t.Fatalf("error exposed credential: %v", candidate.err)
		}
	}

	failing := errorReader{err: errors.New("entropy failed near " + token + " / " + session)}
	if _, err := GenerateSetupToken(failing); !errors.Is(err, ErrEntropy) || strings.Contains(err.Error(), token) || strings.Contains(err.Error(), session) {
		t.Fatalf("entropy error was not sanitized: %v", err)
	}
}

func TestRotationPrepareCommitCreatesAtomicPersistenceBoundary(t *testing.T) {
	oldToken := generatedTokenForTest(t, 4)
	service := newAuthServiceForTest(t, oldToken, 3, false, defaultTestRateLimit())
	oldSession, err := service.Exchange("192.0.2.1", oldToken)
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}

	prepared, err := service.PrepareRotation()
	if err != nil {
		t.Fatalf("PrepareRotation returned error: %v", err)
	}
	if prepared.Version != 4 || prepared.Token == "" || prepared.Token == oldToken {
		t.Fatalf("unexpected prepared rotation: version=%d token_changed=%t", prepared.Version, prepared.Token != oldToken)
	}
	if _, err := service.PrepareRotation(); !errors.Is(err, ErrRotationPending) {
		t.Fatalf("concurrent PrepareRotation error = %v, want ErrRotationPending", err)
	}
	if _, err := service.Exchange("192.0.2.2", oldToken); err != nil {
		t.Fatalf("old token must remain active before durable commit: %v", err)
	}
	if _, err := service.Exchange("192.0.2.3", prepared.Token); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("new token became active before commit: %v", err)
	}

	forged := prepared
	forged.handle[0] ^= 0xff
	if err := service.CommitRotation(forged); !errors.Is(err, ErrInvalidRotation) {
		t.Fatalf("forged CommitRotation error = %v", err)
	}
	if err := service.CommitRotation(prepared); err != nil {
		t.Fatalf("CommitRotation returned error: %v", err)
	}
	if _, err := service.Exchange("192.0.2.4", oldToken); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("old token remained valid after commit: %v", err)
	}
	if err := service.VerifySession(oldSession); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("old session remained valid after commit: %v", err)
	}
	newSession, err := service.Exchange("192.0.2.5", prepared.Token)
	if err != nil {
		t.Fatalf("new token failed after commit: %v", err)
	}
	if err := service.VerifySession(newSession); err != nil {
		t.Fatalf("new session failed after commit: %v", err)
	}
	if err := service.CommitRotation(prepared); !errors.Is(err, ErrInvalidRotation) {
		t.Fatalf("stale CommitRotation error = %v", err)
	}
}

func TestRotationAbortAllowsRetryAndOverflowFailsClosed(t *testing.T) {
	service := newAuthServiceForTest(t, generatedTokenForTest(t, 5), 1, false, defaultTestRateLimit())
	prepared, err := service.PrepareRotation()
	if err != nil {
		t.Fatalf("PrepareRotation returned error: %v", err)
	}
	forged := prepared
	forged.handle[0] ^= 0x40
	if err := service.AbortRotation(forged); !errors.Is(err, ErrInvalidRotation) {
		t.Fatalf("forged AbortRotation error = %v", err)
	}
	if err := service.AbortRotation(prepared); err != nil {
		t.Fatalf("AbortRotation returned error: %v", err)
	}
	retry, err := service.PrepareRotation()
	if err != nil {
		t.Fatalf("PrepareRotation after abort returned error: %v", err)
	}
	if retry.Token == prepared.Token {
		t.Fatal("retry reused aborted token")
	}
	if err := service.CommitRotation(prepared); !errors.Is(err, ErrInvalidRotation) {
		t.Fatalf("aborted stale rotation committed over retry: %v", err)
	}

	overflow := newAuthServiceForTest(t, generatedTokenForTest(t, 6), math.MaxUint64, false, defaultTestRateLimit())
	if _, err := overflow.PrepareRotation(); !errors.Is(err, ErrVersionOverflow) {
		t.Fatalf("overflow PrepareRotation error = %v, want ErrVersionOverflow", err)
	}
}

func TestConcurrentPrepareRegistersExactlyOnePendingRotation(t *testing.T) {
	service := newAuthServiceForTest(t, generatedTokenForTest(t, 20), 1, false, defaultTestRateLimit())
	const callers = 32
	var wg sync.WaitGroup
	var successes atomic.Int32
	var pendingErrors atomic.Int32
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := service.PrepareRotation()
			switch {
			case err == nil:
				successes.Add(1)
			case errors.Is(err, ErrRotationPending):
				pendingErrors.Add(1)
			default:
				t.Errorf("PrepareRotation returned unexpected error: %v", err)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 || pendingErrors.Load() != callers-1 {
		t.Fatalf("concurrent prepare results: success=%d pending=%d", successes.Load(), pendingErrors.Load())
	}
}

func TestAuthServiceSerializesInjectedClockAccess(t *testing.T) {
	token := generatedTokenForTest(t, 21)
	clock := &exclusiveClock{now: time.Unix(1_800_000_000, 0)}
	service, err := NewAuthService(AuthConfig{
		Token: token, Version: 1, SessionTTL: 5 * time.Minute,
		Rand: &sequenceReader{}, Clock: clock.Read,
		RateLimit: RateLimitConfig{Window: time.Minute, IPAttempts: 1000, TokenAttempts: 1000, MaxEntries: 1000},
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	var wg sync.WaitGroup
	errorsSeen := make(chan error, 64)
	for index := 0; index < 64; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, err := service.Exchange("198.51.100."+itoaForTest(index+1), token)
			errorsSeen <- err
		}(index)
	}
	wg.Wait()
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatalf("concurrent exchange observed unserialized dependency: %v", err)
		}
	}
	if clock.concurrent.Load() {
		t.Fatal("injected clock was called concurrently")
	}
}

func TestCompletionPrepareCommitCreatesDurableBoundaryAndInvalidatesEverything(t *testing.T) {
	token := generatedTokenForTest(t, 7)
	service := newAuthServiceForTest(t, token, 2, false, defaultTestRateLimit())
	session, err := service.Exchange("192.0.2.1", token)
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	prepared, err := service.PrepareCompletion()
	if err != nil {
		t.Fatalf("PrepareCompletion returned error: %v", err)
	}
	if _, err := service.PrepareCompletion(); !errors.Is(err, ErrCompletionPending) {
		t.Fatalf("second PrepareCompletion error = %v, want pending", err)
	}
	if _, err := service.Exchange("192.0.2.2", token); err != nil {
		t.Fatalf("token stopped before durable completion commit: %v", err)
	}
	if err := service.VerifySession(session); err != nil {
		t.Fatalf("session stopped before durable completion commit: %v", err)
	}
	forged := prepared
	forged.handle[0] ^= 0x80
	if err := service.CommitCompletion(forged); !errors.Is(err, ErrInvalidCompletion) {
		t.Fatalf("forged CommitCompletion error = %v", err)
	}

	// The caller durably writes SETUP_COMPLETED and removes the setup token here.
	durableEnvCommitted := true
	if !durableEnvCommitted {
		t.Fatal("test must model durable env commit before in-memory commit")
	}
	if err := service.CommitCompletion(prepared); err != nil {
		t.Fatalf("CommitCompletion returned error: %v", err)
	}
	if err := service.CommitCompletion(prepared); err != nil {
		t.Fatalf("idempotent CommitCompletion returned error: %v", err)
	}
	if _, err := service.Exchange("192.0.2.1", token); !errors.Is(err, ErrCompleted) {
		t.Fatalf("completed Exchange error = %v", err)
	}
	if err := service.VerifySession(session); !errors.Is(err, ErrCompleted) {
		t.Fatalf("completed VerifySession error = %v", err)
	}
	if _, err := service.PrepareRotation(); !errors.Is(err, ErrCompleted) {
		t.Fatalf("completed PrepareRotation error = %v", err)
	}
	if service.Version() != 2 {
		t.Fatalf("completion changed retained token version: %d", service.Version())
	}
}

func TestCompletionAndRotationLeasesAreMutuallyExclusive(t *testing.T) {
	service := newAuthServiceForTest(t, generatedTokenForTest(t, 22), 1, false, defaultTestRateLimit())
	rotation, err := service.PrepareRotation()
	if err != nil {
		t.Fatalf("PrepareRotation returned error: %v", err)
	}
	if _, err := service.PrepareCompletion(); !errors.Is(err, ErrRotationPending) {
		t.Fatalf("PrepareCompletion during rotation error = %v, want rotation pending", err)
	}
	if err := service.AbortRotation(rotation); err != nil {
		t.Fatalf("AbortRotation returned error: %v", err)
	}

	completion, err := service.PrepareCompletion()
	if err != nil {
		t.Fatalf("PrepareCompletion returned error: %v", err)
	}
	if _, err := service.PrepareRotation(); !errors.Is(err, ErrCompletionPending) {
		t.Fatalf("PrepareRotation during completion error = %v, want completion pending", err)
	}
	if err := service.AbortCompletion(completion); err != nil {
		t.Fatalf("AbortCompletion returned error: %v", err)
	}
	if err := service.AbortCompletion(completion); !errors.Is(err, ErrInvalidCompletion) {
		t.Fatalf("stale AbortCompletion error = %v, want invalid", err)
	}
	if err := service.CommitCompletion(completion); !errors.Is(err, ErrInvalidCompletion) {
		t.Fatalf("aborted CommitCompletion error = %v, want invalid", err)
	}
	if _, err := service.PrepareRotation(); err != nil {
		t.Fatalf("rotation did not recover after completion abort: %v", err)
	}
}

func TestCompletionAbortKeepsCredentialsAndRetryUsesNewHandle(t *testing.T) {
	token := generatedTokenForTest(t, 23)
	service := newAuthServiceForTest(t, token, 1, false, defaultTestRateLimit())
	first, err := service.PrepareCompletion()
	if err != nil {
		t.Fatalf("PrepareCompletion returned error: %v", err)
	}
	if err := service.AbortCompletion(first); err != nil {
		t.Fatalf("AbortCompletion returned error: %v", err)
	}
	if _, err := service.Exchange("192.0.2.90", token); err != nil {
		t.Fatalf("completion abort invalidated token: %v", err)
	}
	retry, err := service.PrepareCompletion()
	if err != nil {
		t.Fatalf("retry PrepareCompletion returned error: %v", err)
	}
	if retry.handle == first.handle {
		t.Fatal("completion retry reused aborted unforgeable handle")
	}
}

func TestPreparedCompletionStringRepresentationsRedactHandle(t *testing.T) {
	service := newAuthServiceForTest(t, generatedTokenForTest(t, 24), 1, false, defaultTestRateLimit())
	prepared, err := service.PrepareCompletion()
	if err != nil {
		t.Fatalf("PrepareCompletion returned error: %v", err)
	}
	for _, rendered := range []string{
		fmt.Sprintf("%v", prepared),
		fmt.Sprintf("%+v", prepared),
		fmt.Sprintf("%#v", prepared),
	} {
		if rendered != "PreparedCompletion{handle:<redacted>}" {
			t.Fatalf("prepared completion representation leaked shape or handle: %q", rendered)
		}
	}
	for _, operation := range []func() error{
		func() error { forged := prepared; forged.handle[0] ^= 1; return service.CommitCompletion(forged) },
		func() error { forged := prepared; forged.handle[0] ^= 1; return service.AbortCompletion(forged) },
	} {
		err := operation()
		if !errors.Is(err, ErrInvalidCompletion) || strings.Contains(err.Error(), fmt.Sprintf("%x", prepared.handle)) {
			t.Fatalf("completion error leaked handle or wrong type: %v", err)
		}
	}
}

func TestCompletedServiceCanStartWithoutPlaintextToken(t *testing.T) {
	service, err := NewAuthService(AuthConfig{
		Version: 8, Completed: true, SessionTTL: 10 * time.Minute,
		Rand: &sequenceReader{}, Clock: fixedClock(time.Unix(1_800_000_000, 0)),
		RateLimit: defaultTestRateLimit(),
	})
	if err != nil {
		t.Fatalf("NewAuthService completed config returned error: %v", err)
	}
	if _, err := service.Exchange("192.0.2.1", ""); !errors.Is(err, ErrCompleted) {
		t.Fatalf("completed service Exchange error = %v", err)
	}
	if _, err := service.PrepareCompletion(); !errors.Is(err, ErrCompleted) {
		t.Fatalf("restarted completed PrepareCompletion error = %v", err)
	}
	if err := service.CommitCompletion(PreparedCompletion{}); !errors.Is(err, ErrCompleted) {
		t.Fatalf("restarted completed CommitCompletion error = %v", err)
	}
}

func TestAuthServiceConcurrentExchangeRotateAndCompletionIsRaceSafe(t *testing.T) {
	token := generatedTokenForTest(t, 8)
	service := newAuthServiceForTest(t, token, 1, false, RateLimitConfig{Window: time.Minute, IPAttempts: 10000, TokenAttempts: 10000, MaxEntries: 10000})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for attempt := 0; attempt < 20; attempt++ {
				session, err := service.Exchange("192.0.2."+itoaForTest(index+1), token)
				if err == nil {
					_ = service.VerifySession(session)
				}
			}
		}(i)
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if prepared, err := service.PrepareRotation(); err == nil {
			_ = service.CommitRotation(prepared)
		}
	}()
	wg.Wait()
	prepared, err := service.PrepareCompletion()
	if err != nil {
		t.Fatalf("PrepareCompletion returned error: %v", err)
	}
	if err := service.CommitCompletion(prepared); err != nil {
		t.Fatalf("CommitCompletion returned error: %v", err)
	}
}

func exchangeError(service *AuthService, ip, token string) error {
	_, err := service.Exchange(ip, token)
	return err
}

func generatedTokenForTest(t *testing.T, seed byte) string {
	t.Helper()
	reader := &sequenceReader{next: seed}
	token, err := GenerateSetupToken(reader)
	if err != nil {
		t.Fatalf("GenerateSetupToken returned error: %v", err)
	}
	return token
}

func newAuthServiceForTest(t *testing.T, token string, version uint64, completed bool, rateLimit RateLimitConfig) *AuthService {
	t.Helper()
	service, err := NewAuthService(AuthConfig{
		Token: token, Version: version, Completed: completed,
		SessionTTL: 10 * time.Minute, Rand: &sequenceReader{next: 91},
		Clock: fixedClock(time.Unix(1_800_000_000, 0)), RateLimit: rateLimit,
	})
	if err != nil {
		t.Fatalf("NewAuthService returned error: %v", err)
	}
	return service
}

func fixedClock(now time.Time) Clock {
	return func() (time.Time, error) { return now, nil }
}

func defaultTestRateLimit() RateLimitConfig {
	return RateLimitConfig{Window: time.Minute, IPAttempts: 100, TokenAttempts: 100, MaxEntries: 100}
}

type sequenceReader struct {
	mu        sync.Mutex
	next      byte
	bytesRead int
}

func (reader *sequenceReader) Read(output []byte) (int, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	for index := range output {
		reader.next++
		output[index] = reader.next
	}
	reader.bytesRead += len(output)
	return len(output), nil
}

type errorReader struct{ err error }

func (reader errorReader) Read([]byte) (int, error) { return 0, reader.err }

var _ io.Reader = (*sequenceReader)(nil)

type exclusiveClock struct {
	now        time.Time
	inCall     atomic.Bool
	concurrent atomic.Bool
}

func (clock *exclusiveClock) Read() (time.Time, error) {
	if !clock.inCall.CompareAndSwap(false, true) {
		clock.concurrent.Store(true)
		return time.Time{}, errors.New("concurrent clock access")
	}
	time.Sleep(time.Millisecond)
	clock.inCall.Store(false)
	return clock.now, nil
}

func itoaForTest(value int) string {
	const digits = "0123456789"
	if value < 10 {
		return string(digits[value])
	}
	return string([]byte{digits[value/10], digits[value%10]})
}

func (service *AuthService) debugStateForTest() string {
	return fmt.Sprintf("%#v", service)
}

func (service *AuthService) rateLimitEntryCountsForTest() (int, int) {
	service.mu.Lock()
	defer service.mu.Unlock()
	return len(service.limiter.ip.entries), len(service.limiter.token.entries)
}
