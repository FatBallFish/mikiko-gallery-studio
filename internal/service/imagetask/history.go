package imagetask

import (
	"context"
	"errors"
	"fmt"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type historyStore interface {
	HideHistory(context.Context, int64, string) error
	ClearFailedHistory(context.Context, int64, string) (int, error)
}

func (s *Service) HideHistory(ctx context.Context, userID int64, id string) error {
	store, ok := s.store.(historyStore)
	if !ok {
		return errs.Internal("image history is unavailable")
	}
	if err := store.HideHistory(ctx, userID, id); err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			return errs.New(404, errs.CodeNotFound, "image task not found")
		}
		return fmt.Errorf("hide image history: %w", err)
	}
	return nil
}
func (s *Service) ClearFailedHistory(ctx context.Context, userID int64, project string) (int, error) {
	store, ok := s.store.(historyStore)
	if !ok {
		return 0, errs.Internal("image history is unavailable")
	}
	return store.ClearFailedHistory(ctx, userID, project)
}

func (s *MemoryStore) HideHistory(_ context.Context, userID int64, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	task, ok := s.tasksByID[id]
	if !ok || task.UserID != userID {
		return repoerr.ErrNotFound
	}
	switch task.Status {
	case "succeeded", "partial_failed", "failed", "rejected":
	default:
		return errs.New(409, errs.CodeConflict, "only terminal image tasks can be hidden")
	}
	if s.hiddenHistory == nil {
		s.hiddenHistory = map[string]bool{}
	}
	s.hiddenHistory[id] = true
	return nil
}
func (s *MemoryStore) ClearFailedHistory(_ context.Context, userID int64, project string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hiddenHistory == nil {
		s.hiddenHistory = map[string]bool{}
	}
	count := 0
	for id, task := range s.tasksByID {
		if task.UserID == userID && (project == "" || task.ProjectID == project) && task.Status == "failed" && len(task.Results) == 0 && !s.hiddenHistory[id] {
			s.hiddenHistory[id] = true
			count++
		}
	}
	return count, nil
}
