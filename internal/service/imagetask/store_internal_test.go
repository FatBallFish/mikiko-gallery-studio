package imagetask

import (
	"context"
	"testing"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

func TestMemoryStorePublicGalleryHotSortIgnoresLegacyCommentCount(t *testing.T) {
	store := NewMemoryStore()
	ctx := context.Background()

	likedImageID := "liked-image"
	legacyCommentImageID := "legacy-comment-image"
	if err := store.Save(ctx, publicGalleryTask("liked-task", likedImageID)); err != nil {
		t.Fatalf("Save liked task: %v", err)
	}
	if err := store.Save(ctx, publicGalleryTask("legacy-comment-task", legacyCommentImageID)); err != nil {
		t.Fatalf("Save legacy comment task: %v", err)
	}
	if _, err := store.ReviewImage(ctx, likedImageID, domainimagetask.VisibilityApproved, "", nil); err != nil {
		t.Fatalf("ReviewImage liked: %v", err)
	}
	if _, err := store.ReviewImage(ctx, legacyCommentImageID, domainimagetask.VisibilityApproved, "", nil); err != nil {
		t.Fatalf("ReviewImage legacy comment: %v", err)
	}
	if _, err := store.SetPublicImageInteraction(ctx, 91, likedImageID, "favorite", true); err != nil {
		t.Fatalf("SetPublicImageInteraction favorite: %v", err)
	}

	store.publicStats[legacyCommentImageID] = &memoryPublicStats{comments: 99}

	page, err := store.ListPublicGallery(ctx, domainimagetask.GalleryListRequest{Page: 1, PageSize: 10, Sort: "hot"})
	if err != nil {
		t.Fatalf("ListPublicGallery hot: %v", err)
	}
	if len(page.Items) < 2 {
		t.Fatalf("expected two public images, got %#v", page)
	}
	if page.Items[0].ID != likedImageID {
		t.Fatalf("hot sort should ignore legacy comment_count and rank favorite first, got %#v", page.Items)
	}
}

func publicGalleryTask(taskID, imageID string) domainimagetask.Task {
	return domainimagetask.Task{
		UserID:                91,
		ID:                    taskID,
		Status:                domainimagetask.StatusSucceeded,
		Provider:              "openai",
		AbstractModel:         "basic",
		TaskType:              string(provider.TaskTypeTextToImage),
		Prompt:                "public gallery hot sort",
		RequestedQuality:      "auto",
		ResolvedQualityBucket: "1k",
		OutputImageCount:      1,
		Results: []provider.ImageResult{{
			ID:               imageID,
			URL:              "https://cdn.example.com/" + imageID + ".png",
			VisibilityStatus: domainimagetask.VisibilityPrivate,
		}},
	}
}
