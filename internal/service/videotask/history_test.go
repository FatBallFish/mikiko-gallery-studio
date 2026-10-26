package videotask_test

import (
	"context"
	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	mediaasset "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	videotask "github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/google/uuid"
	"testing"
	"time"
)

type retryQuotes struct {
	estimated, verified int
	token               string
}

func (q *retryQuotes) Estimate(context.Context, int64, videotask.EstimateRequest) (videotask.Estimate, error) {
	q.estimated++
	return videotask.Estimate{QuoteToken: "fresh", EstimatedPoints: "12", MaxReservedPoints: "15"}, nil
}
func (q *retryQuotes) Verify(_ context.Context, _ int64, _ videotask.EstimateRequest, token string) (videotask.Estimate, error) {
	q.verified++
	q.token = token
	return videotask.Estimate{EstimatedPoints: "12", MaxReservedPoints: "15"}, nil
}
func TestVideoRetryUsesFreshQuoteAndPreservesBindingsInputsAndOriginal(t *testing.T) {
	projectID, assetID := uuid.New(), uuid.New()
	original := videotask.Task{ID: uuid.New(), UserID: 44, ProjectID: projectID, IdempotencyKey: "old-key", Status: domainvideo.TaskStatusFailed, TaskType: domainvideo.TaskTypeImageToVideo, RouteModelCode: "cinema", PromptTemplate: "{{@hero}} in {{$scene}}", DurationSeconds: 10, Resolution: domainvideo.Resolution720P, AspectRatio: domainvideo.AspectRatio16x9, GenerateAudio: true, RequestedOutputCount: 2, PromptBindingSnapshot: map[string]any{"references": []map[string]any{{"name": "hero", "asset_id": assetID.String()}}, "variables": []map[string]any{{"name": "scene", "value": "forest"}}}, Inputs: []videotask.Input{{AssetID: assetID, Role: string(domainvideo.InputRoleFirstFrame), Ordinal: 0}}}
	store := &memoryTaskStore{tasks: []videotask.Task{original}, assets: map[uuid.UUID]mediaasset.Asset{assetID: {ID: assetID, UserID: 44, Name: "hero", MediaType: domainmedia.MediaTypeImage, Status: "ready", MIMEType: "image/png"}}}
	quotes := &retryQuotes{}
	service := videotask.NewService(store, quotes, &fakeProjectResolver{project: domainproject.Project{ID: projectID.String(), UserID: 44, Status: "active"}}, store, time.Now)
	task, err := service.Retry(t.Context(), 44, original.ID, []string{"default"})
	if err != nil {
		t.Fatal(err)
	}
	if task.ID == original.ID || task.IdempotencyKey == original.IdempotencyKey || quotes.estimated != 1 || quotes.verified != 1 || quotes.token != "fresh" || task.ReservedPoints != "15" {
		t.Fatalf("retry identity/quote %+v quotes=%+v", task, quotes)
	}
	if task.ExecutionPrompt != "图片1 in forest" || !task.GenerateAudio || task.DurationSeconds != 10 || task.RequestedOutputCount != 2 || len(task.Inputs) != 1 || task.Inputs[0].AssetID != assetID || store.tasks[0].Status != domainvideo.TaskStatusFailed {
		t.Fatalf("retry lost original or parameters %+v", task)
	}
}
