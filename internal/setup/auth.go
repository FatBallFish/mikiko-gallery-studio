package setup

import (
	"container/list"
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
	setupTokenBytes                 = 32
	setupTokenEncodedSize           = 43
	rotationHandleBytes             = 32
	minSessionTTL                   = time.Minute
	maxSessionTTL                   = 24 * time.Hour
	maxTokenInputBytes              = 256
	maxRateLimitExpiryStepsPerTable = 8
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
	ErrCompletionPending    = errors.New("setup completion is already pending")
	ErrInvalidCompletion    = errors.New("setup completion is invalid")
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

type PreparedCompletion struct {
	handle [rotationHandleBytes]byte
}

func (PreparedCompletion) String() string {
	return "PreparedCompletion{handle:<redacted>}"
}

func (PreparedCompletion) GoString() string {
	return "PreparedCompletion{handle:<redacted>}"
}

type completionMaterial struct {
	handle [rotationHandleBytes]byte
}

type attemptWindow struct {
	started time.Time
	count   int
}

type rateLimitEntry[K comparable] struct {
	key    K
	window attemptWindow
}

type rateLimitTable[K comparable] struct {
	entries map[K]*list.Element
	order   list.List
}

// setupRateLimiter counts every exchange attempt, including successful ones.
// Fixed-window insertion order is expiry order, so cleanup and oldest-entry
// eviction are O(1) amortized instead of scanning the full maps per request.
type setupRateLimiter struct {
	config RateLimitConfig
	ip     rateLimitTable[string]
	token  rateLimitTable[[sha256.Size]byte]
}

type AuthService struct {
	mu sync.Mutex

	rand       io.Reader
	clock      Clock
	sessionTTL time.Duration
	limiter    setupRateLimiter
	lastNow    time.Time

	completed          bool
	version            uint64
	tokenHash          [sha256.Size]byte
	hasTokenHash       bool
	signingKey         [sha256.Size]byte
	pendingRotation    *rotationMaterial
	pendingCompletion  *completionMaterial
	completedHandle    [rotationHandleBytes]byte
	hasCompletedHandle bool
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
			ip:     newRateLimitTable[string](),
			token:  newRateLimitTable[[sha256.Size]byte](),
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
	if service.pendingCompletion != nil {
		return PreparedRotation{}, ErrCompletionPending
	}
	if service.pendingRotation != nil {
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
	service.pendingRotation = material
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

	service.tokenHash = service.pendingRotation.tokenHash
	service.signingKey = service.pendingRotation.signingKey
	service.version = service.pendingRotation.version
	service.hasTokenHash = true
	service.pendingRotation.clear()
	service.pendingRotation = nil
	service.limiter.clear()
	return nil
}

func (service *AuthService) AbortRotation(prepared PreparedRotation) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		return ErrCompleted
	}
	if service.pendingRotation == nil || subtle.ConstantTimeCompare(service.pendingRotation.handle[:], prepared.handle[:]) != 1 {
		return ErrInvalidRotation
	}
	service.pendingRotation.clear()
	service.pendingRotation = nil
	return nil
}

// PrepareCompletion acquires a process-local completion lease. The caller must
// durably commit the completed runtime env before CommitCompletion. Task 13 is
// responsible for deployctl's cross-process lock and compare-and-swap boundary.
func (service *AuthService) PrepareCompletion() (PreparedCompletion, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		return PreparedCompletion{}, ErrCompleted
	}
	if service.pendingRotation != nil {
		return PreparedCompletion{}, ErrRotationPending
	}
	if service.pendingCompletion != nil {
		return PreparedCompletion{}, ErrCompletionPending
	}
	material := &completionMaterial{}
	if err := fillRandom(service.rand, material.handle[:]); err != nil {
		material.clear()
		return PreparedCompletion{}, err
	}
	service.pendingCompletion = material
	return PreparedCompletion{handle: material.handle}, nil
}

func (service *AuthService) CommitCompletion(prepared PreparedCompletion) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		if service.hasCompletedHandle && subtle.ConstantTimeCompare(service.completedHandle[:], prepared.handle[:]) == 1 {
			return nil
		}
		if service.hasCompletedHandle {
			return ErrInvalidCompletion
		}
		return ErrCompleted
	}
	if !service.matchesPendingCompletionLocked(prepared) {
		return ErrInvalidCompletion
	}
	service.completedHandle = service.pendingCompletion.handle
	service.hasCompletedHandle = true
	service.pendingCompletion.clear()
	service.pendingCompletion = nil
	service.completed = true
	service.clearSecretsLocked()
	return nil
}

func (service *AuthService) AbortCompletion(prepared PreparedCompletion) error {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.completed {
		return ErrCompleted
	}
	if !service.matchesPendingCompletionLocked(prepared) {
		return ErrInvalidCompletion
	}
	service.pendingCompletion.clear()
	service.pendingCompletion = nil
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
	if service.pendingRotation == nil || prepared.Version != service.pendingRotation.version {
		return false
	}
	if subtle.ConstantTimeCompare(service.pendingRotation.handle[:], prepared.handle[:]) != 1 {
		return false
	}
	decoded, ok := decodeSetupToken(prepared.Token)
	if !ok {
		return false
	}
	hash := sha256.Sum256(decoded)
	clear(decoded)
	matched := subtle.ConstantTimeCompare(service.pendingRotation.tokenHash[:], hash[:]) == 1
	clear(hash[:])
	return matched
}

func (service *AuthService) matchesPendingCompletionLocked(prepared PreparedCompletion) bool {
	return service.pendingCompletion != nil && subtle.ConstantTimeCompare(service.pendingCompletion.handle[:], prepared.handle[:]) == 1
}

func (service *AuthService) clearSecretsLocked() {
	clear(service.tokenHash[:])
	clear(service.signingKey[:])
	service.hasTokenHash = false
	if service.pendingRotation != nil {
		service.pendingRotation.clear()
		service.pendingRotation = nil
	}
	if service.pendingCompletion != nil {
		service.pendingCompletion.clear()
		service.pendingCompletion = nil
	}
	service.limiter.clear()
}

func (material *rotationMaterial) clear() {
	clear(material.handle[:])
	clear(material.tokenHash[:])
	clear(material.signingKey[:])
	material.version = 0
}

func (material *completionMaterial) clear() {
	clear(material.handle[:])
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
	limiter.ip.expire(now, limiter.config.Window)
	limiter.token.expire(now, limiter.config.Window)
	ipWindow, ipExists := limiter.ip.lookup(now, limiter.config.Window, ip)
	tokenWindow, tokenExists := limiter.token.lookup(now, limiter.config.Window, fingerprint)
	if (ipExists && ipWindow.count >= limiter.config.IPAttempts) || (tokenExists && tokenWindow.count >= limiter.config.TokenAttempts) {
		return false
	}
	limiter.ip.increment(now, ip, limiter.config.MaxEntries)
	limiter.token.increment(now, fingerprint, limiter.config.MaxEntries)
	return true
}

func newRateLimitTable[K comparable]() rateLimitTable[K] {
	return rateLimitTable[K]{entries: make(map[K]*list.Element)}
}

func (limiter *setupRateLimiter) clear() {
	limiter.ip.clear()
	limiter.token.clear()
}

func (table *rateLimitTable[K]) lookup(now time.Time, window time.Duration, key K) (attemptWindow, bool) {
	element, exists := table.entries[key]
	if !exists {
		return attemptWindow{}, false
	}
	entry := element.Value.(rateLimitEntry[K])
	if !now.Before(entry.window.started.Add(window)) {
		table.removeElement(element)
		return attemptWindow{}, false
	}
	return entry.window, true
}

func (table *rateLimitTable[K]) increment(now time.Time, key K, maxEntries int) int {
	if element, exists := table.entries[key]; exists {
		entry := element.Value.(rateLimitEntry[K])
		entry.window.count++
		element.Value = entry
		return 0
	}
	work := 0
	if len(table.entries) >= maxEntries {
		table.removeOldest()
		work++
	}
	element := table.order.PushBack(rateLimitEntry[K]{key: key, window: attemptWindow{started: now, count: 1}})
	table.entries[key] = element
	return work
}

func (table *rateLimitTable[K]) expire(now time.Time, window time.Duration) int {
	work := 0
	for element := table.order.Front(); element != nil && work < maxRateLimitExpiryStepsPerTable; element = table.order.Front() {
		work++
		entry := element.Value.(rateLimitEntry[K])
		if now.Before(entry.window.started.Add(window)) {
			break
		}
		table.removeElement(element)
	}
	return work
}

func (table *rateLimitTable[K]) removeOldest() {
	element := table.order.Front()
	if element == nil {
		return
	}
	table.removeElement(element)
}

func (table *rateLimitTable[K]) removeElement(element *list.Element) {
	entry := element.Value.(rateLimitEntry[K])
	delete(table.entries, entry.key)
	table.order.Remove(element)
}

func (table *rateLimitTable[K]) clear() {
	clear(table.entries)
	table.order.Init()
}
