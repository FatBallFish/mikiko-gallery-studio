package videotask

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	videopricingservice "github.com/fatballfish/pic-gallery/internal/service/videopricing"
	videoroutingservice "github.com/fatballfish/pic-gallery/internal/service/videorouting"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

const videoQuoteTTL = 120 * time.Second

type EstimateRequest struct {
	RouteModelCode string
	Video          domainvideo.Request
}

type Estimate struct {
	RouteModelID      int64                 `json:"route_model_id,omitempty"`
	RouteModelCode    string                `json:"route_model_code"`
	CapabilityVersion string                `json:"capability_version"`
	ConfigVersion     string                `json:"config_version"`
	PriceVersion      string                `json:"price_version"`
	UnitPoints        string                `json:"unit_points"`
	EstimatedPoints   string                `json:"estimated_points"`
	MaxReservedPoints string                `json:"max_reserved_points"`
	QuoteToken        string                `json:"quote_token"`
	ExpiresAt         time.Time             `json:"expires_at"`
	RouteCandidateID  int64                 `json:"-"`
	AccountModelID    int64                 `json:"-"`
	ModelAccountID    int64                 `json:"-"`
	ProviderCode      string                `json:"-"`
	ModelCode         string                `json:"-"`
	SalesRule         domainvideo.SalesRule `json:"-"`
}

type QuoteService struct {
	routing *videoroutingservice.Service
	pricing *videopricingservice.Service
	key     []byte
	now     func() time.Time
}

type quoteTokenPayload struct {
	UserID            int64                 `json:"uid"`
	Fingerprint       string                `json:"fp"`
	CapabilityVersion string                `json:"cv"`
	ConfigVersion     string                `json:"rv"`
	PriceVersion      string                `json:"pv"`
	UnitPoints        string                `json:"up"`
	EstimatedPoints   string                `json:"ep"`
	MaxReservedPoints string                `json:"mr"`
	ExpiresAtUnix     int64                 `json:"exp"`
	RouteCandidateID  int64                 `json:"rc"`
	AccountModelID    int64                 `json:"am"`
	ModelAccountID    int64                 `json:"ma"`
	ProviderCode      string                `json:"pc"`
	ModelCode         string                `json:"mc"`
	SalesRule         domainvideo.SalesRule `json:"sr"`
}

func NewQuoteService(routing *videoroutingservice.Service, pricing *videopricingservice.Service, key []byte, now func() time.Time) *QuoteService {
	if now == nil {
		now = time.Now
	}
	return &QuoteService{routing: routing, pricing: pricing, key: append([]byte(nil), key...), now: now}
}

func (s *QuoteService) Estimate(ctx context.Context, userID int64, request EstimateRequest) (Estimate, error) {
	if s == nil || s.routing == nil || s.pricing == nil || len(s.key) < 32 {
		return Estimate{}, errs.Internal("video quote service is unavailable")
	}
	resolved, err := s.routing.Resolve(ctx, request.RouteModelCode, request.Video)
	if err != nil {
		return Estimate{}, err
	}
	quoted, err := s.pricing.Quote(ctx, resolved.Group.PricingStrategyFor(request.Video), request.Video)
	if err != nil {
		return Estimate{}, err
	}
	selected := resolved.Candidates[0]
	fingerprint, err := estimateFingerprint(request)
	if err != nil {
		return Estimate{}, err
	}
	expiresAt := s.now().UTC().Add(videoQuoteTTL)
	payload := quoteTokenPayload{
		UserID: userID, Fingerprint: fingerprint, CapabilityVersion: resolved.CapabilityVersion, ConfigVersion: resolved.Group.ConfigVersion,
		PriceVersion: quoted.PriceVersion, UnitPoints: quoted.UnitPoints, EstimatedPoints: quoted.EstimatedPoints,
		MaxReservedPoints: quoted.MaxReservedPoints, ExpiresAtUnix: expiresAt.Unix(),
		RouteCandidateID: selected.RouteCandidateID, AccountModelID: selected.AccountModelID, ModelAccountID: selected.ModelAccountID,
		ProviderCode: selected.AdapterType, ModelCode: selected.ModelCode,
		SalesRule: quoted.SalesRule,
	}
	token, err := s.sign(payload)
	if err != nil {
		return Estimate{}, err
	}
	return Estimate{
		RouteModelID: resolved.Group.RouteModelID, RouteModelCode: request.RouteModelCode, CapabilityVersion: payload.CapabilityVersion, ConfigVersion: payload.ConfigVersion,
		PriceVersion: payload.PriceVersion, UnitPoints: payload.UnitPoints, EstimatedPoints: payload.EstimatedPoints,
		MaxReservedPoints: payload.MaxReservedPoints, QuoteToken: token, ExpiresAt: expiresAt,
		RouteCandidateID: payload.RouteCandidateID, AccountModelID: payload.AccountModelID, ModelAccountID: payload.ModelAccountID,
		ProviderCode: payload.ProviderCode, ModelCode: payload.ModelCode, SalesRule: payload.SalesRule,
	}, nil
}

func (s *QuoteService) Verify(ctx context.Context, userID int64, request EstimateRequest, token string) (Estimate, error) {
	payload, err := s.verifySignature(token)
	if err != nil {
		return Estimate{}, err
	}
	if payload.UserID != userID || s.now().UTC().Unix() >= payload.ExpiresAtUnix {
		return Estimate{}, errs.New(409, errs.CodeVideoQuoteStale, "video quote expired or belongs to another user")
	}
	fingerprint, err := estimateFingerprint(request)
	if err != nil || !hmac.Equal([]byte(payload.Fingerprint), []byte(fingerprint)) {
		return Estimate{}, errs.New(409, errs.CodeVideoQuoteStale, "video quote does not match the request")
	}
	current, err := s.Estimate(ctx, userID, request)
	if err != nil {
		return Estimate{}, err
	}
	if current.CapabilityVersion != payload.CapabilityVersion || current.ConfigVersion != payload.ConfigVersion || current.PriceVersion != payload.PriceVersion {
		return Estimate{}, errs.New(409, errs.CodeVideoQuoteStale, "video capability or price changed; request a new quote")
	}
	if current.RouteCandidateID != payload.RouteCandidateID || current.AccountModelID != payload.AccountModelID || current.ModelAccountID != payload.ModelAccountID || current.ProviderCode != payload.ProviderCode || current.ModelCode != payload.ModelCode {
		return Estimate{}, errs.New(409, errs.CodeVideoQuoteStale, "video routing candidate changed; request a new quote")
	}
	current.QuoteToken = token
	current.ExpiresAt = time.Unix(payload.ExpiresAtUnix, 0).UTC()
	return current, nil
}

func (s *QuoteService) sign(payload quoteTokenPayload) (string, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(encoded)
	return base64.RawURLEncoding.EncodeToString(encoded) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (s *QuoteService) verifySignature(token string) (quoteTokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return quoteTokenPayload{}, errs.New(409, errs.CodeVideoQuoteStale, "invalid video quote token")
	}
	payload, payloadErr := base64.RawURLEncoding.DecodeString(parts[0])
	signature, signatureErr := base64.RawURLEncoding.DecodeString(parts[1])
	if payloadErr != nil || signatureErr != nil {
		return quoteTokenPayload{}, errs.New(409, errs.CodeVideoQuoteStale, "invalid video quote token")
	}
	mac := hmac.New(sha256.New, s.key)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return quoteTokenPayload{}, errs.New(409, errs.CodeVideoQuoteStale, "invalid video quote signature")
	}
	var decoded quoteTokenPayload
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return quoteTokenPayload{}, errs.New(409, errs.CodeVideoQuoteStale, "invalid video quote payload")
	}
	return decoded, nil
}

func estimateFingerprint(request EstimateRequest) (string, error) {
	payload, err := json.Marshal(struct {
		RouteModelCode string              `json:"route_model_code"`
		Video          domainvideo.Request `json:"video"`
	}{strings.TrimSpace(request.RouteModelCode), request.Video})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:]), nil
}
