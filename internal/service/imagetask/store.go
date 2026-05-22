package imagetask

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

type Store interface {
	Save(ctx context.Context, task domainimagetask.Task) error
	SaveIfOwned(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error
	SaveTerminalState(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error
	GetByID(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error)
	GetImageResultByID(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error)
	ListByUser(ctx context.Context, userID int64) ([]domainimagetask.Task, error)
	DeleteByID(ctx context.Context, userID int64, taskID string) error
	AcquireNextQueuedTask(ctx context.Context, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error)
	RenewTaskLease(ctx context.Context, taskID, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error)
}

type MemoryStore struct {
	mu        sync.Mutex
	tasksByID map[string]domainimagetask.Task
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{tasksByID: map[string]domainimagetask.Task{}}
}

func (s *MemoryStore) Save(_ context.Context, task domainimagetask.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.tasksByID[task.ID] = cloneTask(task)
	return nil
}

func (s *MemoryStore) SaveIfOwned(_ context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.tasksByID[task.ID]
	if !ok {
		return repoerr.ErrNotFound
	}
	if current.Status != domainimagetask.StatusRunning {
		return repoerr.ErrConflict
	}
	if current.LeaseOwner != owner {
		return repoerr.ErrConflict
	}
	if current.LeaseExpiresAt != nil && current.LeaseExpiresAt.Before(now) {
		return repoerr.ErrConflict
	}
	if task.Status == domainimagetask.StatusRunning {
		task.LeaseOwner = current.LeaseOwner
		if current.LeaseExpiresAt != nil {
			expiresAt := *current.LeaseExpiresAt
			task.LeaseExpiresAt = &expiresAt
		} else {
			task.LeaseExpiresAt = nil
		}
	}

	s.tasksByID[task.ID] = cloneTask(task)
	return nil
}

func (s *MemoryStore) SaveTerminalState(_ context.Context, task domainimagetask.Task, _ string, _ time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.tasksByID[task.ID]
	if !ok {
		return repoerr.ErrNotFound
	}
	if current.Status != domainimagetask.StatusRunning {
		return repoerr.ErrConflict
	}
	if task.Status == domainimagetask.StatusRunning {
		task.LeaseOwner = current.LeaseOwner
		if current.LeaseExpiresAt != nil {
			expiresAt := *current.LeaseExpiresAt
			task.LeaseExpiresAt = &expiresAt
		} else {
			task.LeaseExpiresAt = nil
		}
	}

	s.tasksByID[task.ID] = cloneTask(task)
	return nil
}

func (s *MemoryStore) GetByID(_ context.Context, userID int64, taskID string) (domainimagetask.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasksByID[taskID]
	if !ok || task.UserID != userID {
		return domainimagetask.Task{}, repoerr.ErrNotFound
	}
	return cloneTask(task), nil
}

func (s *MemoryStore) GetImageResultByID(_ context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, task := range s.tasksByID {
		if task.UserID != userID || task.Status == domainimagetask.StatusDeleted {
			continue
		}
		for _, result := range task.Results {
			if result.ID == imageID && strings.TrimSpace(result.VisibilityStatus) != "deleted" {
				return result, nil
			}
		}
	}
	return provider.ImageResult{}, repoerr.ErrNotFound
}

func (s *MemoryStore) ListByUser(_ context.Context, userID int64) ([]domainimagetask.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	list := make([]domainimagetask.Task, 0, len(s.tasksByID))
	for _, task := range s.tasksByID {
		if task.UserID != userID {
			continue
		}
		list = append(list, cloneTask(task))
	}
	return list, nil
}

func (s *MemoryStore) DeleteByID(_ context.Context, userID int64, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasksByID[taskID]
	if !ok || task.UserID != userID {
		return repoerr.ErrNotFound
	}
	delete(s.tasksByID, taskID)
	return nil
}

func (s *MemoryStore) AcquireNextQueuedTask(_ context.Context, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := make([]string, 0, len(s.tasksByID))
	for id := range s.tasksByID {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	for _, id := range ids {
		task := s.tasksByID[id]
		if !taskEligibleForLease(task, now) {
			continue
		}
		task.Status = domainimagetask.StatusRunning
		task.LeaseOwner = owner
		expiresAt := now.Add(leaseTTL)
		task.LeaseExpiresAt = &expiresAt
		s.tasksByID[id] = cloneTask(task)
		return cloneTask(task), nil
	}
	return domainimagetask.Task{}, repoerr.ErrNotFound
}

func (s *MemoryStore) RenewTaskLease(_ context.Context, taskID, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	task, ok := s.tasksByID[taskID]
	if !ok {
		return domainimagetask.Task{}, repoerr.ErrNotFound
	}
	if task.Status != domainimagetask.StatusRunning {
		return domainimagetask.Task{}, repoerr.ErrConflict
	}
	if task.LeaseOwner != owner {
		return domainimagetask.Task{}, repoerr.ErrConflict
	}
	if task.LeaseExpiresAt != nil && task.LeaseExpiresAt.Before(now) {
		return domainimagetask.Task{}, repoerr.ErrConflict
	}

	expiresAt := now.Add(leaseTTL)
	task.LeaseExpiresAt = &expiresAt
	s.tasksByID[taskID] = cloneTask(task)
	return cloneTask(task), nil
}

func taskEligibleForLease(task domainimagetask.Task, now time.Time) bool {
	switch task.Status {
	case domainimagetask.StatusQueued:
		return true
	case domainimagetask.StatusRunning:
		return task.LeaseExpiresAt == nil || task.LeaseExpiresAt.Before(now)
	default:
		return false
	}
}
