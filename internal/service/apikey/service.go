package apikey

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/shopspring/decimal"
)

const (
	defaultHMACMaxDrift = 5 * time.Minute
	zeroPoints          = "0.00000"
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

type ResetSecretResult struct {
	Key    domainapikey.APIKey
	Secret string
}

type UpdateRequest struct {
	Name             *string
	GroupCode        *string
	TotalQuotaPoints *string
	DailyQuotaPoints *string
	RPMLimit         *int
	ExpiresAt        *time.Time
}

type HMACRequest struct {
	AccessKey  string
	Method     string
	Path       string
	Timestamp  time.Time
	Body       []byte
	BodySHA256 string
	Signature  string
	MaxDrift   time.Duration
	Now        time.Time
}

type Service struct {
	store                      *MemoryStore
	signingSecretEncryptionKey string
}

type quotaReservation struct {
	KeyID  int64
	Day    string
	Points decimal.Decimal
	Status string
}

func NewService(store Store) *Service {
	return NewServiceWithSigningSecretKey(store, defaultSigningSecretEncryptionKey())
}

func NewServiceWithSigningSecretKey(store Store, signingSecretEncryptionKey string) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	memory, ok := store.(*MemoryStore)
	if ok {
		return newServiceWithMemoryStore(memory, signingSecretEncryptionKey)
	}
	return newServiceWithMemoryStore(&MemoryStore{delegate: store}, signingSecretEncryptionKey)
}

func newServiceWithMemoryStore(store *MemoryStore, signingSecretEncryptionKey string) *Service {
	if strings.TrimSpace(signingSecretEncryptionKey) == "" {
		signingSecretEncryptionKey = defaultSigningSecretEncryptionKey()
	}
	return &Service{store: store, signingSecretEncryptionKey: signingSecretEncryptionKey}
}

func (s *Service) CreateKey(ctx context.Context, req CreateRequest) (CreateResult, error) {
	if req.UserID <= 0 {
		return CreateResult{}, errs.BadRequest("user_id is required")
	}
	if req.RPMLimit != nil && *req.RPMLimit <= 0 {
		return CreateResult{}, errs.BadRequest("rpm_limit must be greater than 0")
	}
	secret := strings.TrimSpace(req.Secret)
	if secret == "" {
		secret = "sk-" + randomToken()
	}
	secretCiphertext, err := s.encryptSecret(secret)
	if err != nil {
		return CreateResult{}, errs.Internal("failed to protect api key secret")
	}
	key := domainapikey.APIKey{
		UserID:           req.UserID,
		AccessKey:        "ak-" + randomToken(),
		SecretHash:       domainapikey.HashSecret(secret),
		SecretCiphertext: secretCiphertext,
		SigningSecret:    secretCiphertext,
		Name:             defaultString(req.Name, "api-key"),
		Status:           domainapikey.StatusActive,
		GroupCode:        defaultString(req.GroupCode, "basic"),
		TotalQuotaPoints: cloneStringPtr(req.TotalQuotaPoints),
		DailyQuotaPoints: cloneStringPtr(req.DailyQuotaPoints),
		RPMLimit:         cloneIntPtr(req.RPMLimit),
		ExpiresAt:        req.ExpiresAt,
	}
	created, err := s.store.Create(ctx, key)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Key: created, Secret: secret}, nil
}

func (s *Service) ListByUser(ctx context.Context, userID int64) ([]domainapikey.APIKey, error) {
	if userID <= 0 {
		return nil, errs.BadRequest("user_id is required")
	}
	list, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, errs.Internal("failed to list api keys")
	}
	return list, nil
}

func (s *Service) GetByID(ctx context.Context, userID, id int64) (domainapikey.APIKey, error) {
	if userID <= 0 || id <= 0 {
		return domainapikey.APIKey{}, errs.BadRequest("user_id and id are required")
	}
	key, err := s.store.GetByID(ctx, userID, id)
	if err != nil {
		return domainapikey.APIKey{}, mapLifecycleError(err, "api key not found", "failed to load api key")
	}
	return key, nil
}

func (s *Service) Update(ctx context.Context, userID, id int64, req UpdateRequest) (domainapikey.APIKey, error) {
	if userID <= 0 || id <= 0 {
		return domainapikey.APIKey{}, errs.BadRequest("user_id and id are required")
	}
	if req.RPMLimit != nil && *req.RPMLimit <= 0 {
		return domainapikey.APIKey{}, errs.BadRequest("rpm_limit must be greater than 0")
	}
	key, err := s.store.GetByID(ctx, userID, id)
	if err != nil {
		return domainapikey.APIKey{}, mapLifecycleError(err, "api key not found", "failed to load api key")
	}
	if req.Name != nil {
		key.Name = defaultString(*req.Name, "api-key")
	}
	if req.GroupCode != nil {
		key.GroupCode = defaultString(*req.GroupCode, "basic")
	}
	if req.TotalQuotaPoints != nil {
		key.TotalQuotaPoints = cloneStringPtr(req.TotalQuotaPoints)
	}
	if req.DailyQuotaPoints != nil {
		key.DailyQuotaPoints = cloneStringPtr(req.DailyQuotaPoints)
	}
	if req.RPMLimit != nil {
		key.RPMLimit = cloneIntPtr(req.RPMLimit)
	}
	if req.ExpiresAt != nil {
		key.ExpiresAt = req.ExpiresAt
	}
	updated, err := s.store.UpdateForUser(ctx, userID, key)
	if err != nil {
		return domainapikey.APIKey{}, mapLifecycleError(err, "api key not found", "failed to update api key")
	}
	return updated, nil
}

func (s *Service) UpdateStatus(ctx context.Context, userID, id int64, status string) error {
	status = strings.TrimSpace(status)
	if userID <= 0 || id <= 0 || status == "" {
		return errs.BadRequest("user_id, id and status are required")
	}
	if !validStatus(status) {
		return errs.BadRequest("invalid api key status")
	}
	_, err := s.store.UpdateStatusForUser(ctx, userID, id, status)
	if err != nil {
		return mapLifecycleError(err, "api key not found", "failed to update api key status")
	}
	return nil
}

func (s *Service) Delete(ctx context.Context, userID, id int64) error {
	if userID <= 0 || id <= 0 {
		return errs.BadRequest("user_id and id are required")
	}
	if err := s.store.DeleteForUser(ctx, userID, id, time.Now().UTC()); err != nil {
		return mapLifecycleError(err, "api key not found", "failed to delete api key")
	}
	return nil
}

func (s *Service) ResetSecret(ctx context.Context, userID, id int64) (ResetSecretResult, error) {
	if userID <= 0 || id <= 0 {
		return ResetSecretResult{}, errs.BadRequest("user_id and id are required")
	}
	secret := "sk-" + randomToken()
	secretCiphertext, err := s.encryptSecret(secret)
	if err != nil {
		return ResetSecretResult{}, errs.Internal("failed to protect api key secret")
	}
	key, err := s.store.UpdateSecretForUser(ctx, userID, id, domainapikey.HashSecret(secret), secretCiphertext)
	if err != nil {
		return ResetSecretResult{}, mapLifecycleError(err, "api key not found", "failed to reset api key secret")
	}
	return ResetSecretResult{Key: key, Secret: secret}, nil
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

func (s *Service) VerifyCanonicalHMAC(ctx context.Context, req HMACRequest) (domainapikey.Identity, error) {
	accessKey := strings.TrimSpace(req.AccessKey)
	signature := strings.TrimSpace(req.Signature)
	bodyHash := strings.TrimSpace(req.BodySHA256)
	if accessKey == "" || signature == "" || bodyHash == "" || req.Timestamp.IsZero() {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "missing api key credentials")
	}
	if subtle.ConstantTimeCompare([]byte(bodyHash), []byte(BodySHA256(req.Body))) != 1 {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid api key body hash")
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	drift := req.MaxDrift
	if drift <= 0 {
		drift = defaultHMACMaxDrift
	}
	if absDuration(now.Sub(req.Timestamp)) > drift {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "api key timestamp expired")
	}
	key, err := s.store.GetByAccessKey(ctx, accessKey)
	if err != nil {
		return domainapikey.Identity{}, mapLookupError(err)
	}
	signingSecret, err := s.decryptSecret(key.SecretCiphertext)
	if err != nil {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "api key signing secret is unavailable")
	}
	expected := signCanonicalHMACWithKey(signingSecret, req.Method, req.Path, req.Timestamp, bodyHash)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return domainapikey.Identity{}, errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid api key credentials")
	}
	return s.acceptKey(ctx, key)
}

func (s *Service) ReserveQuota(ctx context.Context, identity domainapikey.Identity, reservationID, points string) error {
	if identity.APIKeyID <= 0 {
		return nil
	}
	reservationID = strings.TrimSpace(reservationID)
	if reservationID == "" {
		return errs.BadRequest("quota reservation id is required")
	}
	parsedPoints, err := decimal.NewFromString(strings.TrimSpace(points))
	if err != nil {
		return errs.BadRequest("invalid quota points")
	}
	if parsedPoints.IsNegative() {
		return errs.BadRequest("quota points must be non-negative")
	}
	if parsedPoints.IsZero() {
		return nil
	}
	if err := s.store.ReserveQuota(ctx, identity.UserID, identity.APIKeyID, reservationID, normalizePoints(parsedPoints), time.Now().UTC()); err != nil {
		return mapQuotaError(err)
	}
	return nil
}

func (s *Service) ReleaseQuota(ctx context.Context, identity domainapikey.Identity, reservationID string) error {
	if identity.APIKeyID <= 0 || strings.TrimSpace(reservationID) == "" {
		return nil
	}
	return s.store.ReleaseQuota(ctx, identity.APIKeyID, reservationID)
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
	if key.RPMLimit != nil {
		if err := s.store.RecordRequest(ctx, key.ID, *key.RPMLimit, time.Now().UTC()); err != nil {
			return domainapikey.Identity{}, mapQuotaError(err)
		}
	}
	_ = s.store.UpdateLastUsedAt(ctx, key.ID, time.Now().UTC())
	return domainapikey.Identity{
		APIKeyID:  key.ID,
		UserID:    key.UserID,
		AccessKey: key.AccessKey,
		GroupCode: defaultString(key.GroupCode, "basic"),
	}, nil
}

func BodySHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func SignCanonicalHMAC(secret, method, path string, timestamp time.Time, bodySHA256 string) string {
	return signCanonicalHMACWithKey(secret, method, path, timestamp, bodySHA256)
}

func signCanonicalHMACWithKey(secretKey, method, path string, timestamp time.Time, bodySHA256 string) string {
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(canonicalHMACPayload(method, path, timestamp, bodySHA256)))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func canonicalHMACPayload(method, path string, timestamp time.Time, bodySHA256 string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + "\n" + strings.TrimSpace(path) + "\n" + timestamp.UTC().Format(time.RFC3339) + "\n" + strings.TrimSpace(bodySHA256)
}

func (s *Service) encryptSecret(secret string) (string, error) {
	return domainapikey.EncryptSigningSecret(secret, s.signingSecretEncryptionKey)
}

func (s *Service) decryptSecret(ciphertext string) (string, error) {
	if secret, ok := domainapikey.DecryptSigningSecret(ciphertext, s.signingSecretEncryptionKey); ok {
		return secret, nil
	}
	return "", errors.New("unsupported secret ciphertext")
}

func defaultSigningSecretEncryptionKey() string {
	material := strings.TrimSpace(os.Getenv("PIC_GALLERY_API_KEY_ENCRYPTION_KEY"))
	if material == "" {
		material = strings.TrimSpace(os.Getenv("AUTH_ACCESS_TOKEN_SECRET"))
	}
	if material == "" {
		material = "pic-gallery-local-api-key-secret-encryption"
	}
	return material
}

func mapLookupError(err error) error {
	if errors.Is(err, repoerr.ErrNotFound) {
		return errs.New(http.StatusUnauthorized, errs.CodeUnauthorized, "invalid api key credentials")
	}
	return errs.Internal("failed to authenticate api key")
}

func mapLifecycleError(err error, notFoundMessage, internalMessage string) error {
	if errors.Is(err, repoerr.ErrNotFound) {
		return errs.New(http.StatusNotFound, errs.CodeNotFound, notFoundMessage)
	}
	return errs.Internal(internalMessage)
}

func mapQuotaError(err error) error {
	if err == nil {
		return nil
	}
	if appErr, ok := err.(*errs.Error); ok {
		return appErr
	}
	if errors.Is(err, repoerr.ErrNotFound) {
		return errs.New(http.StatusNotFound, errs.CodeNotFound, "api key not found")
	}
	return errs.Internal("failed to update api key usage")
}

func validStatus(status string) bool {
	switch status {
	case domainapikey.StatusActive, domainapikey.StatusDisabled, domainapikey.StatusRevoked:
		return true
	default:
		return false
	}
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

func cloneStringPtr(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneIntPtr(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func absDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
}

func quotaReservationKey(keyID int64, reservationID string) string {
	return fmt.Sprintf("%d:%s", keyID, strings.TrimSpace(reservationID))
}

func normalizePoints(value decimal.Decimal) string {
	return value.Round(5).StringFixed(5)
}
