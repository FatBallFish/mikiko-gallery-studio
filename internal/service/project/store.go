package project

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"time"

	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	"github.com/google/uuid"
)

type MemoryStore struct {
	mu          sync.Mutex
	projects    map[string]domainproject.Project
	idempotency map[string]string
	deletions   map[string]memoryDeleteReplay
	ownership   map[string]domainproject.OwnershipCounts
}

type memoryDeleteReplay struct {
	projectID string
	result    domainproject.DeleteResult
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		projects: map[string]domainproject.Project{}, idempotency: map[string]string{}, deletions: map[string]memoryDeleteReplay{}, ownership: map[string]domainproject.OwnershipCounts{},
	}
}

func (s *MemoryStore) EnsureDefault(_ context.Context, userID int64) (domainproject.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, item := range s.projects {
		if item.UserID == userID && item.IsDefault && item.Status == domainproject.StatusActive {
			return s.withCounts(item), nil
		}
	}
	now := time.Now().UTC()
	item := domainproject.Project{ID: uuid.NewString(), UserID: userID, Name: domainproject.DefaultName, NameKey: domainproject.DefaultName, IsDefault: true, Status: domainproject.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	s.projects[item.ID] = item
	return item, nil
}

func (s *MemoryStore) List(_ context.Context, userID int64) ([]domainproject.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	items := make([]domainproject.Project, 0)
	for _, item := range s.projects {
		if item.UserID == userID && item.Status == domainproject.StatusActive {
			items = append(items, s.withCounts(item))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsDefault != items[j].IsDefault {
			return items[i].IsDefault
		}
		return items[i].CreatedAt.Before(items[j].CreatedAt)
	})
	return items, nil
}

func (s *MemoryStore) Get(_ context.Context, userID int64, projectID string) (domainproject.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.projects[projectID]
	if !ok || item.UserID != userID || item.Status != domainproject.StatusActive {
		return domainproject.Project{}, ErrNotFound
	}
	return s.withCounts(item), nil
}

func (s *MemoryStore) Create(_ context.Context, userID int64, name, nameKey, idempotencyKey string) (domainproject.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if idempotencyKey != "" {
		if projectID := s.idempotency[idempotencyStorageKey(userID, idempotencyKey)]; projectID != "" {
			item, ok := s.projects[projectID]
			if ok && item.Status == domainproject.StatusActive {
				return s.withCounts(item), nil
			}
		}
	}
	for _, item := range s.projects {
		if item.UserID == userID && item.Status == domainproject.StatusActive && item.NameKey == nameKey {
			return domainproject.Project{}, ErrNameConflict
		}
	}
	now := time.Now().UTC()
	item := domainproject.Project{ID: uuid.NewString(), UserID: userID, Name: name, NameKey: nameKey, Status: domainproject.StatusActive, Version: 1, CreatedAt: now, UpdatedAt: now}
	s.projects[item.ID] = item
	if idempotencyKey != "" {
		s.idempotency[idempotencyStorageKey(userID, idempotencyKey)] = item.ID
	}
	return item, nil
}

func (s *MemoryStore) Rename(_ context.Context, userID int64, projectID, name, nameKey string, expectedVersion int64) (domainproject.Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.projects[projectID]
	if !ok || item.UserID != userID || item.Status != domainproject.StatusActive {
		return domainproject.Project{}, ErrNotFound
	}
	if item.Version != expectedVersion {
		return domainproject.Project{}, ErrProjectChanged
	}
	for _, other := range s.projects {
		if other.ID != item.ID && other.UserID == userID && other.Status == domainproject.StatusActive && other.NameKey == nameKey {
			return domainproject.Project{}, ErrNameConflict
		}
	}
	item.Name, item.NameKey, item.Version, item.UpdatedAt = name, nameKey, item.Version+1, time.Now().UTC()
	s.projects[item.ID] = item
	return s.withCounts(item), nil
}

func (s *MemoryStore) Delete(_ context.Context, userID int64, projectID string, req domainproject.DeleteRequest) (domainproject.DeleteResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.IdempotencyKey != "" {
		if replay, ok := s.deletions[idempotencyStorageKey(userID, req.IdempotencyKey)]; ok {
			if replay.projectID != projectID {
				return domainproject.DeleteResult{}, ErrIdempotencyConflict
			}
			return replay.result, nil
		}
	}
	item, ok := s.projects[projectID]
	if !ok || item.UserID != userID || item.Status != domainproject.StatusActive {
		return domainproject.DeleteResult{}, ErrNotFound
	}
	if item.IsDefault {
		return domainproject.DeleteResult{}, ErrDefaultImmutable
	}
	if item.Version != req.ExpectedVersion {
		return domainproject.DeleteResult{}, ErrProjectChanged
	}
	counts := s.ownership[ownershipStorageKey(userID, projectID)]
	if counts.Tasks+counts.Assets > 0 && req.TargetProjectID == "" {
		return domainproject.DeleteResult{}, &NonEmptyError{Counts: counts}
	}
	if req.TargetProjectID != "" {
		target, ok := s.projects[req.TargetProjectID]
		if !ok || target.UserID != userID || target.Status != domainproject.StatusActive {
			return domainproject.DeleteResult{}, ErrNotFound
		}
		targetKey := ownershipStorageKey(userID, req.TargetProjectID)
		targetCounts := s.ownership[targetKey]
		targetCounts.Tasks += counts.Tasks
		targetCounts.Assets += counts.Assets
		s.ownership[targetKey] = targetCounts
	}
	delete(s.ownership, ownershipStorageKey(userID, projectID))
	now := time.Now().UTC()
	item.Status, item.Version, item.UpdatedAt = domainproject.StatusDeleted, item.Version+1, now
	s.projects[item.ID] = item
	result := domainproject.DeleteResult{Project: item, Transferred: counts}
	if req.IdempotencyKey != "" {
		s.deletions[idempotencyStorageKey(userID, req.IdempotencyKey)] = memoryDeleteReplay{projectID: projectID, result: result}
	}
	return result, nil
}

func (s *MemoryStore) SeedOwnedRecords(userID int64, projectID string, tasks, assets int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ownership[ownershipStorageKey(userID, projectID)] = domainproject.OwnershipCounts{Tasks: tasks, Assets: assets}
}

func (s *MemoryStore) CountOwnedRecords(userID int64, projectID string) domainproject.OwnershipCounts {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ownership[ownershipStorageKey(userID, projectID)]
}

func (s *MemoryStore) withCounts(item domainproject.Project) domainproject.Project {
	counts := s.ownership[ownershipStorageKey(item.UserID, item.ID)]
	item.TaskCount, item.AssetCount = counts.Tasks, counts.Assets
	return item
}

func ownershipStorageKey(userID int64, projectID string) string {
	return strconv.FormatInt(userID, 10) + ":" + projectID
}
func idempotencyStorageKey(userID int64, key string) string {
	return strconv.FormatInt(userID, 10) + ":" + key
}
