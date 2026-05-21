package apikey

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type CreateRequest struct {
	UserID           int64
	Name             string
	GroupCode        string
	Secret           string
	TotalQuotaPoints *string
	DailyQuotaPoints *string
	RPMLimit         *int
	ExpiresAt        *time.Time
}

type CreateResult struct {
	Key    domainapikey.APIKey
	Secret string
}

type Service struct {
	store                *MemoryStore
	usage                UsageStore
	signingEncryptionKey string
	rpmMu                sync.Mutex
	rpmWindows           map[int64][]time.Time
}

type UsageStore interface {
	APIKeyUsage(ctx context.Context, apiKeyID int64, since *time.Time) (string, error)
}

func NewService(store Store) *Service {
	return NewServiceWithSigningSecretKey(store, "")
}

func NewServiceWithSigningSecretKey(store Store, signingEncryptionKey string) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	signingEncryptionKey = defaultSigningEncryptionKey(signingEncryptionKey)
	memory, ok := store.(*MemoryStore)
	if ok {
		return &Service{store: memory, signingEncryptionKey: signingEncryptionKey, rpmWindows: map[int64][]time.Time{}}
	}
	return &Service{store: &MemoryStore{delegate: store}, signingEncryptionKey: signingEncryptionKey, rpmWindows: map[int64][]time.Time{}}
}

func (s *Service) SetUsageStore(usage UsageStore) {
	s.usage = usage
}

func (s *Service) CreateKey(ctx context.Context, req CreateRequest) (CreateResult, error) {
	if req.UserID <= 0 {
		return CreateResult{}, errs.BadRequest("user_id is required")
	}
	secret := strings.TrimSpace(req.Secret)
	if secret == "" {
		secret = "sk-" + randomToken()
	}
	signingSecret, err := domainapikey.EncryptSigningSecret(secret, s.signingEncryptionKey)
	if err != nil {
		return CreateResult{}, errs.Internal("failed to protect api key signing secret")
	}
	key := domainapikey.APIKey{
		UserID:           req.UserID,
		AccessKey:        "ak-" + randomToken(),
		SecretHash:       domainapikey.HashSecret(secret),
		SigningSecret:    signingSecret,
		Name:             defaultString(req.Name, "api-key"),
		Status:           domainapikey.StatusActive,
		GroupCode:        defaultString(req.GroupCode, "basic"),
		TotalQuotaPoints: req.TotalQuotaPoints,
		DailyQuotaPoints: req.DailyQuotaPoints,
		RPMLimit:         req.RPMLimit,
		ExpiresAt:        req.ExpiresAt,
	}
	created, err := s.store.Create(ctx, key)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Key: created, Secret: secret}, nil
}

func (s *Service) ListKeys(ctx context.Context, userID int64) ([]domainapikey.APIKey, error) {
	if userID <= 0 {
		return nil, errs.BadRequest("user_id is required")
	}
	return s.store.ListByUser(ctx, userID)
}

type UpdateRequest struct {
	UserID           int64
	ID               int64
	Name             *string
	Status           *string
	GroupCode        *string
	TotalQuotaPoints *string
	DailyQuotaPoints *string
	RPMLimit         *int
	ExpiresAt        **time.Time
	AllowGroupUpdate bool
}

func (s *Service) UpdateKey(ctx context.Context, req UpdateRequest) (domainapikey.APIKey, error) {
	key, err := s.store.GetByID(ctx, req.UserID, req.ID)
	if err != nil {
		return domainapikey.APIKey{}, mapOwnerLookupError(err)
	}
	if req.Name != nil {
		key.Name = defaultString(*req.Name, key.Name)
	}
	if req.Status != nil {
		status := strings.TrimSpace(*req.Status)
		if status != domainapikey.StatusActive && status != domainapikey.StatusDisabled {
			return domainapikey.APIKey{}, errs.BadRequest("invalid api key status")
		}
		key.Status = status
	}
	if req.GroupCode != nil {
		if !req.AllowGroupUpdate {
			return domainapikey.APIKey{}, errs.BadRequest("group_code cannot be changed")
		}
		key.GroupCode = defaultString(*req.GroupCode, key.GroupCode)
	}
	if req.TotalQuotaPoints != nil {
		key.TotalQuotaPoints = req.TotalQuotaPoints
	}
	if req.DailyQuotaPoints != nil {
		key.DailyQuotaPoints = req.DailyQuotaPoints
	}
	if req.RPMLimit != nil {
		key.RPMLimit = req.RPMLimit
	}
	if req.ExpiresAt != nil {
		key.ExpiresAt = *req.ExpiresAt
	}
	return s.store.Update(ctx, key)
}

func (s *Service) ResetSecret(ctx context.Context, userID int64, id int64) (CreateResult, error) {
	key, err := s.store.GetByID(ctx, userID, id)
	if err != nil {
		return CreateResult{}, mapOwnerLookupError(err)
	}
	secret := "sk-" + randomToken()
	signingSecret, encErr := domainapikey.EncryptSigningSecret(secret, s.signingEncryptionKey)
	if encErr != nil {
		return CreateResult{}, errs.Internal("failed to protect api key signing secret")
	}
	key.SecretHash = domainapikey.HashSecret(secret)
	key.SigningSecret = signingSecret
	updated, err := s.store.Update(ctx, key)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Key: updated, Secret: secret}, nil
}

func (s *Service) DeleteKey(ctx context.Context, userID int64, id int64) error {
	if err := s.store.SoftDelete(ctx, userID, id, time.Now().UTC()); err != nil {
		return mapOwnerLookupError(err)
	}
	return nil
}

func (s *Service) AuthenticateNative(ctx context.Context, accessKey, signature string) (domainapikey.Identity, error) {
	accessKey = strings.TrimSpace(accessKey)
	signature = strings.TrimSpace(signature)
	if accessKey == "" || signature == "" {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing api key credentials")
	}
	key, err := s.store.GetByAccessKey(ctx, accessKey)
	if err != nil {
		return domainapikey.Identity{}, mapLookupError(err)
	}
	if subtle.ConstantTimeCompare([]byte(key.SecretHash), []byte(domainapikey.HashSecret(signature))) != 1 {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid api key credentials")
	}
	return s.acceptKey(ctx, key)
}

func (s *Service) AuthenticateCanonical(ctx context.Context, method, path, timestamp, bodySHA256, accessKey, signature string, maxSkew time.Duration) (domainapikey.Identity, error) {
	accessKey = strings.TrimSpace(accessKey)
	signature = strings.TrimSpace(signature)
	if accessKey == "" || signature == "" || strings.TrimSpace(timestamp) == "" {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing api key credentials")
	}
	unixSeconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid api key timestamp")
	}
	if maxSkew <= 0 {
		maxSkew = 5 * time.Minute
	}
	if delta := time.Since(time.Unix(unixSeconds, 0)); delta > maxSkew || delta < -maxSkew {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "api key timestamp expired")
	}
	key, err := s.store.GetByAccessKey(ctx, accessKey)
	if err != nil {
		return domainapikey.Identity{}, mapLookupError(err)
	}
	secret, ok := domainapikey.DecryptSigningSecret(key.SigningSecret, s.signingEncryptionKey)
	if !ok {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid api key credentials")
	}
	canonical := strings.ToUpper(method) + path + timestamp + strings.ToLower(bodySHA256)
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(canonical))
	expected := hex.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(strings.ToLower(signature))) != 1 {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid api key credentials")
	}
	return s.acceptKey(ctx, key)
}

func (s *Service) CheckTaskAllowed(ctx context.Context, apiKeyID, userID int64, estimatedPoints string, now time.Time) (domainbilling.APIKeyQuota, error) {
	if apiKeyID <= 0 {
		return domainbilling.APIKeyQuota{}, nil
	}
	key, err := s.store.GetByID(ctx, userID, apiKeyID)
	if err != nil {
		return domainbilling.APIKeyQuota{}, mapOwnerLookupError(err)
	}
	estimated, err := decimal.NewFromString(strings.TrimSpace(estimatedPoints))
	if err != nil || estimated.IsNegative() {
		return domainbilling.APIKeyQuota{}, errs.BadRequest("estimated points must be non-negative")
	}
	if err := s.checkRPM(key, now); err != nil {
		return domainbilling.APIKeyQuota{}, err
	}
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return domainbilling.APIKeyQuota{
		APIKeyTotalQuotaPoints: key.TotalQuotaPoints,
		APIKeyDailyQuotaPoints: key.DailyQuotaPoints,
		APIKeyQuotaDayStart:    &startOfDay,
	}, nil
}

func (s *Service) apiKeyUsage(ctx context.Context, apiKeyID int64, since *time.Time) (decimal.Decimal, error) {
	value, err := s.usage.APIKeyUsage(ctx, apiKeyID, since)
	if err != nil {
		return decimal.Zero, errs.Internal("failed to load api key usage")
	}
	used, err := decimal.NewFromString(strings.TrimSpace(value))
	if err != nil {
		return decimal.Zero, errs.Internal("invalid api key usage")
	}
	if used.IsNegative() {
		return decimal.Zero, nil
	}
	return used, nil
}

func (s *Service) checkRPM(key domainapikey.APIKey, now time.Time) error {
	if key.RPMLimit == nil || *key.RPMLimit <= 0 {
		return nil
	}
	windowStart := now.Add(-time.Minute)
	s.rpmMu.Lock()
	defer s.rpmMu.Unlock()
	hits := s.rpmWindows[key.ID]
	kept := hits[:0]
	for _, hit := range hits {
		if hit.After(windowStart) {
			kept = append(kept, hit)
		}
	}
	if len(kept) >= *key.RPMLimit {
		s.rpmWindows[key.ID] = kept
		return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "api key rpm limit exceeded")
	}
	kept = append(kept, now)
	s.rpmWindows[key.ID] = kept
	return nil
}

func (s *Service) AuthenticateBearer(ctx context.Context, bearerSecret string) (domainapikey.Identity, error) {
	bearerSecret = strings.TrimSpace(bearerSecret)
	if bearerSecret == "" {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing bearer token")
	}
	key, err := s.store.GetBySecretHash(ctx, domainapikey.HashSecret(bearerSecret))
	if err != nil {
		return domainapikey.Identity{}, mapLookupError(err)
	}
	return s.acceptKey(ctx, key)
}

func (s *Service) SetStatusForTest(ctx context.Context, id int64, status string) error {
	return s.store.UpdateStatus(ctx, id, status)
}

func (s *Service) SetExpiresAtForTest(ctx context.Context, id int64, expiresAt time.Time) error {
	return s.store.UpdateExpiresAt(ctx, id, &expiresAt)
}

func (s *Service) acceptKey(ctx context.Context, key domainapikey.APIKey) (domainapikey.Identity, error) {
	if key.Status != domainapikey.StatusActive {
		return domainapikey.Identity{}, errs.New(http.StatusForbidden, errs.CodeAPIKeyDisabled, "api key is disabled")
	}
	if key.ExpiresAt != nil && time.Now().After(*key.ExpiresAt) {
		return domainapikey.Identity{}, errs.New(http.StatusForbidden, errs.CodeAPIKeyDisabled, "api key is expired")
	}
	_ = s.store.UpdateLastUsedAt(ctx, key.ID, time.Now().UTC())
	return domainapikey.Identity{
		APIKeyID:  key.ID,
		UserID:    key.UserID,
		AccessKey: key.AccessKey,
		GroupCode: defaultString(key.GroupCode, "basic"),
	}, nil
}

func mapOwnerLookupError(err error) error {
	if err == repoerr.ErrNotFound {
		return errs.New(http.StatusNotFound, errs.CodeNotFound, "api key not found")
	}
	return err
}

func mapLookupError(err error) error {
	if err == repoerr.ErrNotFound {
		return errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid api key credentials")
	}
	return errs.Internal("failed to authenticate api key")
}

func randomToken() string {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func defaultSigningEncryptionKey(value string) string {
	if strings.TrimSpace(value) != "" {
		return value
	}
	return "local-dev-api-key-signing-secret-encryption-key"
}
