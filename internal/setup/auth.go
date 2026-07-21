package setup

import (
	cryptorand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"math"
	"net/netip"
	"strings"
	"sync"
	"time"
)

const (
	setupTokenBytes       = 32
	setupTokenEncodedSize = 43
	rotationHandleBytes   = 32
	minSessionTTL         = time.Minute
	maxSessionTTL         = 24 * time.Hour
	maxTokenInputBytes    = 256
)

var (
	ErrInvalidToken         = errors.New("setup credentials are invalid")
	ErrInvalidSession       = errors.New("setup session is invalid")
	ErrRateLimited          = errors.New("setup authentication is temporarily rate limited")
	ErrCompleted            = errors.New("setup is already completed")
	ErrInvalidConfiguration = errors.New("setup authentication configuration is invalid")
	ErrEntropy              = errors.New("secure random generation failed")
	ErrClock                = errors.New("setup authentication clock failed")
	ErrRotationPending      = errors.New("setup token rotation is already pending")
	ErrInvalidRotation      = errors.New("setup token rotation is invalid")
	ErrVersionOverflow      = errors.New("setup token version cannot be incremented")
	ErrInvalidClientIP      = errors.New("setup client IP is invalid")
)

type Clock func() (time.Time, error)

type RateLimitConfig struct {
	Window        time.Duration
	IPAttempts    int
	TokenAttempts int
	MaxEntries    int
}

func DefaultSetupRateLimitConfig() RateLimitConfig {
	return RateLimitConfig{
		Window:        15 * time.Minute,
		IPAttempts:    10,
		TokenAttempts: 10,
		MaxEntries:    4096,
	}
}

type AuthConfig struct {
	Token      string
	Version    uint64
	Completed  bool
	SessionTTL time.Duration
	Rand       io.Reader
	Clock      Clock
	RateLimit  RateLimitConfig
}

type PreparedRotation struct {
	Token   string
	Version uint64
	handle  [rotationHandleBytes]byte
}

type rotationMaterial struct {
	handle     [rotationHandleBytes]byte
	tokenHash  [sha256.Size]byte
	signingKey [sha256.Size]byte
	version    uint64
}

type attemptWindow struct {
	started time.Time
	count   int
}

// setupRateLimiter counts every exchange attempt, including successful ones.
// Capacity exhaustion fails closed; expired fixed-window entries are removed
// before new entries are admitted.
type setupRateLimiter struct {
	config RateLimitConfig
	ip     map[string]attemptWindow
	token  map[[sha256.Size]byte]attemptWindow
}

type AuthService struct {
	mu sync.Mutex

	rand       io.Reader
	clock      Clock
	sessionTTL time.Duration
	limiter    setupRateLimiter
	lastNow    time.Time

	completed    bool
	version      uint64
	tokenHash    [sha256.Size]byte
	hasTokenHash bool
	signingKey   [sha256.Size]byte
	pending      *rotationMaterial
}

func GenerateSetupToken(random io.Reader) (string, error) {
	if random == nil {
		return "", ErrEntropy
	}
	raw := make([]byte, setupTokenBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		clear(raw)
		return "", ErrEntropy
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	clear(raw)
	if len(token) != setupTokenEncodedSize {
		return "", ErrEntropy
	}
	return token, nil
}

func NewAuthService(cfg AuthConfig) (*AuthService, error) {
	if cfg.Version == 0 || cfg.SessionTTL < minSessionTTL || cfg.SessionTTL > maxSessionTTL {
		return nil, ErrInvalidConfiguration
	}
	if cfg.Rand == nil {
		cfg.Rand = cryptorand.Reader
	}
	if cfg.Clock == nil {
		cfg.Clock = func() (time.Time, error) { return time.Now(), nil }
	}
	if cfg.RateLimit == (RateLimitConfig{}) {
		cfg.RateLimit = DefaultSetupRateLimitConfig()
	}
	if err := validateRateLimitConfig(cfg.RateLimit); err != nil {
		return nil, ErrInvalidConfiguration
	}

	service := &AuthService{
		rand: cfg.Rand, clock: cfg.Clock, sessionTTL: cfg.SessionTTL,
		completed: cfg.Completed, version: cfg.Version,
		limiter: setupRateLimiter{
			config: cfg.RateLimit,
			ip:     make(map[string]attemptWindow),
			token:  make(map[[sha256.Size]byte]attemptWindow),
		},
	}
	if cfg.Completed {
		return service, nil
	}

	decoded, ok := decodeSetupToken(cfg.Token)
	if !ok {
		return nil, ErrInvalidConfiguration
	}
	service.tokenHash = sha256.Sum256(decoded)
	service.hasTokenHash = true
	clear(decoded)
	if err := fillRandom(service.rand, service.signingKey[:]); err != nil {
		service.clearSecretsLocked()
		return nil, err
	}
	return service, nil
}

func (service *AuthService) Exchange(clientIP, token string) (string, error) {
	normalizedIP, err := NormalizeClientIP(clientIP)
	if err != nil {
		return "", err
	}
	candidate, formatOK := decodeSetupToken(token)
	fingerprint := tokenFingerprint(token, candidate, formatOK)
	if formatOK {
		defer clear(candidate)
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		return "", ErrCompleted
	}
	now, err := service.nowLocked()
	if err != nil {
		return "", err
	}
	if err := service.acceptTimeLocked(now); err != nil {
		return "", err
	}
	if !service.limiter.allow(now, normalizedIP, fingerprint) {
		return "", ErrRateLimited
	}
	candidateHash := sha256.Sum256(candidate)
	valid := subtle.ConstantTimeCompare(service.tokenHash[:], candidateHash[:]) == 1
	clear(candidateHash[:])
	if !formatOK || !service.hasTokenHash || !valid {
		return "", ErrInvalidToken
	}

	nonce := make([]byte, sessionNonceSize)
	if err := fillRandom(service.rand, nonce); err != nil {
		clear(nonce)
		return "", err
	}
	session, err := signSession(service.version, now, service.sessionTTL, nonce, service.signingKey[:])
	clear(nonce)
	if err != nil {
		return "", err
	}
	return session, nil
}

func (service *AuthService) VerifySession(session string) error {
	encoded, ok := decodeSession(session)
	if !ok {
		return ErrInvalidSession
	}
	defer clear(encoded)

	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		return ErrCompleted
	}
	now, err := service.nowLocked()
	if err != nil {
		return err
	}
	if err := service.acceptTimeLocked(now); err != nil {
		return err
	}
	if !verifySignedSession(encoded, service.version, now, service.signingKey[:]) {
		return ErrInvalidSession
	}
	return nil
}

func (service *AuthService) PrepareRotation() (PreparedRotation, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		return PreparedRotation{}, ErrCompleted
	}
	if service.pending != nil {
		return PreparedRotation{}, ErrRotationPending
	}
	if service.version == math.MaxUint64 {
		return PreparedRotation{}, ErrVersionOverflow
	}

	token, err := GenerateSetupToken(service.rand)
	if err != nil {
		return PreparedRotation{}, err
	}
	decoded, ok := decodeSetupToken(token)
	if !ok {
		return PreparedRotation{}, ErrEntropy
	}
	material := &rotationMaterial{version: service.version + 1}
	material.tokenHash = sha256.Sum256(decoded)
	clear(decoded)
	if err := fillRandom(service.rand, material.signingKey[:]); err != nil {
		material.clear()
		return PreparedRotation{}, err
	}
	if err := fillRandom(service.rand, material.handle[:]); err != nil {
		material.clear()
		return PreparedRotation{}, err
	}
	service.pending = material
	return PreparedRotation{Token: token, Version: material.version, handle: material.handle}, nil
}

// CommitRotation must be called only after Token and Version have been
// durably written together. Until commit, the old token and sessions remain
// valid. A process restart after persistence reconstructs this service from
// the newly written runtime snapshot.
func (service *AuthService) CommitRotation(prepared PreparedRotation) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		return ErrCompleted
	}
	if !service.matchesPendingLocked(prepared) {
		return ErrInvalidRotation
	}

	service.tokenHash = service.pending.tokenHash
	service.signingKey = service.pending.signingKey
	service.version = service.pending.version
	service.hasTokenHash = true
	service.pending.clear()
	service.pending = nil
	service.limiter.clear()
	return nil
}

func (service *AuthService) AbortRotation(prepared PreparedRotation) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		return ErrCompleted
	}
	if service.pending == nil || subtle.ConstantTimeCompare(service.pending.handle[:], prepared.handle[:]) != 1 {
		return ErrInvalidRotation
	}
	service.pending.clear()
	service.pending = nil
	return nil
}

func (service *AuthService) Complete() error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		return nil
	}
	service.completed = true
	service.clearSecretsLocked()
	return nil
}

func (service *AuthService) Version() uint64 {
	service.mu.Lock()
	defer service.mu.Unlock()
	return service.version
}

func NormalizeClientIP(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidClientIP
	}
	if addressPort, err := netip.ParseAddrPort(value); err == nil {
		return addressPort.Addr().Unmap().WithZone("").String(), nil
	}
	address, err := netip.ParseAddr(value)
	if err != nil {
		return "", ErrInvalidClientIP
	}
	return address.Unmap().WithZone("").String(), nil
}

// nowLocked serializes access to the injected clock as part of AuthService's
// thread-safety contract. Callers must hold service.mu.
func (service *AuthService) nowLocked() (time.Time, error) {
	now, err := service.clock()
	if err != nil || now.IsZero() || now.Year() < 1 || now.Year() > 9999 {
		return time.Time{}, ErrClock
	}
	now = now.UTC()
	expires := now.Add(service.sessionTTL)
	if now.Unix() < 0 || !expires.After(now) || expires.Year() > 9999 {
		return time.Time{}, ErrClock
	}
	return now, nil
}

func (service *AuthService) acceptTimeLocked(now time.Time) error {
	if !service.lastNow.IsZero() && now.Before(service.lastNow) {
		return ErrClock
	}
	service.lastNow = now
	return nil
}

func (service *AuthService) matchesPendingLocked(prepared PreparedRotation) bool {
	if service.pending == nil || prepared.Version != service.pending.version {
		return false
	}
	if subtle.ConstantTimeCompare(service.pending.handle[:], prepared.handle[:]) != 1 {
		return false
	}
	decoded, ok := decodeSetupToken(prepared.Token)
	if !ok {
		return false
	}
	hash := sha256.Sum256(decoded)
	clear(decoded)
	matched := subtle.ConstantTimeCompare(service.pending.tokenHash[:], hash[:]) == 1
	clear(hash[:])
	return matched
}

func (service *AuthService) clearSecretsLocked() {
	clear(service.tokenHash[:])
	clear(service.signingKey[:])
	service.hasTokenHash = false
	if service.pending != nil {
		service.pending.clear()
		service.pending = nil
	}
	service.limiter.clear()
}

func (material *rotationMaterial) clear() {
	clear(material.handle[:])
	clear(material.tokenHash[:])
	clear(material.signingKey[:])
	material.version = 0
}

func decodeSetupToken(token string) ([]byte, bool) {
	if len(token) != setupTokenEncodedSize || strings.Contains(token, "=") {
		return nil, false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(token)
	if err != nil || len(decoded) != setupTokenBytes || base64.RawURLEncoding.EncodeToString(decoded) != token {
		clear(decoded)
		return nil, false
	}
	return decoded, true
}

func tokenFingerprint(raw string, decoded []byte, formatOK bool) [sha256.Size]byte {
	if formatOK {
		return sha256.Sum256(decoded)
	}
	if len(raw) > maxTokenInputBytes {
		return sha256.Sum256([]byte("oversized-invalid-setup-token"))
	}
	return sha256.Sum256([]byte(raw))
}

func fillRandom(random io.Reader, target []byte) error {
	if random == nil {
		return ErrEntropy
	}
	if _, err := io.ReadFull(random, target); err != nil {
		clear(target)
		return ErrEntropy
	}
	return nil
}

func validateRateLimitConfig(cfg RateLimitConfig) error {
	if cfg.Window < time.Second || cfg.Window > 24*time.Hour || cfg.IPAttempts <= 0 || cfg.TokenAttempts <= 0 || cfg.MaxEntries <= 0 || cfg.MaxEntries > 1_000_000 {
		return fmt.Errorf("invalid setup rate limit")
	}
	return nil
}

func (limiter *setupRateLimiter) allow(now time.Time, ip string, fingerprint [sha256.Size]byte) bool {
	limiter.cleanup(now)
	ipWindow, ipExists := limiter.ip[ip]
	tokenWindow, tokenExists := limiter.token[fingerprint]
	if (ipExists && ipWindow.count >= limiter.config.IPAttempts) || (tokenExists && tokenWindow.count >= limiter.config.TokenAttempts) {
		return false
	}
	if (!ipExists && len(limiter.ip) >= limiter.config.MaxEntries) || (!tokenExists && len(limiter.token) >= limiter.config.MaxEntries) {
		return false
	}
	if !ipExists {
		ipWindow = attemptWindow{started: now}
	}
	if !tokenExists {
		tokenWindow = attemptWindow{started: now}
	}
	ipWindow.count++
	tokenWindow.count++
	limiter.ip[ip] = ipWindow
	limiter.token[fingerprint] = tokenWindow
	return true
}

func (limiter *setupRateLimiter) cleanup(now time.Time) {
	for key, window := range limiter.ip {
		if !now.Before(window.started.Add(limiter.config.Window)) {
			delete(limiter.ip, key)
		}
	}
	for key, window := range limiter.token {
		if !now.Before(window.started.Add(limiter.config.Window)) {
			delete(limiter.token, key)
		}
	}
}

func (limiter *setupRateLimiter) clear() {
	clear(limiter.ip)
	clear(limiter.token)
}
