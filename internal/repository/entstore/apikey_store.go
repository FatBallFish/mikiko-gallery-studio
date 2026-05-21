package entstore

import (
	"context"
	"errors"
	"net/http"
	"time"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/apikey"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/apikeyquotareservation"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/shopspring/decimal"
)

const zeroAPIKeyPoints = "0.00000"

type APIKeyStore struct {
	client *repoent.Client
}

func NewAPIKeyStore(client *repoent.Client) *APIKeyStore {
	return &APIKeyStore{client: client}
}

func (s *APIKeyStore) Create(ctx context.Context, key domainapikey.APIKey) (domainapikey.APIKey, error) {
	create := s.client.APIKey.Create().
		SetUserID(key.UserID).
		SetAccessKey(key.AccessKey).
		SetSecretHash(key.SecretHash).
		SetNillableSecretCiphertext(stringPtrOrNil(key.SecretCiphertext)).
		SetName(key.Name).
		SetStatus(key.Status).
		SetGroupCode(key.GroupCode).
		SetNillableTotalQuotaPoints(key.TotalQuotaPoints).
		SetNillableDailyQuotaPoints(key.DailyQuotaPoints).
		SetTotalQuotaUsedPoints(defaultAPIKeyString(key.TotalQuotaUsedPoints, zeroAPIKeyPoints)).
		SetDailyQuotaUsedPoints(defaultAPIKeyString(key.DailyQuotaUsedPoints, zeroAPIKeyPoints)).
		SetNillableQuotaUsageDay(key.QuotaUsageDay).
		SetNillableRpmLimit(key.RPMLimit)
	if key.RPMWindowStartedAt != nil {
		create.SetRpmWindowStartedAt(*key.RPMWindowStartedAt)
	}
	create.SetRpmWindowCount(key.RPMWindowCount)
	if key.ExpiresAt != nil {
		create.SetExpiresAt(*key.ExpiresAt)
	}
	entity, err := create.Save(ctx)
	if err != nil {
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) ListByUser(ctx context.Context, userID int64) ([]domainapikey.APIKey, error) {
	entities, err := s.client.APIKey.Query().
		Where(apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		Order(repoent.Desc(apikey.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]domainapikey.APIKey, 0, len(entities))
	for _, entity := range entities {
		list = append(list, mapAPIKeyEntity(entity))
	}
	return list, nil
}

func (s *APIKeyStore) GetByID(ctx context.Context, userID, id int64) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Query().
		Where(apikey.IDEQ(int(id)), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainapikey.APIKey{}, repoerr.ErrNotFound
		}
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) GetByAccessKey(ctx context.Context, accessKey string) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Query().Where(apikey.AccessKeyEQ(accessKey), apikey.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainapikey.APIKey{}, repoerr.ErrNotFound
		}
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) GetBySecretHash(ctx context.Context, secretHash string) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Query().Where(apikey.SecretHashEQ(secretHash), apikey.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainapikey.APIKey{}, repoerr.ErrNotFound
		}
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func (s *APIKeyStore) UpdateForUser(ctx context.Context, userID int64, key domainapikey.APIKey) (domainapikey.APIKey, error) {
	update := s.client.APIKey.Update().
		Where(apikey.IDEQ(int(key.ID)), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		SetName(key.Name).
		SetGroupCode(key.GroupCode).
		SetNillableTotalQuotaPoints(key.TotalQuotaPoints).
		SetNillableDailyQuotaPoints(key.DailyQuotaPoints).
		SetNillableRpmLimit(key.RPMLimit)
	if key.ExpiresAt == nil {
		update.ClearExpiresAt()
	} else {
		update.SetExpiresAt(*key.ExpiresAt)
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return domainapikey.APIKey{}, err
	}
	if updated != 1 {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	return s.GetByID(ctx, userID, key.ID)
}

func (s *APIKeyStore) UpdateLastUsedAt(ctx context.Context, id int64, at time.Time) error {
	updated, err := s.client.APIKey.Update().
		Where(apikey.IDEQ(int(id)), apikey.DeletedAtIsNil()).
		SetLastUsedAt(at).
		Save(ctx)
	return mapUpdatedCountError(updated, err)
}

func (s *APIKeyStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	updated, err := s.client.APIKey.Update().
		Where(apikey.IDEQ(int(id)), apikey.DeletedAtIsNil()).
		SetStatus(status).
		Save(ctx)
	return mapUpdatedCountError(updated, err)
}

func (s *APIKeyStore) UpdateStatusForUser(ctx context.Context, userID, id int64, status string) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Update().
		Where(apikey.IDEQ(int(id)), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		SetStatus(status).
		Save(ctx)
	if err != nil {
		return domainapikey.APIKey{}, err
	}
	if entity != 1 {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	return s.GetByID(ctx, userID, id)
}

func (s *APIKeyStore) UpdateExpiresAt(ctx context.Context, id int64, expiresAt *time.Time) error {
	update := s.client.APIKey.Update().Where(apikey.IDEQ(int(id)), apikey.DeletedAtIsNil())
	if expiresAt == nil {
		update.ClearExpiresAt()
	} else {
		update.SetExpiresAt(*expiresAt)
	}
	updated, err := update.Save(ctx)
	return mapUpdatedCountError(updated, err)
}

func (s *APIKeyStore) UpdateSecretForUser(ctx context.Context, userID, id int64, secretHash string, secretCiphertext string) (domainapikey.APIKey, error) {
	updated, err := s.client.APIKey.Update().
		Where(apikey.IDEQ(int(id)), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		SetSecretHash(secretHash).
		SetNillableSecretCiphertext(stringPtrOrNil(secretCiphertext)).
		Save(ctx)
	if err != nil {
		return domainapikey.APIKey{}, err
	}
	if updated != 1 {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	return s.GetByID(ctx, userID, id)
}

func (s *APIKeyStore) DeleteForUser(ctx context.Context, userID, id int64, at time.Time) error {
	updated, err := s.client.APIKey.Update().
		Where(apikey.IDEQ(int(id)), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		SetStatus(domainapikey.StatusRevoked).
		SetDeletedAt(at).
		Save(ctx)
	return mapUpdatedCountError(updated, err)
}

func (s *APIKeyStore) RecordRequest(ctx context.Context, id int64, rpmLimit int, at time.Time) error {
	if rpmLimit <= 0 {
		return nil
	}
	windowCutoff := at.Add(-time.Minute)
	for attempt := 0; attempt < 16; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		updated, err := s.client.APIKey.Update().
			Where(apikey.IDEQ(int(id)), apikey.DeletedAtIsNil()).
			Where(apikey.Or(apikey.RpmWindowStartedAtIsNil(), apikey.RpmWindowStartedAtLTE(windowCutoff))).
			SetRpmWindowStartedAt(at).
			SetRpmWindowCount(1).
			Save(ctx)
		if err != nil {
			return err
		}
		if updated == 1 {
			return nil
		}

		updated, err = s.client.APIKey.Update().
			Where(apikey.IDEQ(int(id)), apikey.DeletedAtIsNil()).
			Where(apikey.RpmWindowStartedAtGT(windowCutoff)).
			Where(apikey.RpmWindowCountLT(rpmLimit)).
			AddRpmWindowCount(1).
			Save(ctx)
		if err != nil {
			return err
		}
		if updated == 1 {
			return nil
		}

		key, err := s.getByIDOnly(ctx, id)
		if err != nil {
			return err
		}
		if key.RPMWindowStartedAt != nil && key.RPMWindowStartedAt.After(windowCutoff) && key.RPMWindowCount >= rpmLimit {
			return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "api key rate limit exceeded")
		}
	}
	return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "api key rate limit temporarily saturated")
}

func (s *APIKeyStore) ReserveQuota(ctx context.Context, userID, id int64, reservationID, points string, at time.Time) error {
	parsedPoints, err := decimal.NewFromString(points)
	if err != nil {
		return err
	}
	if parsedPoints.IsZero() {
		return nil
	}
	for attempt := 0; attempt < 3; attempt++ {
		err := s.reserveQuotaOnce(ctx, userID, id, reservationID, parsedPoints, at)
		if errors.Is(err, repoerr.ErrConflict) {
			continue
		}
		return err
	}
	return errs.New(http.StatusConflict, errs.CodeConflict, "api key quota update conflict")
}

func (s *APIKeyStore) reserveQuotaOnce(ctx context.Context, userID, id int64, reservationID string, points decimal.Decimal, at time.Time) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	existing, err := tx.APIKeyQuotaReservation.Query().
		Where(apikeyquotareservation.APIKeyIDEQ(id), apikeyquotareservation.ReservationIDEQ(reservationID)).
		Only(ctx)
	reservationStatus := ""
	if err == nil {
		if existing.Points != formatDecimal(points) {
			return errs.New(http.StatusConflict, errs.CodeConflict, "quota reservation points conflict")
		}
		if existing.Status == "active" {
			return tx.Commit()
		}
		if existing.Status != "released" {
			return errs.New(http.StatusConflict, errs.CodeConflict, "quota reservation conflict")
		}
		reservationStatus = existing.Status
	} else if !repoent.IsNotFound(err) {
		return err
	}

	entity, err := tx.APIKey.Query().
		Where(apikey.IDEQ(int(id)), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	key := mapAPIKeyEntity(entity)
	day := at.UTC().Format("2006-01-02")
	totalUsed := parseDecimalOrZero(key.TotalQuotaUsedPoints)
	dailyUsed := parseDecimalOrZero(key.DailyQuotaUsedPoints)
	if key.QuotaUsageDay == nil || *key.QuotaUsageDay != day {
		dailyUsed = decimal.Zero
	}
	if key.TotalQuotaPoints != nil {
		limit, err := decimal.NewFromString(*key.TotalQuotaPoints)
		if err != nil {
			return err
		}
		if totalUsed.Add(points).GreaterThan(limit) {
			return errs.New(http.StatusForbidden, errs.CodeInsufficientPoints, "api key total quota exceeded")
		}
	}
	if key.DailyQuotaPoints != nil {
		limit, err := decimal.NewFromString(*key.DailyQuotaPoints)
		if err != nil {
			return err
		}
		if dailyUsed.Add(points).GreaterThan(limit) {
			return errs.New(http.StatusForbidden, errs.CodeInsufficientPoints, "api key daily quota exceeded")
		}
	}
	update := tx.APIKey.Update().
		Where(apikey.IDEQ(int(id)), apikey.UserIDEQ(userID), apikey.DeletedAtIsNil()).
		Where(apikey.TotalQuotaUsedPointsEQ(defaultAPIKeyString(key.TotalQuotaUsedPoints, zeroAPIKeyPoints))).
		Where(apikey.DailyQuotaUsedPointsEQ(defaultAPIKeyString(key.DailyQuotaUsedPoints, zeroAPIKeyPoints))).
		SetTotalQuotaUsedPoints(formatDecimal(totalUsed.Add(points))).
		SetDailyQuotaUsedPoints(formatDecimal(dailyUsed.Add(points))).
		SetQuotaUsageDay(day)
	if key.QuotaUsageDay == nil {
		update.Where(apikey.QuotaUsageDayIsNil())
	} else {
		update.Where(apikey.QuotaUsageDayEQ(*key.QuotaUsageDay))
	}
	updated, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if updated != 1 {
		return repoerr.ErrConflict
	}
	if reservationStatus == "released" {
		updatedReservation, err := tx.APIKeyQuotaReservation.Update().
			Where(apikeyquotareservation.IDEQ(existing.ID), apikeyquotareservation.StatusEQ("released")).
			SetUsageDay(day).
			SetStatus("active").
			Save(ctx)
		if err != nil {
			return err
		}
		if updatedReservation != 1 {
			return repoerr.ErrConflict
		}
	} else {
		if _, err := tx.APIKeyQuotaReservation.Create().
			SetAPIKeyID(id).
			SetReservationID(reservationID).
			SetPoints(formatDecimal(points)).
			SetUsageDay(day).
			SetStatus("active").
			Save(ctx); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *APIKeyStore) ReleaseQuota(ctx context.Context, id int64, reservationID string) error {
	for attempt := 0; attempt < 3; attempt++ {
		err := s.releaseQuotaOnce(ctx, id, reservationID)
		if errors.Is(err, repoerr.ErrConflict) {
			continue
		}
		return err
	}
	return errs.New(http.StatusConflict, errs.CodeConflict, "api key quota release conflict")
}

func (s *APIKeyStore) releaseQuotaOnce(ctx context.Context, id int64, reservationID string) error {
	tx, err := s.client.Tx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	reservation, err := tx.APIKeyQuotaReservation.Query().
		Where(apikeyquotareservation.APIKeyIDEQ(id), apikeyquotareservation.ReservationIDEQ(reservationID)).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return nil
		}
		return err
	}
	if reservation.Status != "active" {
		return tx.Commit()
	}
	updatedReservation, err := tx.APIKeyQuotaReservation.Update().
		Where(apikeyquotareservation.IDEQ(reservation.ID), apikeyquotareservation.StatusEQ("active")).
		SetStatus("released").
		Save(ctx)
	if err != nil {
		return err
	}
	if updatedReservation == 0 {
		return tx.Commit()
	}
	entity, err := tx.APIKey.Query().Where(apikey.IDEQ(int(id)), apikey.DeletedAtIsNil()).Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return repoerr.ErrNotFound
		}
		return err
	}
	key := mapAPIKeyEntity(entity)
	points, err := decimal.NewFromString(reservation.Points)
	if err != nil {
		return err
	}
	totalUsed := parseDecimalOrZero(key.TotalQuotaUsedPoints).Sub(points)
	if totalUsed.IsNegative() {
		totalUsed = decimal.Zero
	}
	dailyUsed := parseDecimalOrZero(key.DailyQuotaUsedPoints)
	if key.QuotaUsageDay != nil && *key.QuotaUsageDay == reservation.UsageDay {
		dailyUsed = dailyUsed.Sub(points)
		if dailyUsed.IsNegative() {
			dailyUsed = decimal.Zero
		}
	}
	update := tx.APIKey.Update().
		Where(apikey.IDEQ(int(id)), apikey.DeletedAtIsNil()).
		Where(apikey.TotalQuotaUsedPointsEQ(defaultAPIKeyString(key.TotalQuotaUsedPoints, zeroAPIKeyPoints))).
		Where(apikey.DailyQuotaUsedPointsEQ(defaultAPIKeyString(key.DailyQuotaUsedPoints, zeroAPIKeyPoints))).
		SetTotalQuotaUsedPoints(formatDecimal(totalUsed)).
		SetDailyQuotaUsedPoints(formatDecimal(dailyUsed))
	if key.QuotaUsageDay == nil {
		update.Where(apikey.QuotaUsageDayIsNil())
	} else {
		update.Where(apikey.QuotaUsageDayEQ(*key.QuotaUsageDay))
	}
	updatedKey, err := update.Save(ctx)
	if err != nil {
		return err
	}
	if updatedKey != 1 {
		return repoerr.ErrConflict
	}
	return tx.Commit()
}

func (s *APIKeyStore) getByIDOnly(ctx context.Context, id int64) (domainapikey.APIKey, error) {
	entity, err := s.client.APIKey.Query().
		Where(apikey.IDEQ(int(id)), apikey.DeletedAtIsNil()).
		Only(ctx)
	if err != nil {
		if repoent.IsNotFound(err) {
			return domainapikey.APIKey{}, repoerr.ErrNotFound
		}
		return domainapikey.APIKey{}, err
	}
	return mapAPIKeyEntity(entity), nil
}

func mapUpdatedCountError(updated int, err error) error {
	if err != nil {
		return err
	}
	if updated == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

func mapAPIKeyEntity(entity *repoent.APIKey) domainapikey.APIKey {
	return domainapikey.APIKey{
		ID:                   int64(entity.ID),
		UserID:               entity.UserID,
		AccessKey:            entity.AccessKey,
		SecretHash:           entity.SecretHash,
		SecretCiphertext:     stringValue(entity.SecretCiphertext),
		SigningSecret:        stringValue(entity.SecretCiphertext),
		Name:                 entity.Name,
		Status:               entity.Status,
		GroupCode:            entity.GroupCode,
		TotalQuotaPoints:     entity.TotalQuotaPoints,
		DailyQuotaPoints:     entity.DailyQuotaPoints,
		TotalQuotaUsedPoints: entity.TotalQuotaUsedPoints,
		DailyQuotaUsedPoints: entity.DailyQuotaUsedPoints,
		QuotaUsageDay:        entity.QuotaUsageDay,
		RPMLimit:             entity.RpmLimit,
		RPMWindowStartedAt:   entity.RpmWindowStartedAt,
		RPMWindowCount:       entity.RpmWindowCount,
		ExpiresAt:            entity.ExpiresAt,
		LastUsedAt:           entity.LastUsedAt,
		DeletedAt:            entity.DeletedAt,
		CreatedAt:            entity.CreatedAt,
		UpdatedAt:            entity.UpdatedAt,
	}
}

func parseDecimalOrZero(value string) decimal.Decimal {
	parsed, err := decimal.NewFromString(defaultAPIKeyString(value, zeroAPIKeyPoints))
	if err != nil {
		return decimal.Zero
	}
	return parsed
}

func formatDecimal(value decimal.Decimal) string {
	return value.Round(5).StringFixed(5)
}

func defaultAPIKeyString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func stringPtrOrNil(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
