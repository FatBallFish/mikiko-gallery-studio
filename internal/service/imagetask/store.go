package imagetask

import (
	"context"
	"fmt"
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
	UpdateProgressIfOwned(ctx context.Context, taskID, owner, stage, message string, now time.Time) error
	SaveTerminalState(ctx context.Context, task domainimagetask.Task, owner string, now time.Time) error
	GetByID(ctx context.Context, userID int64, taskID string) (domainimagetask.Task, error)
	GetImageResultByID(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error)
	GetImageResultForAdmin(ctx context.Context, imageID string) (provider.ImageResult, error)
	ListByUser(ctx context.Context, userID int64) ([]domainimagetask.Task, error)
	RequestPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error)
	CancelPublish(ctx context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error)
	SetImageGroup(ctx context.Context, userID int64, imageID, imageGroup string) (domainimagetask.GalleryImage, error)
	ReviewImage(ctx context.Context, imageID, nextStatus, reviewReason string, publishedAt *time.Time) (domainimagetask.GalleryImage, error)
	DeleteImageResult(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error)
	ListGalleryByUser(ctx context.Context, userID int64, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error)
	ListGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error)
	ListPublicGallery(ctx context.Context, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error)
	GetPublicImage(ctx context.Context, imageID string, viewerUserID int64) (domainimagetask.GalleryImage, error)
	SetPublicImageInteraction(ctx context.Context, userID int64, imageID, kind string, active bool) (domainimagetask.GalleryImage, error)
	DeleteByID(ctx context.Context, userID int64, taskID string) error
	AcquireNextQueuedTask(ctx context.Context, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error)
	RenewTaskLease(ctx context.Context, taskID, owner string, now time.Time, leaseTTL time.Duration) (domainimagetask.Task, error)
}

type MemoryStore struct {
	mu           sync.Mutex
	tasksByID    map[string]domainimagetask.Task
	publicStats  map[string]*memoryPublicStats
	interactions map[string]map[int64]*memoryPublicInteraction
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		tasksByID:    map[string]domainimagetask.Task{},
		publicStats:  map[string]*memoryPublicStats{},
		interactions: map[string]map[int64]*memoryPublicInteraction{},
	}
}

type memoryPublicStats struct {
	likes     int
	favorites int
	comments  int
}

type memoryPublicInteraction struct {
	liked     bool
	favorited bool
}

func (s *MemoryStore) Save(_ context.Context, task domainimagetask.Task) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	if current, ok := s.tasksByID[task.ID]; ok && !current.CreatedAt.IsZero() {
		task.CreatedAt = current.CreatedAt
	}
	if task.CreatedAt.IsZero() {
		task.CreatedAt = now
	}
	task.UpdatedAt = now
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
	task.CreatedAt = current.CreatedAt
	task.UpdatedAt = now.UTC()

	s.tasksByID[task.ID] = cloneTask(task)
	return nil
}

func (s *MemoryStore) UpdateProgressIfOwned(_ context.Context, taskID, owner, stage, message string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.tasksByID[taskID]
	if !ok {
		return repoerr.ErrNotFound
	}
	if current.Status != domainimagetask.StatusRunning || current.LeaseOwner != owner {
		return repoerr.ErrConflict
	}
	if current.LeaseExpiresAt != nil && current.LeaseExpiresAt.Before(now) {
		return repoerr.ErrConflict
	}
	current.ProgressStage = stage
	current.ProgressMessage = message
	current.UpdatedAt = now.UTC()
	s.tasksByID[taskID] = cloneTask(current)
	return nil
}

func (s *MemoryStore) SaveTerminalState(_ context.Context, task domainimagetask.Task, owner string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	current, ok := s.tasksByID[task.ID]
	if !ok {
		return repoerr.ErrNotFound
	}
	if current.Status != domainimagetask.StatusRunning || current.LeaseOwner != owner {
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
	task.CreatedAt = current.CreatedAt
	task.UpdatedAt = now.UTC()

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

func (s *MemoryStore) GetImageResultForAdmin(_ context.Context, imageID string) (provider.ImageResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, task := range s.tasksByID {
		if task.Status == domainimagetask.StatusDeleted {
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

func (s *MemoryStore) CancelPublish(_ context.Context, userID int64, imageID string) (domainimagetask.GalleryImage, error) {
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
			switch defaultVisibilityStatus(result.VisibilityStatus) {
			case domainimagetask.VisibilityPrivate, domainimagetask.VisibilityPendingReview, domainimagetask.VisibilityApproved:
				result.VisibilityStatus = domainimagetask.VisibilityPrivate
				result.ReviewReason = ""
				result.PublishedAt = nil
				task.Results[idx] = result
				s.tasksByID[taskID] = cloneTask(task)
				return galleryImageFromMemoryTask(task, result), nil
			default:
				return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
			}
		}
	}
	return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
}

func (s *MemoryStore) SetImageGroup(_ context.Context, userID int64, imageID, imageGroup string) (domainimagetask.GalleryImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	imageGroup = strings.TrimSpace(imageGroup)
	for taskID, task := range s.tasksByID {
		if task.UserID != userID || task.Status == domainimagetask.StatusDeleted {
			continue
		}
		for idx, result := range task.Results {
			if result.ID != imageID {
				continue
			}
			result.ImageGroup = imageGroup
			task.Results[idx] = result
			s.tasksByID[taskID] = cloneTask(task)
			return galleryImageFromMemoryTask(task, result), nil
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

func (s *MemoryStore) DeleteImageResult(_ context.Context, userID int64, imageID string) (provider.ImageResult, error) {
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
			task.Results = append(task.Results[:idx], task.Results[idx+1:]...)
			s.tasksByID[taskID] = cloneTask(task)
			return result, nil
		}
	}
	return provider.ImageResult{}, repoerr.ErrNotFound
}

func (s *MemoryStore) ListGalleryByUser(_ context.Context, userID int64, req domainimagetask.GalleryListRequest) (domainimagetask.GalleryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page, pageSize := normalizeGalleryPage(req.Page, req.PageSize)
	items := make([]domainimagetask.GalleryImage, 0)
	for _, task := range s.tasksByID {
		if task.UserID != userID || task.Status == domainimagetask.StatusDeleted {
			continue
		}
		for _, result := range task.Results {
			image := galleryImageFromMemoryTask(task, result)
			if req.ReviewOnly && image.VisibilityStatus == domainimagetask.VisibilityPrivate {
				continue
			}
			if req.Status != "" && !strings.EqualFold(image.VisibilityStatus, req.Status) {
				continue
			}
			items = append(items, image)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})
	return sliceGalleryPage(items, page, pageSize), nil
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
			if req.ReviewOnly && image.VisibilityStatus == domainimagetask.VisibilityPrivate {
				continue
			}
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
			image := s.decoratePublicImage(galleryImageFromMemoryTask(task, result), req.ViewerUserID)
			if req.Query != "" && !publicGalleryQueryMatches(image, req.Query) {
				continue
			}
			if req.RouteModelCode != "" && !publicGalleryRouteModelMatches(image, req.RouteModelCode) {
				continue
			}
			if req.TaskType != "" && !strings.EqualFold(image.TaskType, req.TaskType) {
				continue
			}
			if req.LikedOnly && !image.LikedByViewer {
				continue
			}
			if req.FavoritedOnly && !image.FavoritedByViewer {
				continue
			}
			items = append(items, image)
		}
	}
	sort.SliceStable(items, func(i, j int) bool {
		if req.Sort == "hot" {
			left := publicGalleryHotScore(items[i])
			right := publicGalleryHotScore(items[j])
			if left != right {
				return left > right
			}
		}
		return items[i].ID > items[j].ID
	})
	return sliceGalleryPage(items, page, pageSize), nil
}

func publicGalleryRouteModelMatches(image domainimagetask.GalleryImage, routeModelCode string) bool {
	routeModelCode = strings.TrimSpace(routeModelCode)
	if routeModelCode == "" {
		return true
	}
	return strings.EqualFold(image.RouteModelCode, routeModelCode) || (image.RouteModelCode == "" && strings.EqualFold(image.AbstractModel, routeModelCode))
}

func publicGalleryQueryMatches(image domainimagetask.GalleryImage, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	fields := []string{
		image.ID,
		image.PromptExcerpt,
		publicGallerySearchExcerpt(image.Prompt, 24),
		image.RouteModelCode,
		image.AbstractModel,
		image.TaskType,
		image.AuthorName,
	}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(strings.TrimSpace(field)), query) {
			return true
		}
	}
	return false
}

func publicGallerySearchExcerpt(prompt string, limit int) string {
	prompt = strings.Join(strings.Fields(prompt), " ")
	if limit <= 0 || prompt == "" {
		return ""
	}
	runes := []rune(prompt)
	if len(runes) <= limit {
		visible := len(runes) / 2
		if visible < 1 {
			return "…"
		}
		if visible > limit-1 {
			visible = limit - 1
		}
		return string(runes[:visible]) + "…"
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func (s *MemoryStore) GetPublicImage(_ context.Context, imageID string, viewerUserID int64) (domainimagetask.GalleryImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, task := range s.tasksByID {
		if task.Status == domainimagetask.StatusDeleted {
			continue
		}
		for _, result := range task.Results {
			if result.ID == imageID && defaultVisibilityStatus(result.VisibilityStatus) == domainimagetask.VisibilityApproved {
				return s.decoratePublicImage(galleryImageFromMemoryTask(task, result), viewerUserID), nil
			}
		}
	}
	return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
}

func (s *MemoryStore) SetPublicImageInteraction(_ context.Context, userID int64, imageID, kind string, active bool) (domainimagetask.GalleryImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var found domainimagetask.GalleryImage
	for _, task := range s.tasksByID {
		if task.Status == domainimagetask.StatusDeleted {
			continue
		}
		for _, result := range task.Results {
			if result.ID == imageID && defaultVisibilityStatus(result.VisibilityStatus) == domainimagetask.VisibilityApproved {
				found = galleryImageFromMemoryTask(task, result)
			}
		}
	}
	if found.ID == "" {
		return domainimagetask.GalleryImage{}, repoerr.ErrNotFound
	}
	if s.interactions[imageID] == nil {
		s.interactions[imageID] = map[int64]*memoryPublicInteraction{}
	}
	if s.publicStats[imageID] == nil {
		s.publicStats[imageID] = &memoryPublicStats{}
	}
	interaction := s.interactions[imageID][userID]
	if interaction == nil {
		interaction = &memoryPublicInteraction{}
		s.interactions[imageID][userID] = interaction
	}
	stats := s.publicStats[imageID]
	switch kind {
	case "like":
		if interaction.liked != active {
			if active {
				stats.likes++
			} else if stats.likes > 0 {
				stats.likes--
			}
			interaction.liked = active
		}
	case "favorite":
		if interaction.favorited != active {
			if active {
				stats.favorites++
			} else if stats.favorites > 0 {
				stats.favorites--
			}
			interaction.favorited = active
		}
	}
	return s.decoratePublicImage(found, userID), nil
}

func (s *MemoryStore) decoratePublicImage(image domainimagetask.GalleryImage, viewerUserID int64) domainimagetask.GalleryImage {
	stats := s.publicStats[image.ID]
	if stats != nil {
		image.LikeCount = stats.likes
		image.FavoriteCount = stats.favorites
	}
	image.AuthorName = fmt.Sprintf("user-%d", image.UserID)
	if viewerUserID > 0 {
		if interaction := s.interactions[image.ID][viewerUserID]; interaction != nil {
			image.LikedByViewer = interaction.liked
			image.FavoritedByViewer = interaction.favorited
		}
	}
	return image
}

func publicGalleryHotScore(image domainimagetask.GalleryImage) int {
	return image.LikeCount*2 + image.FavoriteCount*3
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
		task.ProgressStage = domainimagetask.ProgressStageProvider
		task.ProgressMessage = "正在调用模型生成图片"
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
		return task.ArtifactRecovery.Status != artifactRecoveryPending || task.ArtifactRecovery.NextRetryAt == nil || !task.ArtifactRecovery.NextRetryAt.After(now)
	case domainimagetask.StatusRunning:
		return task.LeaseExpiresAt == nil || task.LeaseExpiresAt.Before(now)
	default:
		return false
	}
}

func galleryImageFromMemoryTask(task domainimagetask.Task, result provider.ImageResult) domainimagetask.GalleryImage {
	return domainimagetask.GalleryImage{
		ID:                result.ID,
		TaskID:            task.ID,
		UserID:            task.UserID,
		Prompt:            task.Prompt,
		AbstractModel:     task.AbstractModel,
		RouteModelCode:    task.RouteModelCode,
		TaskType:          task.TaskType,
		TaskStatus:        task.Status,
		SizeMode:          task.SizeMode,
		RequestedSize:     task.RequestedSize,
		BaseResolution:    task.BaseResolution,
		Quality:           task.Quality,
		AspectRatio:       task.AspectRatio,
		OutputFormat:      task.OutputFormat,
		OutputCompression: task.OutputCompression,
		Moderation:        task.Moderation,
		OutputImageCount:  task.OutputImageCount,
		ActualPoints:      task.ActualPoints,
		ReferenceAssetIDs: append([]string(nil), task.ReferenceAssetIDs...),
		ReferenceAssets:   galleryReferenceAssets(task.ReferenceAssetIDs),
		URL:               result.URL,
		DownloadURL:       result.DownloadURL,
		MimeType:          result.MimeType,
		FileSizeBytes:     result.FileSizeBytes,
		Width:             result.Width,
		Height:            result.Height,
		SHA256:            result.SHA256,
		ObjectKey:         result.ObjectKey,
		StorageDriver:     result.StorageDriver,
		StorageConfigID:   result.StorageConfigID,
		ImageGroup:        result.ImageGroup,
		VisibilityStatus:  defaultVisibilityStatus(result.VisibilityStatus),
		ReviewReason:      result.ReviewReason,
		PublishedAt:       result.PublishedAt,
	}
}

func galleryReferenceAssets(assetIDs []string) []domainimagetask.GalleryReferenceAsset {
	assets := make([]domainimagetask.GalleryReferenceAsset, 0, len(assetIDs))
	for _, id := range assetIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		assets = append(assets, domainimagetask.GalleryReferenceAsset{
			ID:         id,
			Name:       "reference",
			PreviewURL: "/api/agent/image/v1/reference-assets/" + id + "/download",
		})
	}
	return assets
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
