package apikey

import (
	"context"
	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/shopspring/decimal"
	"net/http"
	"strings"
	"time"
)

type UsageReader interface {
	APIKeyUsage(context.Context, int64, *time.Time) (string, error)
}

// SetUsageReader must be called during initialization before serving requests.
func (s *Service) SetUsageReader(reader UsageReader) { s.usageReader = reader }

// CheckQuota validates current key state and quotas without recording usage.
// Task creation must still reserve atomically; a preflight is not a reservation.
func (s *Service) CheckQuota(ctx context.Context, identity domainapikey.Identity, points string) error {
	_, err := s.CheckTaskAllowed(ctx, identity.APIKeyID, identity.UserID, points, time.Now().UTC())
	return err
}

func (s *Service) CheckTaskAllowed(ctx context.Context, keyID, userID int64, points string, now time.Time) (domainbilling.APIKeyQuota, error) {
	if keyID <= 0 {
		return domainbilling.APIKeyQuota{}, nil
	}
	amount, err := decimal.NewFromString(strings.TrimSpace(points))
	if err != nil || amount.IsNegative() {
		return domainbilling.APIKeyQuota{}, errs.BadRequest("invalid quota points")
	}
	key, err := s.store.GetByID(ctx, userID, keyID)
	if err != nil {
		return domainbilling.APIKeyQuota{}, mapLookupError(err)
	}
	if err = validateKeyAvailable(key, now); err != nil {
		return domainbilling.APIKeyQuota{}, err
	}
	day := now.UTC().Format("2006-01-02")
	usedTotal, _ := decimal.NewFromString(defaultString(key.TotalQuotaUsedPoints, zeroPoints))
	usedDaily := decimal.Zero
	if key.QuotaUsageDay != nil && *key.QuotaUsageDay == day {
		usedDaily, _ = decimal.NewFromString(defaultString(key.DailyQuotaUsedPoints, zeroPoints))
	}
	if s.usageReader != nil {
		utc := now.UTC()
		start := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
		for _, query := range []struct {
			since *time.Time
			used  *decimal.Decimal
		}{{nil, &usedTotal}, {&start, &usedDaily}} {
			value, readErr := s.usageReader.APIKeyUsage(ctx, keyID, query.since)
			if readErr != nil {
				return domainbilling.APIKeyQuota{}, errs.Internal("failed to read api key usage")
			}
			usage, parseErr := decimal.NewFromString(value)
			if parseErr != nil || usage.IsNegative() {
				return domainbilling.APIKeyQuota{}, errs.Internal("invalid api key usage")
			}
			if usage.GreaterThan(*query.used) {
				*query.used = usage
			}
		}
	}
	for _, quota := range []struct {
		limit *string
		used  decimal.Decimal
		name  string
	}{{key.TotalQuotaPoints, usedTotal, "total"}, {key.DailyQuotaPoints, usedDaily, "daily"}} {
		if quota.limit == nil {
			continue
		}
		limit, parseErr := decimal.NewFromString(*quota.limit)
		if parseErr != nil {
			return domainbilling.APIKeyQuota{}, errs.Internal("invalid persisted api key quota")
		}
		if quota.used.Add(amount).GreaterThan(limit) {
			return domainbilling.APIKeyQuota{}, errs.New(http.StatusForbidden, errs.CodeInsufficientPoints, "api key "+quota.name+" quota exceeded")
		}
	}
	utc := now.UTC()
	dayStart := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return domainbilling.APIKeyQuota{APIKeyTotalQuotaPoints: cloneStringPtr(key.TotalQuotaPoints), APIKeyDailyQuotaPoints: cloneStringPtr(key.DailyQuotaPoints), APIKeyQuotaDayStart: &dayStart}, nil
}
