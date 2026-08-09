package imagetask

import (
	"context"
	"errors"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
)

func TestBatchGroupUsesExplicitProjectScopedIDsAndReturnsPartialResult(t *testing.T) {
	store := NewMemoryStore()
	seedGalleryBatchTask(t, store, domainimagetask.Task{
		ID: "task-1", UserID: 7, ProjectID: "project-1", Status: domainimagetask.StatusSucceeded,
		TaskType: "text_to_image", AbstractModel: "plus", Results: []provider.ImageResult{{ID: "image-1", ProjectID: "project-1"}, {ID: "image-2", ProjectID: "project-1"}},
	})
	seedGalleryBatchTask(t, store, domainimagetask.Task{
		ID: "task-2", UserID: 7, ProjectID: "project-2", Status: domainimagetask.StatusSucceeded,
		TaskType: "text_to_image", AbstractModel: "plus", Results: []provider.ImageResult{{ID: "foreign-project", ProjectID: "project-2"}},
	})
	service := NewServiceWithStoreAssetsAndBilling(config.Config{}, store, nil, nil)

	result, err := service.BatchSetImageGroup(context.Background(), 7, "project-1", []string{"image-1", "image-1", "foreign-project", "missing"}, "客户素材")
	if err != nil {
		t.Fatalf("batch group: %v", err)
	}
	if len(result.Succeeded) != 1 || result.Succeeded[0].ID != "image-1" || result.Succeeded[0].Entity.ImageGroup != "客户素材" {
		t.Fatalf("batch group successes = %#v", result.Succeeded)
	}
	if len(result.Failed) != 2 || result.Failed[0].Code != "not_found" || result.Failed[1].Code != "not_found" {
		t.Fatalf("batch group failures = %#v", result.Failed)
	}
}

func TestProjectScopedGalleryMutationsRejectStaleSourceProject(t *testing.T) {
	store := NewMemoryStore()
	seedGalleryBatchTask(t, store, domainimagetask.Task{
		ID: "task-stale", UserID: 7, ProjectID: "source", Status: domainimagetask.StatusSucceeded,
		TaskType: "text_to_image", Prompt: "safe", Results: []provider.ImageResult{{ID: "image-stale", ProjectID: "source"}},
	})
	if _, err := store.TransferImageProject(t.Context(), 7, "image-stale", "source", "target"); err != nil {
		t.Fatal(err)
	}
	checks := []struct {
		name string
		run  func() error
	}{
		{"publish", func() error {
			_, err := store.RequestPublishInProject(t.Context(), 7, "image-stale", "source")
			return err
		}},
		{"cancel", func() error {
			_, err := store.CancelPublishInProject(t.Context(), 7, "image-stale", "source")
			return err
		}},
		{"group", func() error {
			_, err := store.SetImageGroupInProject(t.Context(), 7, "image-stale", "source", "stale")
			return err
		}},
		{"delete", func() error {
			_, err := store.DeleteImageResultInProject(t.Context(), 7, "image-stale", "source")
			return err
		}},
	}
	for _, check := range checks {
		if err := check.run(); !errors.Is(err, repoerr.ErrNotFound) {
			t.Errorf("%s stale-source error=%v, want opaque not found", check.name, err)
		}
	}
	if image, err := store.GetImageResultByID(t.Context(), 7, "image-stale"); err != nil || image.ProjectID != "target" {
		t.Fatalf("stale-source mutations changed moved image=%#v err=%v", image, err)
	}
}

func TestBatchMutationFailsWhenAssetMovesAfterAuthorizationRead(t *testing.T) {
	base := NewMemoryStore()
	seedGalleryBatchTask(t, base, domainimagetask.Task{
		ID: "task-race", UserID: 7, ProjectID: "source", Status: domainimagetask.StatusSucceeded,
		TaskType: "text_to_image", Results: []provider.ImageResult{{ID: "image-race", ProjectID: "source"}},
	})
	store := &moveAfterGalleryReadStore{MemoryStore: base}
	service := NewServiceWithStoreAssetsAndBilling(config.Config{}, store, nil, nil)
	result, err := service.BatchSetImageGroup(t.Context(), 7, "source", []string{"image-race"}, "must-not-apply")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Succeeded) != 0 || len(result.Failed) != 1 || result.Failed[0].Code != "not_found" {
		t.Fatalf("post-authorization move result=%#v", result)
	}
	image, err := base.GetImageResultByID(t.Context(), 7, "image-race")
	if err != nil || image.ProjectID != "target" || image.ImageGroup != "" {
		t.Fatalf("moved image was mutated=%#v err=%v", image, err)
	}
}

type moveAfterGalleryReadStore struct {
	*MemoryStore
	moved bool
}

func (s *moveAfterGalleryReadStore) GetImageResultByID(ctx context.Context, userID int64, imageID string) (provider.ImageResult, error) {
	result, err := s.MemoryStore.GetImageResultByID(ctx, userID, imageID)
	if err == nil && !s.moved {
		s.moved = true
		_, _ = s.MemoryStore.TransferImageProject(ctx, userID, imageID, "source", "target")
	}
	return result, err
}

func TestBatchDeleteKeepsFailuresAndRemovesOnlySuccessfulAssets(t *testing.T) {
	store := NewMemoryStore()
	seedGalleryBatchTask(t, store, domainimagetask.Task{
		ID: "task-delete", UserID: 7, ProjectID: "project-1", Status: domainimagetask.StatusSucceeded,
		TaskType: "text_to_image", AbstractModel: "plus", Results: []provider.ImageResult{{ID: "image-1", ProjectID: "project-1"}, {ID: "image-2", ProjectID: "project-1"}},
	})
	service := NewServiceWithStoreAssetsAndBilling(config.Config{}, store, nil, nil)
	result, err := service.BatchDeleteImages(context.Background(), 7, "project-1", []string{"image-1", "missing"})
	if err != nil {
		t.Fatalf("batch delete: %v", err)
	}
	if len(result.Succeeded) != 1 || result.Succeeded[0].ID != "image-1" || len(result.Failed) != 1 || result.Failed[0].ID != "missing" {
		t.Fatalf("batch delete result = %#v", result)
	}
	if _, err := store.GetImageResultByID(context.Background(), 7, "image-1"); err == nil {
		t.Fatal("successfully deleted image remains readable")
	}
	if _, err := store.GetImageResultByID(context.Background(), 7, "image-2"); err != nil {
		t.Fatalf("unselected image was deleted: %v", err)
	}
}

func seedGalleryBatchTask(t *testing.T, store *MemoryStore, task domainimagetask.Task) {
	t.Helper()
	if err := store.Save(context.Background(), task); err != nil {
		t.Fatalf("seed gallery batch task: %v", err)
	}
}
