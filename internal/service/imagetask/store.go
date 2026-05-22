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
	RequestPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error)
	ReviewImage(ctx context.Context, imageID, nextStatus, reviewReason string, publishedAt *time.Time) (domainimagetask.GalleryImage, error)
	ListGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error)
	ListPublicGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error)
	GetPublicImage(ctx context.Context, imageID string) (domainimagetask.GalleryImage, error)
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

func (s *MemoryStore) RequestPublish(_ context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for taskID, task := range s.tasksByID {
		if task.UserID != userID || task.Status == domainimagetask.StatusDeleted {
			continue
		}
		for idx, result := range task.Results {
			if result.ID != imageID {
				continue
			}
			status := defaultVisibilityStatus(result.VisibilityStatus)
			switch status {
			case domainimagetask.VisibilityPrivate, domainimagetask.VisibilityRejected, domainimagetask.VisibilityUnpublished:
				result.VisibilityStatus = domainimagetask.VisibilityPendingReview
				task.Results[idx] = result
				s.tasksByID[taskID] = cloneTask(task)
				return galleryImageFromMemoryTask(task, result), nil
			case domainimagetask.VisibilityPendingReview, domainimagetask.VisibilityApproved:
				return galleryImageFromMemoryTask(task, result), nil
			default:
				result.VisibilityStatus = domainimagetask.VisibilityPendingReview
				task.Results[idx] = result
				s.tasksByID[taskID] = cloneTask(task)
				return galleryImageFromMemoryTask(task, result), nil
			}
		}
	}
	return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
}

func (s *MemoryStore) ReviewImage(_ context.Context, imageID, nextStatus, reviewReason string, publishedAt *time.Time) (domainimagetask.GalleryImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for taskID, task := range s.tasksByID {
		for idx, result := range task.Results {
			if result.ID != imageID {
				continue
			}
			result.VisibilityStatus = nextStatus
			result.ReviewReason = strings.TrimSpace(reviewReason)
			result.PublishedAt = nil
			if publishedAt != nil {
				copyValue := *publishedAt
				result.PublishedAt = &copyValue
			}
			task.Results[idx] = result
			s.tasksByID[taskID] = cloneTask(task)
			image := galleryImageFromMemoryTask(task, result)
			image.ReviewReason = result.ReviewReason
			image.PublishedAt = result.PublishedAt
			return image, nil
		}
	}
	return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
}

func (s *MemoryStore) ListGallery(_ context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page, pageSize := normalizeGalleryPage(req.Page, req.PageSize)
	items := make([]domainimagetask.GalleryImage, 0)
	for _, task := range s.tasksByID {
		if task.Status == domainimagetask.StatusDeleted {
			continue
		}
		for _, result := range task.Results {
			image := galleryImageFromMemoryTask(task, result)
			if req.Status != "" && !strings.EqualFold(image.VisibilityStatus, req.Status) {
				continue
			}
			items = append(items, image)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ID > items[j].ID
	})
	return sliceGalleryPage(items, page, pageSize), nil
}

func (s *MemoryStore) ListPublicGallery(_ context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page, pageSize := normalizeGalleryPage(req.Page, req.PageSize)
	items := make([]domainimagetask.GalleryImage, 0)
	for _, task := range s.tasksByID {
		if task.Status == domainimagetask.StatusDeleted {
			continue
		}
		for _, result := range task.Results {
			if defaultVisibilityStatus(result.VisibilityStatus) != domainimagetask.VisibilityApproved {
				continue
			}
			items = append(items, galleryImageFromMemoryTask(task, result))
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].ID > items[j].ID
	})
	return sliceGalleryPage(items, page, pageSize), nil
}

func (s *MemoryStore) GetPublicImage(_ context.Context, imageID string) (domainimagetask.GalleryImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, task := range s.tasksByID {
		if task.Status == domainimagetask.StatusDeleted {
			continue
		}
		for _, result := range task.Results {
			if result.ID == imageID && defaultVisibilityStatus(result.VisibilityStatus) == domainimagetask.VisibilityApproved {
				return galleryImageFromMemoryTask(task, result), nil
			}
		}
	}
	return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
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

func galleryImageFromMemoryTask(task domainimagetask.Task, result provider.ImageResult) domainimagetask.GalleryImage {
	return domainimagetask.GalleryImage{
		ID:               result.ID,
		TaskID:           task.ID,
		UserID:           task.UserID,
		Prompt:           task.Prompt,
		AbstractModel:    task.AbstractModel,
		TaskType:         task.TaskType,
		URL:              result.URL,
		DownloadURL:      result.DownloadURL,
		MimeType:         result.MimeType,
		FileSizeBytes:    result.FileSizeBytes,
		Width:            result.Width,
		Height:           result.Height,
		SHA256:           result.SHA256,
		ObjectKey:        result.ObjectKey,
		StorageDriver:    result.StorageDriver,
		VisibilityStatus: defaultVisibilityStatus(result.VisibilityStatus),
		ReviewReason:     result.ReviewReason,
		PublishedAt:      result.PublishedAt,
	}
}

func defaultVisibilityStatus(value string) string {
	if strings.TrimSpace(value) == "" {
		return domainimagetask.VisibilityPrivate
	}
	return value
}

func normalizeGalleryPage(page, pageSize int) (int, int) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}

func sliceGalleryPage(items []domainimagetask.GalleryImage, page, pageSize int) domainimagetask.GalleryPage {
	total := len(items)
	start := (page - 1) * pageSize
	if start >= total {
		return domainimagetask.GalleryPage{Items: []domainimagetask.GalleryImage{}, Page: page, PageSize: pageSize, Total: total}
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	return domainimagetask.GalleryPage{
		Items:    items[start:end],
		Page:     page,
		PageSize: pageSize,
		Total:    total,
	}
}
