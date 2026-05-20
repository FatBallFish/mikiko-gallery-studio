package apikey

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type CreateRequest struct {
	UserID    int64
	Name      string
	GroupCode string
	Secret    string
	ExpiresAt *time.Time
}

type CreateResult struct {
	Key    domainapikey.APIKey
	Secret string
}

type Service struct {
	store *MemoryStore
}

func NewService(store Store) *Service {
	if store == nil {
		store = NewMemoryStore()
	}
	memory, ok := store.(*MemoryStore)
	if ok {
		return &Service{store: memory}
	}
	return &Service{store: &MemoryStore{delegate: store}}
}

func (s *Service) CreateKey(ctx context.Context, req CreateRequest) (CreateResult, error) {
	if req.UserID <= 0 {
		return CreateResult{}, errs.BadRequest("user_id is required")
	}
	secret := strings.TrimSpace(req.Secret)
	if secret == "" {
		secret = "sk-" + randomToken()
	}
	key := domainapikey.APIKey{
		UserID:     req.UserID,
		AccessKey:  "ak-" + randomToken(),
		SecretHash: domainapikey.HashSecret(secret),
		Name:       defaultString(req.Name, "api-key"),
		Status:     domainapikey.StatusActive,
		GroupCode:  defaultString(req.GroupCode, "basic"),
		ExpiresAt:  req.ExpiresAt,
	}
	created, err := s.store.Create(ctx, key)
	if err != nil {
		return CreateResult{}, err
	}
	return CreateResult{Key: created, Secret: secret}, nil
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
