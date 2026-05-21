package apikey

import (
	"context"
	"net/http"
	"sort"
	"sync"
	"time"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/shopspring/decimal"
)

type MemoryStore struct {
	mu                sync.Mutex
	nextID            int64
	keysByID          map[int64]domainapikey.APIKey
	accessIndex       map[string]int64
	secretIndex       map[string]int64
	quotaReservations map[string]quotaReservation
	delegate          Store
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		nextID:            1,
		keysByID:          map[int64]domainapikey.APIKey{},
		accessIndex:       map[string]int64{},
		secretIndex:       map[string]int64{},
		quotaReservations: map[string]quotaReservation{},
	}
}

func (s *MemoryStore) Create(ctx context.Context, key domainapikey.APIKey) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.Create(ctx, key)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	key.ID = s.nextID
	s.nextID++
	now := time.Now().UTC()
	key.CreatedAt = now
	key.UpdatedAt = now
	if key.TotalQuotaUsedPoints == "" {
		key.TotalQuotaUsedPoints = zeroPoints
	}
	if key.DailyQuotaUsedPoints == "" {
		key.DailyQuotaUsedPoints = zeroPoints
	}
	s.keysByID[key.ID] = key
	s.accessIndex[key.AccessKey] = key.ID
	s.secretIndex[key.SecretHash] = key.ID
	return key, nil
}

func (s *MemoryStore) ListByUser(ctx context.Context, userID int64) ([]domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.ListByUser(ctx, userID)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	list := make([]domainapikey.APIKey, 0)
	for _, key := range s.keysByID {
		if key.UserID == userID && key.DeletedAt == nil {
			list = append(list, key)
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].CreatedAt.After(list[j].CreatedAt) })
	return list, nil
}

func (s *MemoryStore) GetByID(ctx context.Context, userID, id int64) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.GetByID(ctx, userID, id)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keysByID[id]
	if !ok || key.UserID != userID || key.DeletedAt != nil {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	return key, nil
}

func (s *MemoryStore) GetByAccessKey(ctx context.Context, accessKey string) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.GetByAccessKey(ctx, accessKey)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.accessIndex[accessKey]
	if !ok {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	key := s.keysByID[id]
	if key.DeletedAt != nil {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	return key, nil
}

func (s *MemoryStore) GetBySecretHash(ctx context.Context, secretHash string) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.GetBySecretHash(ctx, secretHash)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.secretIndex[secretHash]
	if !ok {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	key := s.keysByID[id]
	if key.DeletedAt != nil {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	return key, nil
}

func (s *MemoryStore) UpdateForUser(ctx context.Context, userID int64, key domainapikey.APIKey) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.UpdateForUser(ctx, userID, key)
	}
	return s.updateForUser(ctx, userID, key.ID, func(existing *domainapikey.APIKey) {
		existing.Name = key.Name
		existing.GroupCode = key.GroupCode
		existing.TotalQuotaPoints = cloneStringPtr(key.TotalQuotaPoints)
		existing.DailyQuotaPoints = cloneStringPtr(key.DailyQuotaPoints)
		existing.RPMLimit = cloneIntPtr(key.RPMLimit)
		existing.ExpiresAt = key.ExpiresAt
	})
}

func (s *MemoryStore) UpdateLastUsedAt(ctx context.Context, id int64, at time.Time) error {
	if s.delegate != nil {
		return s.delegate.UpdateLastUsedAt(ctx, id, at)
	}
	return s.update(ctx, id, func(key *domainapikey.APIKey) { key.LastUsedAt = &at })
}

func (s *MemoryStore) UpdateStatus(ctx context.Context, id int64, status string) error {
	if s.delegate != nil {
		return s.delegate.UpdateStatus(ctx, id, status)
	}
	return s.update(ctx, id, func(key *domainapikey.APIKey) { key.Status = status })
}

func (s *MemoryStore) UpdateStatusForUser(ctx context.Context, userID, id int64, status string) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.UpdateStatusForUser(ctx, userID, id, status)
	}
	return s.updateForUser(ctx, userID, id, func(key *domainapikey.APIKey) { key.Status = status })
}

func (s *MemoryStore) UpdateExpiresAt(ctx context.Context, id int64, expiresAt *time.Time) error {
	if s.delegate != nil {
		return s.delegate.UpdateExpiresAt(ctx, id, expiresAt)
	}
	return s.update(ctx, id, func(key *domainapikey.APIKey) { key.ExpiresAt = expiresAt })
}

func (s *MemoryStore) UpdateSecretForUser(ctx context.Context, userID, id int64, secretHash string, secretCiphertext string) (domainapikey.APIKey, error) {
	if s.delegate != nil {
		return s.delegate.UpdateSecretForUser(ctx, userID, id, secretHash, secretCiphertext)
	}
	return s.updateForUser(ctx, userID, id, func(key *domainapikey.APIKey) {
		delete(s.secretIndex, key.SecretHash)
		key.SecretHash = secretHash
		key.SecretCiphertext = secretCiphertext
		key.SigningSecret = secretCiphertext
		s.secretIndex[secretHash] = key.ID
	})
}

func (s *MemoryStore) DeleteForUser(ctx context.Context, userID, id int64, at time.Time) error {
	if s.delegate != nil {
		return s.delegate.DeleteForUser(ctx, userID, id, at)
	}
	_, err := s.updateForUser(ctx, userID, id, func(key *domainapikey.APIKey) {
		key.Status = domainapikey.StatusRevoked
		key.DeletedAt = &at
		delete(s.accessIndex, key.AccessKey)
		delete(s.secretIndex, key.SecretHash)
	})
	return err
}

func (s *MemoryStore) RecordRequest(ctx context.Context, id int64, rpmLimit int, at time.Time) error {
	if s.delegate != nil {
		return s.delegate.RecordRequest(ctx, id, rpmLimit, at)
	}
	if rpmLimit <= 0 {
		return nil
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keysByID[id]
	if !ok || key.DeletedAt != nil {
		return repoerr.ErrNotFound
	}
	if key.RPMWindowStartedAt == nil || !at.Before(key.RPMWindowStartedAt.Add(time.Minute)) {
		key.RPMWindowStartedAt = &at
		key.RPMWindowCount = 1
	} else {
		if key.RPMWindowCount >= rpmLimit {
			return errs.New(http.StatusTooManyRequests, errs.CodeRateLimited, "api key rate limit exceeded")
		}
		key.RPMWindowCount++
	}
	key.UpdatedAt = time.Now().UTC()
	s.keysByID[id] = key
	return nil
}

func (s *MemoryStore) ReserveQuota(ctx context.Context, userID, id int64, reservationID, points string, at time.Time) error {
	if s.delegate != nil {
		return s.delegate.ReserveQuota(ctx, userID, id, reservationID, points, at)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keysByID[id]
	if !ok || key.UserID != userID || key.DeletedAt != nil {
		return repoerr.ErrNotFound
	}
	reservationKey := quotaReservationKey(id, reservationID)
	if existing, ok := s.quotaReservations[reservationKey]; ok {
		parsedPoints, err := decimal.NewFromString(points)
		if err != nil {
			return err
		}
		if existing.KeyID == id && existing.Points.Equal(parsedPoints) && existing.Status == "active" {
			return nil
		}
		if existing.KeyID == id && existing.Points.Equal(parsedPoints) && existing.Status == "released" {
			return s.reserveQuotaLocked(id, key, reservationKey, existing, parsedPoints, at)
		}
		return errs.New(http.StatusConflict, errs.CodeConflict, "quota reservation conflict")
	}
	parsedPoints, err := decimal.NewFromString(points)
	if err != nil {
		return err
	}
	return s.reserveQuotaLocked(id, key, reservationKey, quotaReservation{KeyID: id, Points: parsedPoints}, parsedPoints, at)
}

func (s *MemoryStore) reserveQuotaLocked(id int64, key domainapikey.APIKey, reservationKey string, reservation quotaReservation, parsedPoints decimal.Decimal, at time.Time) error {
	day := at.UTC().Format("2006-01-02")
	totalUsed, _ := decimal.NewFromString(defaultString(key.TotalQuotaUsedPoints, zeroPoints))
	dailyUsed, _ := decimal.NewFromString(defaultString(key.DailyQuotaUsedPoints, zeroPoints))
	if key.QuotaUsageDay == nil || *key.QuotaUsageDay != day {
		dailyUsed = decimal.Zero
	}
	if key.TotalQuotaPoints != nil {
		totalLimit, err := decimal.NewFromString(*key.TotalQuotaPoints)
		if err != nil {
			return err
		}
		if totalUsed.Add(parsedPoints).GreaterThan(totalLimit) {
			return errs.New(http.StatusForbidden, errs.CodeInsufficientPoints, "api key total quota exceeded")
		}
	}
	if key.DailyQuotaPoints != nil {
		dailyLimit, err := decimal.NewFromString(*key.DailyQuotaPoints)
		if err != nil {
			return err
		}
		if dailyUsed.Add(parsedPoints).GreaterThan(dailyLimit) {
			return errs.New(http.StatusForbidden, errs.CodeInsufficientPoints, "api key daily quota exceeded")
		}
	}
	key.TotalQuotaUsedPoints = normalizePoints(totalUsed.Add(parsedPoints))
	key.DailyQuotaUsedPoints = normalizePoints(dailyUsed.Add(parsedPoints))
	key.QuotaUsageDay = &day
	key.UpdatedAt = time.Now().UTC()
	s.keysByID[id] = key
	reservation.KeyID = id
	reservation.Day = day
	reservation.Points = parsedPoints
	reservation.Status = "active"
	s.quotaReservations[reservationKey] = reservation
	return nil
}

func (s *MemoryStore) ReleaseQuota(ctx context.Context, id int64, reservationID string) error {
	if s.delegate != nil {
		return s.delegate.ReleaseQuota(ctx, id, reservationID)
	}
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	reservationKey := quotaReservationKey(id, reservationID)
	reservation, ok := s.quotaReservations[reservationKey]
	if !ok || reservation.Status != "active" {
		return nil
	}
	key, ok := s.keysByID[id]
	if !ok || key.DeletedAt != nil {
		return repoerr.ErrNotFound
	}
	totalUsed, _ := decimal.NewFromString(defaultString(key.TotalQuotaUsedPoints, zeroPoints))
	dailyUsed, _ := decimal.NewFromString(defaultString(key.DailyQuotaUsedPoints, zeroPoints))
	totalUsed = totalUsed.Sub(reservation.Points)
	dailyUsed = dailyUsed.Sub(reservation.Points)
	if totalUsed.IsNegative() {
		totalUsed = decimal.Zero
	}
	if dailyUsed.IsNegative() {
		dailyUsed = decimal.Zero
	}
	key.TotalQuotaUsedPoints = normalizePoints(totalUsed)
	if key.QuotaUsageDay != nil && *key.QuotaUsageDay == reservation.Day {
		key.DailyQuotaUsedPoints = normalizePoints(dailyUsed)
	}
	key.UpdatedAt = time.Now().UTC()
	s.keysByID[id] = key
	reservation.Status = "released"
	s.quotaReservations[reservationKey] = reservation
	return nil
}

func (s *MemoryStore) update(_ context.Context, id int64, fn func(*domainapikey.APIKey)) error {
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keysByID[id]
	if !ok {
		return repoerr.ErrNotFound
	}
	fn(&key)
	key.UpdatedAt = time.Now().UTC()
	s.keysByID[id] = key
	return nil
}

func (s *MemoryStore) updateForUser(_ context.Context, userID, id int64, fn func(*domainapikey.APIKey)) (domainapikey.APIKey, error) {
	s.ensure()
	s.mu.Lock()
	defer s.mu.Unlock()
	key, ok := s.keysByID[id]
	if !ok || key.UserID != userID || key.DeletedAt != nil {
		return domainapikey.APIKey{}, repoerr.ErrNotFound
	}
	fn(&key)
	key.UpdatedAt = time.Now().UTC()
	s.keysByID[id] = key
	return key, nil
}

func (s *MemoryStore) ensure() {
	if s.keysByID != nil {
		return
	}
	s.nextID = 1
	s.keysByID = map[int64]domainapikey.APIKey{}
	s.accessIndex = map[string]int64{}
	s.secretIndex = map[string]int64{}
	s.quotaReservations = map[string]quotaReservation{}
}
