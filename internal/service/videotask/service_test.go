package videotask_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	domainproject "github.com/fatballfish/pic-gallery/internal/domain/project"
	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	videotask "github.com/fatballfish/pic-gallery/internal/service/videotask"
)

func TestServiceCreateVerifiesQuoteOwnershipTemplateAndIdempotency(t *testing.T) {
	projectID := uuid.New()
	assetID := uuid.New()
	store := &memoryTaskStore{assets: map[uuid.UUID]mediaassetservice.Asset{
		assetID: {ID: assetID, UserID: 44, ProjectID: uuid.New(), Name: "hero", MediaType: domainmedia.MediaTypeImage, Status: "ready", MIMEType: "image/png", FileSizeBytes: 100},
	}}
	quotes := &fakeQuoteVerifier{estimate: videotask.Estimate{RouteModelCode: "cinema", CapabilityVersion: "cap-v1", ConfigVersion: "route-v1", PriceVersion: "price-v1", UnitPoints: "20.00000", EstimatedPoints: "40.00000", MaxReservedPoints: "46.00000", RouteCandidateID: 71, AccountModelID: 72, ModelAccountID: 73, ProviderCode: "seedance", ModelCode: "seedance-2-5", ExpiresAt: time.Now().Add(time.Minute)}}
	projects := &fakeProjectResolver{project: domainproject.Project{ID: projectID.String(), UserID: 44, Status: "active"}}
	service := videotask.NewService(store, quotes, projects, store, func() time.Time { return time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC) })

	request := videotask.CreateRequest{
		UserID: 44, ProjectID: projectID, IdempotencyKey: "create-one", QuoteToken: "quote-token", RouteModelCode: "cinema",
		TaskType: domainvideo.TaskTypeImageToVideo, PromptTemplate: "让 {{@hero}} 在 {{$scene}} 中奔跑",
		PromptVariables:   []videotask.VariableBinding{{Name: "scene", Value: "森林"}},
		ReferenceBindings: []videotask.ReferenceBinding{{Name: "hero", AssetID: assetID}},
		Inputs:            []videotask.InputRequest{{AssetID: assetID, Role: domainvideo.InputRoleFirstFrame, Ordinal: 0}},
		DurationSeconds:   5, Resolution: domainvideo.Resolution720P, AspectRatio: domainvideo.AspectRatioAdaptive,
		AudioMode: domainvideo.AudioModeSilent, OutputCount: 2,
	}
	created, replayed, err := service.Create(t.Context(), request)
	if err != nil || replayed {
		t.Fatalf("Create() = %#v replayed=%v err=%v", created, replayed, err)
	}
	if created.ExecutionPrompt != "让 图片1 在 森林 中奔跑" || created.PromptTemplate != request.PromptTemplate {
		t.Fatalf("prompt snapshots = template %q execution %q", created.PromptTemplate, created.ExecutionPrompt)
	}
	if len(created.Items) != 2 || created.ReservedPoints != "46.00000" || store.createCalls != 1 {
		t.Fatalf("created task = %#v", created)
	}
	if store.lastCreate.Inputs[0].AssetSnapshot["project_id"] != store.assets[assetID].ProjectID.String() {
		t.Fatalf("cross-project asset snapshot = %#v", store.lastCreate.Inputs[0].AssetSnapshot)
	}
	for key, want := range map[string]any{"route_candidate_id": int64(71), "account_model_id": int64(72), "model_account_id": int64(73), "provider_code": "seedance", "model_code": "seedance-2-5"} {
		if got := created.RoutingSnapshot[key]; got != want {
			t.Fatalf("routing snapshot %s = %#v, want %#v", key, got, want)
		}
	}

	replayedTask, replayed, err := service.Create(t.Context(), request)
	if err != nil || !replayed || replayedTask.ID != created.ID || store.createCalls != 1 {
		t.Fatalf("replay = %#v replayed=%v err=%v calls=%d", replayedTask, replayed, err, store.createCalls)
	}
	request.PromptVariables[0].Value = "沙漠"
	if _, _, err := service.Create(t.Context(), request); !errors.Is(err, videotask.ErrIdempotencyConflict) {
		t.Fatalf("different-body replay error = %v", err)
	}
}

func TestServiceNormalizesDisplayCasedEnumTokens(t *testing.T) {
	projectID := uuid.New()
	store := &memoryTaskStore{}
	quotes := &capturingQuoteVerifier{fakeQuoteVerifier: fakeQuoteVerifier{estimate: videotask.Estimate{MaxReservedPoints: "10.00000"}}}
	service := videotask.NewService(store, quotes, &fakeProjectResolver{project: domainproject.Project{ID: projectID.String(), UserID: 44, Status: "active"}}, store, time.Now)

	request := videotask.CreateRequest{
		UserID: 44, ProjectID: projectID, IdempotencyKey: "upper-res", QuoteToken: "quote-token", RouteModelCode: "seedance-2.5",
		TaskType: "IMAGE_TO_VIDEO", PromptTemplate: "paper boat",
		Resolution: domainvideo.Resolution("720P"), AspectRatio: domainvideo.AspectRatio("1:1"), AudioMode: "GENERATED",
		DurationSeconds: 5, OutputCount: 1,
	}
	created, replayed, err := service.Create(t.Context(), request)
	if err != nil || replayed {
		t.Fatalf("Create() err=%v replayed=%v", err, replayed)
	}
	if created.Resolution != domainvideo.Resolution720P || created.TaskType != domainvideo.TaskTypeImageToVideo || created.AspectRatio != domainvideo.AspectRatio1x1 || !created.GenerateAudio {
		t.Fatalf("stored enum tokens not normalized: %#v", created)
	}
	if quotes.verifyVideo.Resolution != domainvideo.Resolution720P || quotes.verifyVideo.TaskType != domainvideo.TaskTypeImageToVideo {
		t.Fatalf("verified request not normalized: %#v", quotes.verifyVideo)
	}

	lowercase := request
	lowercase.TaskType = domainvideo.TaskTypeImageToVideo
	lowercase.Resolution = domainvideo.Resolution720P
	lowercase.AspectRatio = domainvideo.AspectRatio1x1
	lowercase.AudioMode = domainvideo.AudioModeGenerated
	replayedTask, replayed, err := service.Create(t.Context(), lowercase)
	if err != nil || !replayed || replayedTask.ID != created.ID {
		t.Fatalf("normalized replay err=%v replayed=%v", err, replayed)
	}
}

func TestServiceRejectsForeignOrUnavailableInput(t *testing.T) {
	projectID := uuid.New()
	assetID := uuid.New()
	store := &memoryTaskStore{assets: map[uuid.UUID]mediaassetservice.Asset{assetID: {ID: assetID, UserID: 99, ProjectID: projectID, Name: "foreign", MediaType: domainmedia.MediaTypeImage, Status: "ready"}}}
	service := videotask.NewService(store, &fakeQuoteVerifier{estimate: videotask.Estimate{MaxReservedPoints: "10.00000"}}, &fakeProjectResolver{project: domainproject.Project{ID: projectID.String(), UserID: 44, Status: "active"}}, store, time.Now)
	req := videotask.CreateRequest{UserID: 44, ProjectID: projectID, IdempotencyKey: "foreign", QuoteToken: "q", RouteModelCode: "cinema", TaskType: domainvideo.TaskTypeImageToVideo, PromptTemplate: "move", Inputs: []videotask.InputRequest{{AssetID: assetID, Role: domainvideo.InputRoleFirstFrame}}, DurationSeconds: 5, Resolution: domainvideo.Resolution720P, AspectRatio: domainvideo.AspectRatioAdaptive, AudioMode: domainvideo.AudioModeSilent, OutputCount: 1}
	if _, _, err := service.Create(t.Context(), req); err == nil {
		t.Fatal("expected foreign input rejection")
	}
}

func TestServiceListGetAndCancelAreOwnerScopedAndMonotonic(t *testing.T) {
	store := &memoryTaskStore{}
	service := videotask.NewService(store, nil, nil, nil, time.Now)
	task := videotask.Task{ID: uuid.New(), UserID: 44, Status: domainvideo.TaskStatusRunning, Version: 2, Items: []videotask.Item{{ID: uuid.New(), Status: domainvideo.ItemStateProviderQueued, Version: 3}}}
	store.tasks = []videotask.Task{task}
	page, err := service.List(t.Context(), videotask.ListRequest{UserID: 44, Limit: 20})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("List() = %#v, %v", page, err)
	}
	if _, err := service.Get(t.Context(), 99, task.ID); err == nil {
		t.Fatal("expected owner isolation")
	}
	cancelled, err := service.Cancel(t.Context(), 44, task.ID, "cancel-key")
	if err != nil || cancelled.Items[0].Status != domainvideo.ItemStateCancelRequested || cancelled.Version <= task.Version {
		t.Fatalf("Cancel() = %#v, %v", cancelled, err)
	}
}

type fakeQuoteVerifier struct{ estimate videotask.Estimate }

func (f *fakeQuoteVerifier) Verify(context.Context, int64, videotask.EstimateRequest, string) (videotask.Estimate, error) {
	return f.estimate, nil
}

func (f *fakeQuoteVerifier) Estimate(context.Context, int64, videotask.EstimateRequest) (videotask.Estimate, error) {
	return f.estimate, nil
}

type capturingQuoteVerifier struct {
	fakeQuoteVerifier
	verifyVideo domainvideo.Request
}

func (f *capturingQuoteVerifier) Verify(_ context.Context, _ int64, req videotask.EstimateRequest, _ string) (videotask.Estimate, error) {
	f.verifyVideo = req.Video
	return f.estimate, nil
}

type fakeProjectResolver struct{ project domainproject.Project }

func (f *fakeProjectResolver) ResolveOwned(context.Context, int64, string) (domainproject.Project, error) {
	return f.project, nil
}

type memoryTaskStore struct {
	tasks       []videotask.Task
	assets      map[uuid.UUID]mediaassetservice.Asset
	createCalls int
	lastCreate  videotask.CreateRecord
}

func (s *memoryTaskStore) FindByIdempotency(_ context.Context, userID int64, key string) (videotask.Task, bool, error) {
	for _, task := range s.tasks {
		if task.UserID == userID && task.IdempotencyKey == key {
			return task, true, nil
		}
	}
	return videotask.Task{}, false, nil
}
func (s *memoryTaskStore) Create(_ context.Context, record videotask.CreateRecord) (videotask.Task, bool, error) {
	s.createCalls++
	s.lastCreate = record
	s.tasks = append(s.tasks, record.Task)
	return record.Task, false, nil
}
func (s *memoryTaskStore) List(_ context.Context, req videotask.ListRequest) (videotask.Page, error) {
	var out []videotask.Task
	for _, task := range s.tasks {
		if task.UserID == req.UserID {
			out = append(out, task)
		}
	}
	return videotask.Page{Items: out}, nil
}
func (s *memoryTaskStore) Get(_ context.Context, userID int64, id uuid.UUID) (videotask.Task, error) {
	for _, task := range s.tasks {
		if task.UserID == userID && task.ID == id {
			return task, nil
		}
	}
	return videotask.Task{}, projectservice.ErrNotFound
}
func (s *memoryTaskStore) RequestCancel(_ context.Context, userID int64, id uuid.UUID, key string) (videotask.Task, error) {
	task, err := s.Get(context.Background(), userID, id)
	if err != nil {
		return videotask.Task{}, err
	}
	for i := range task.Items {
		if task.Items[i].Status == domainvideo.ItemStateProviderQueued || task.Items[i].Status == domainvideo.ItemStateProviderRunning || task.Items[i].Status == domainvideo.ItemStateQueued {
			task.Items[i].Status = domainvideo.ItemStateCancelRequested
			task.Items[i].Version++
		}
	}
	task.Version++
	for i := range s.tasks {
		if s.tasks[i].ID == id {
			s.tasks[i] = task
		}
	}
	return task, nil
}
func (s *memoryTaskStore) GetAsset(_ context.Context, userID int64, id uuid.UUID) (mediaassetservice.Asset, error) {
	asset, ok := s.assets[id]
	if !ok || asset.UserID != userID {
		return mediaassetservice.Asset{}, projectservice.ErrNotFound
	}
	return asset, nil
}

func TestServicePrepareAcceptsReferenceMediaRolesByKind(t *testing.T) {
	projectID := uuid.New()
	imageID, videoID, audioID := uuid.New(), uuid.New(), uuid.New()
	store := &memoryTaskStore{assets: map[uuid.UUID]mediaassetservice.Asset{
		imageID: {ID: imageID, UserID: 9, ProjectID: projectID, Name: "参考图", MediaType: domainmedia.MediaTypeImage, Status: "ready_original", MIMEType: "image/png", FileSizeBytes: 100},
		videoID: {ID: videoID, UserID: 9, ProjectID: projectID, Name: "参考视频", MediaType: domainmedia.MediaTypeVideo, Status: "ready", MIMEType: "video/mp4", FileSizeBytes: 4096, DurationMS: int64Ptr(5200)},
		audioID: {ID: audioID, UserID: 9, ProjectID: projectID, Name: "参考音频", MediaType: domainmedia.MediaTypeAudio, Status: "ready", MIMEType: "audio/wav", FileSizeBytes: 512},
	}}
	quotes := &fakeQuoteVerifier{estimate: videotask.Estimate{RouteModelCode: "cinema", CapabilityVersion: "cap-v1", ConfigVersion: "route-v1", PriceVersion: "price-v1", UnitPoints: "20.00000", EstimatedPoints: "20.00000", MaxReservedPoints: "20.00000", RouteCandidateID: 1, AccountModelID: 1, ModelAccountID: 1, ProviderCode: "minimax", ModelCode: "MiniMax-H3", ExpiresAt: time.Now().Add(time.Minute)}}
	projects := &fakeProjectResolver{project: domainproject.Project{ID: projectID.String(), UserID: 9, Status: "active"}}
	service := videotask.NewService(store, quotes, projects, store, func() time.Time { return time.Date(2026, 10, 2, 12, 0, 0, 0, time.UTC) })

	request := videotask.CreateRequest{
		UserID: 9, ProjectID: projectID, IdempotencyKey: "reference-media", QuoteToken: "quote-token", RouteModelCode: "cinema",
		TaskType: domainvideo.TaskTypeReferenceToVideo, PromptTemplate: "参考素材演绎",
		Inputs: []videotask.InputRequest{
			{AssetID: imageID, Role: domainvideo.InputRoleReferenceImage, Ordinal: 0},
			{AssetID: videoID, Role: domainvideo.InputRoleReferenceVideo, Ordinal: 1},
			{AssetID: audioID, Role: domainvideo.InputRoleReferenceAudio, Ordinal: 2},
		},
		DurationSeconds: 8, Resolution: domainvideo.Resolution768P, AspectRatio: domainvideo.AspectRatioAdaptive,
		AudioMode: domainvideo.AudioModeSilent, OutputCount: 1,
	}
	estimate, err := service.Estimate(t.Context(), request)
	if err != nil {
		t.Fatalf("Estimate() error = %v", err)
	}
	if estimate.EstimatedPoints != "20.00000" {
		t.Fatalf("estimate = %#v", estimate)
	}

	wrongKind := request
	wrongKind.Inputs = []videotask.InputRequest{{AssetID: audioID, Role: domainvideo.InputRoleReferenceVideo, Ordinal: 0}}
	if _, err := service.Estimate(t.Context(), wrongKind); err == nil {
		t.Fatal("audio asset with reference_video role must be rejected")
	}
}

func int64Ptr(value int) *int64 { out := int64(value); return &out }
