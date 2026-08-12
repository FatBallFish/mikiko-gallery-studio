package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	domainmedia "github.com/fatballfish/pic-gallery/internal/domain/media"
	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	mediaassetservice "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	projectservice "github.com/fatballfish/pic-gallery/internal/service/project"
	videopricingservice "github.com/fatballfish/pic-gallery/internal/service/videopricing"
	videoroutingservice "github.com/fatballfish/pic-gallery/internal/service/videorouting"
	videotaskservice "github.com/fatballfish/pic-gallery/internal/service/videotask"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"gopkg.in/yaml.v3"
)

func TestVideoTasksAPICreatesReplaysListsGetsAndCancels(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	client, err := repoent.Open(dialect.SQLite, "file:video-tasks-api-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	authSvc, owner := loginTestUser(t, "video-tasks@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	userID := claims.UserID
	project, err := client.Project.Create().SetUserID(userID).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(userID).SetProjectID(project.ID).SetName("hero").SetNameKey("hero").SetMediaType("image").SetSourceType("local_upload").SetStatus("ready").SetObjectKey("media/original/hero.png").SetMimeType("image/png").SetFileSizeBytes(10).Save(ctx); err != nil {
		t.Fatal(err)
	}

	account, _ := client.ModelAccount.Create().SetName("MiniMax").SetAdapterType("minimax").SetAuthType("api_key").SetBaseURL("https://provider.invalid").SetStatus("enabled").Save(ctx)
	accountModel, _ := client.ModelAccountModel.Create().SetAccountID(int64(account.ID)).SetModelCode("MiniMax-H3").SetDisplayName("MiniMax H3").SetEnabled(true).Save(ctx)
	capability := domainvideo.Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, PromptMaxRunes: 7000, TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{domainvideo.TaskTypeImageToVideo: {Durations: domainvideo.DiscreteIntValues(5), Resolutions: []domainvideo.Resolution{domainvideo.Resolution2K}, AspectRatios: []domainvideo.AspectRatio{domainvideo.AspectRatioAdaptive}, AudioModes: []domainvideo.AudioMode{domainvideo.AudioModeSilent}, Inputs: map[domainvideo.InputRole]domainvideo.InputCapability{domainvideo.InputRoleFirstFrame: {Required: true, MaxCount: 1, MaxBytes: 30 << 20, MediaTypes: []string{"image"}, Formats: []string{"image/png"}}}}}}
	capabilityJSON := map[string]any{}
	raw, _ := json.Marshal(capability)
	_ = json.Unmarshal(raw, &capabilityJSON)
	_, _ = client.VideoModelCapability.Create().SetAccountModelID(int64(accountModel.ID)).SetCapabilityVersion("cap-v1").SetCapabilityJSON(capabilityJSON).SetValidationStatus("verified").SetEnabled(true).Save(ctx)
	route, _ := client.RouteModel.Create().SetCode("cinema").SetName("Cinema").SetMediaType("video").SetVisibility("public").SetEnabled(true).Save(ctx)
	_, _ = client.RouteModelCandidate.Create().SetRouteModelID(int64(route.ID)).SetAccountModelID(int64(accountModel.ID)).SetEnabled(true).Save(ctx)
	strategy, _ := client.VideoPricingStrategy.Create().SetCode("video").SetName("Video").SetEnabled(true).Save(ctx)
	_, _ = client.VideoPriceRule.Create().SetPricingStrategyID(int64(strategy.ID)).SetTaskType("image_to_video").SetResolution("2k").SetAudioMode("silent").SetEffectiveAt(now.Add(-time.Hour)).SetOutputSecondPoints("2.00000").SetMinimumTaskPoints("8.00000").SetReserveMarkup("1.00000").SetSafetyPoints("8.00000").SetSafetySnapshot(map[string]any{}).SetEnabled(true).Save(ctx)
	_, _ = client.VideoRouteConfig.Create().SetRouteModelID(int64(route.ID)).SetTaskTypes([]string{"image_to_video"}).SetVisibleOptions(map[string]any{}).SetDefaults(map[string]any{}).SetMaxOutputCount(4).SetPricingStrategyID(int64(strategy.ID)).SetConfigVersion("route-v1").SetEnabled(true).Save(ctx)

	billingStore := entstore.NewBillingStore(client, 5)
	if _, err := billingStore.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: userID, ChangePoints: "100.00000", Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	configStore := entstore.NewVideoConfigStore(client)
	routing := videoroutingservice.NewService(configStore)
	quotes := videotaskservice.NewQuoteService(routing, videopricingservice.NewService(configStore, func() time.Time { return now }), []byte("test-video-quote-signing-key-32bytes"), func() time.Time { return now })
	mediaService := mediaassetservice.NewService(entstore.NewMediaStore(client), storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), mediaassetservice.Options{Policy: domainmedia.DefaultPolicy()})
	taskService := videotaskservice.NewService(entstore.NewVideoTaskStore(client, billingStore), quotes, projectservice.NewService(entstore.NewProjectStore(client)), mediaService, func() time.Time { return now })
	api := handlers.NewAPIWithRuntimeServices(taskAPIConfig("http://provider.invalid"), authSvc, nil, nil, enabledFeatureAdmin(t, "video_creation"), nil)
	api.SetMediaAssetService(mediaService)
	api.SetVideoServices(routing, quotes, taskService)
	handler := NewWithAPI(api)

	estimateBody := `{"project_id":"` + project.ID.String() + `","route_model_code":"cinema","task_type":"image_to_video","prompt_template":"make {{@hero}} move","reference_bindings":[{"name":"hero","asset_id":"` + assetID.String() + `"}],"duration_seconds":5,"resolution":"2k","aspect_ratio":"adaptive","audio_mode":"silent","output_count":2,"inputs":[{"asset_id":"` + assetID.String() + `","role":"first_frame","ordinal":0}]}`
	estimate := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/video/v1/estimates", estimateBody, nil)
	if estimate.Code != http.StatusOK {
		t.Fatalf("estimate=%d %s", estimate.Code, estimate.Body.String())
	}
	var quoted struct {
		Data videotaskservice.Estimate `json:"data"`
	}
	if err := json.Unmarshal(estimate.Body.Bytes(), &quoted); err != nil {
		t.Fatal(err)
	}
	createBody := `{"project_id":"` + project.ID.String() + `","route_model_code":"cinema","task_type":"image_to_video","prompt_template":"make {{@hero}} move","reference_bindings":[{"name":"hero","asset_id":"` + assetID.String() + `"}],"duration_seconds":5,"resolution":"2k","aspect_ratio":"adaptive","audio_mode":"silent","output_count":2,"inputs":[{"asset_id":"` + assetID.String() + `","role":"first_frame","ordinal":0}],"quote_token":"` + quoted.Data.QuoteToken + `"}`
	created := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/video/v1/tasks", createBody, map[string]string{"Idempotency-Key": "video-create-1"})
	if created.Code != http.StatusAccepted || !bytes.Contains(created.Body.Bytes(), []byte(`"requested_output_count":2`)) {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	var createResponse struct {
		Data videotaskservice.Task `json:"data"`
	}
	_ = json.Unmarshal(created.Body.Bytes(), &createResponse)
	replay := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/video/v1/tasks", createBody, map[string]string{"Idempotency-Key": "video-create-1"})
	if replay.Code != http.StatusOK || !bytes.Contains(replay.Body.Bytes(), []byte(createResponse.Data.ID.String())) {
		t.Fatalf("replay=%d %s", replay.Code, replay.Body.String())
	}
	list := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/video/v1/tasks?project_id="+project.ID.String(), "", nil)
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(createResponse.Data.ID.String())) {
		t.Fatalf("list=%d %s", list.Code, list.Body.String())
	}
	detail := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, "/api/agent/video/v1/tasks/"+createResponse.Data.ID.String(), "", nil)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail=%d %s", detail.Code, detail.Body.String())
	}
	cancel := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodPost, "/api/agent/video/v1/tasks/"+createResponse.Data.ID.String()+":cancel", "", map[string]string{"Idempotency-Key": "cancel-1"})
	if cancel.Code != http.StatusAccepted || !bytes.Contains(cancel.Body.Bytes(), []byte("cancel_requested")) {
		t.Fatalf("cancel=%d %s", cancel.Code, cancel.Body.String())
	}
}

func TestVideoTaskEventsFiltersProjectsAndSupportsResumeWithCompactProjection(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:video-task-events-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	authSvc, owner := loginTestUser(t, "video-events@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	project, err := client.Project.Create().SetUserID(claims.UserID).SetName("Selected").SetNameKey("selected").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	otherProject, err := client.Project.Create().SetUserID(claims.UserID).SetName("Other").SetNameKey("other").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	firstUpdated := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	firstTask := createVideoEventTask(t, client, claims.UserID, project.ID, "events-first", firstUpdated, "queued", "queued")
	secondTask := createVideoEventTask(t, client, claims.UserID, project.ID, "events-second", firstUpdated.Add(time.Second), "succeeded", "completed")
	_ = createVideoEventTask(t, client, claims.UserID, otherProject.ID, "events-other", firstUpdated.Add(2*time.Second), "running", "generating")

	taskService := videotaskservice.NewService(entstore.NewVideoTaskStore(client, entstore.NewBillingStore(client, 5)), nil, nil, nil, nil)
	api := handlers.NewAPIWithRuntimeServices(taskAPIConfig("http://provider.invalid"), authSvc, nil, nil, enabledFeatureAdmin(t, "video_creation"), nil)
	api.SetVideoServices(nil, nil, taskService)
	handler := NewWithAPI(api)
	basePath := "/api/agent/video/v1/tasks/events?once=true&project_id=" + project.ID.String()

	initial := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, basePath, "", nil)
	if initial.Code != http.StatusOK {
		t.Fatalf("events=%d %s", initial.Code, initial.Body.String())
	}
	events := decodeVideoTaskEvents(t, initial.Body.String())
	if len(events) != 2 || !strings.HasPrefix(events[0].ID, firstTask.ID.String()+":") || !strings.HasPrefix(events[1].ID, secondTask.ID.String()+":") {
		t.Fatalf("events=%#v body=%s", events, initial.Body.String())
	}
	for _, event := range events {
		if event.Name != "task" {
			t.Fatalf("event name=%q", event.Name)
		}
		if len(event.Data) != 6 {
			t.Fatalf("projection keys=%#v", event.Data)
		}
		for _, key := range []string{"id", "version", "status", "stage", "updated_at", "result_ready"} {
			if _, ok := event.Data[key]; !ok {
				t.Fatalf("projection missing %q: %#v", key, event.Data)
			}
		}
		for _, forbidden := range []string{"prompt_template", "execution_prompt", "pricing_snapshot", "inputs", "items"} {
			if _, ok := event.Data[forbidden]; ok {
				t.Fatalf("projection leaked %q: %#v", forbidden, event.Data)
			}
		}
	}
	if events[0].Data["result_ready"] != false || events[1].Data["result_ready"] != true {
		t.Fatalf("result readiness projections=%#v", events)
	}

	after := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, basePath+"&after="+events[0].ID, "", nil)
	afterEvents := decodeVideoTaskEvents(t, after.Body.String())
	if after.Code != http.StatusOK || len(afterEvents) != 1 || afterEvents[0].ID != events[1].ID {
		t.Fatalf("after=%d events=%#v body=%s", after.Code, afterEvents, after.Body.String())
	}
	headerResume := authenticatedMediaRequest(t, handler, owner.AccessToken, http.MethodGet, basePath, "", map[string]string{"Last-Event-ID": events[0].ID})
	headerEvents := decodeVideoTaskEvents(t, headerResume.Body.String())
	if headerResume.Code != http.StatusOK || len(headerEvents) != 1 || headerEvents[0].ID != events[1].ID {
		t.Fatalf("last-event-id=%d events=%#v body=%s", headerResume.Code, headerEvents, headerResume.Body.String())
	}
}

func createVideoEventTask(t *testing.T, client *repoent.Client, userID int64, projectID uuid.UUID, key string, updatedAt time.Time, status, stage string) *repoent.VideoTask {
	t.Helper()
	task, err := client.VideoTask.Create().
		SetUserID(userID).SetProjectID(projectID).SetTaskType("text_to_video").SetStatus(status).SetProgressStage(stage).
		SetPromptTemplate("secret prompt").SetPromptBindingSnapshot(map[string]any{}).SetExecutionPrompt("expanded secret prompt").SetRouteModelID(1).SetRouteModelCode("cinema").
		SetDurationSeconds(5).SetResolution("720p").SetAspectRatio("16:9").SetRequestedOutputCount(1).
		SetPricingSnapshot(map[string]any{"unit_points": "10.00000"}).SetRoutingSnapshot(map[string]any{}).
		SetIdempotencyKey(key).SetRequestFingerprint(strings.Repeat("a", 64)).SetUpdatedAt(updatedAt).Save(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	item := client.VideoTaskItem.Create().SetTaskID(task.ID).SetOrdinal(0).SetStatus(status).SetStage(stage)
	if status == "succeeded" {
		item.SetResultAssetID(uuid.New())
	}
	if _, err := item.Save(t.Context()); err != nil {
		t.Fatal(err)
	}
	return task
}

type videoTaskSSEEvent struct {
	ID   string
	Name string
	Data map[string]any
}

func decodeVideoTaskEvents(t *testing.T, body string) []videoTaskSSEEvent {
	t.Helper()
	var events []videoTaskSSEEvent
	for _, block := range strings.Split(strings.TrimSpace(body), "\n\n") {
		var event videoTaskSSEEvent
		for _, line := range strings.Split(block, "\n") {
			switch {
			case strings.HasPrefix(line, "id: "):
				event.ID = strings.TrimPrefix(line, "id: ")
			case strings.HasPrefix(line, "event: "):
				event.Name = strings.TrimPrefix(line, "event: ")
			case strings.HasPrefix(line, "data: "):
				if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event.Data); err != nil {
					t.Fatalf("decode SSE data: %v", err)
				}
			}
		}
		if event.ID != "" || event.Name != "" || event.Data != nil {
			events = append(events, event)
		}
	}
	return events
}

func TestVideoTaskErrorsUseDomainCodes(t *testing.T) {
	fixture := newVideoTaskAPIFixture(t)

	invalidField := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, "/api/agent/video/v1/estimates", `{"unknown":true}`, nil)
	assertVideoAPIError(t, invalidField, http.StatusBadRequest, "VIDEO_FIELD_INVALID")
	invalidLimit := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodGet, "/api/agent/video/v1/tasks?limit=101", "", nil)
	assertVideoAPIError(t, invalidLimit, http.StatusBadRequest, "VIDEO_FIELD_INVALID")
	invalidStatus := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodGet, "/api/agent/video/v1/tasks?status=unknown", "", nil)
	assertVideoAPIError(t, invalidStatus, http.StatusBadRequest, "VIDEO_FIELD_INVALID")

	inputBody := strings.Replace(fixture.estimateBody, fixture.assetID.String(), uuid.Nil.String(), 2)
	invalidInput := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, "/api/agent/video/v1/estimates", inputBody, nil)
	assertVideoAPIError(t, invalidInput, http.StatusBadRequest, "VIDEO_INPUT_INVALID")

	estimate := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, "/api/agent/video/v1/estimates", fixture.estimateBody, nil)
	quote := decodeVideoEstimate(t, estimate)
	createBody := fixture.createBody(quote.QuoteToken)
	created := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, "/api/agent/video/v1/tasks", createBody, map[string]string{"Idempotency-Key": "domain-errors"})
	if created.Code != http.StatusAccepted {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	conflictingBody := strings.Replace(createBody, "make {{@hero}} move", "make {{@hero}} dance", 1)
	conflict := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, "/api/agent/video/v1/tasks", conflictingBody, map[string]string{"Idempotency-Key": "domain-errors"})
	assertVideoAPIError(t, conflict, http.StatusConflict, "IDEMPOTENCY_KEY_REUSED")
}

func TestVideoTasksAPIOwnerScopingMethodsAndPreflight(t *testing.T) {
	fixture := newVideoTaskAPIFixture(t)
	task := fixture.createTask(t, "owner-scoping")
	foreign := loginExistingAuthUser(t, fixture.auth, "video-foreign@example.com")

	detailPath := "/api/agent/video/v1/tasks/" + task.ID.String()
	foreignDetail := authenticatedMediaRequest(t, fixture.handler, foreign.AccessToken, http.MethodGet, detailPath, "", nil)
	assertVideoAPIError(t, foreignDetail, http.StatusNotFound, "NOT_FOUND")
	foreignCancel := authenticatedMediaRequest(t, fixture.handler, foreign.AccessToken, http.MethodPost, detailPath+":cancel", "", map[string]string{"Idempotency-Key": "foreign-cancel"})
	assertVideoAPIError(t, foreignCancel, http.StatusNotFound, "NOT_FOUND")

	for _, testCase := range []struct {
		method string
		path   string
	}{
		{method: http.MethodPatch, path: "/api/agent/video/v1/tasks"},
		{method: http.MethodPost, path: detailPath},
		{method: http.MethodGet, path: detailPath + ":cancel"},
		{method: http.MethodPost, path: "/api/agent/video/v1/tasks/events"},
	} {
		response := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, testCase.method, testCase.path, "", nil)
		assertVideoAPIError(t, response, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED")
	}

	for _, testCase := range []struct {
		path   string
		method string
		want   int
	}{
		{path: "/api/agent/video/v1/estimates", method: http.MethodPost, want: http.StatusNoContent},
		{path: "/api/agent/video/v1/tasks", method: http.MethodGet, want: http.StatusNoContent},
		{path: detailPath, method: http.MethodGet, want: http.StatusNoContent},
		{path: detailPath + ":cancel", method: http.MethodPost, want: http.StatusNoContent},
		{path: "/api/agent/video/v1/tasks/events", method: http.MethodGet, want: http.StatusNoContent},
		{path: detailPath, method: http.MethodPatch, want: http.StatusNotFound},
	} {
		req := httptest.NewRequest(http.MethodOptions, testCase.path, nil)
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Access-Control-Request-Method", testCase.method)
		recorder := httptest.NewRecorder()
		fixture.handler.ServeHTTP(recorder, req)
		if recorder.Code != testCase.want {
			t.Errorf("OPTIONS %s for %s = %d, want %d body=%s", testCase.path, testCase.method, recorder.Code, testCase.want, recorder.Body.String())
		}
	}
}

func TestVideoTaskCreateConcurrentReplayAndCancelIdempotency(t *testing.T) {
	fixture := newVideoTaskAPIFixture(t)
	estimate := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, "/api/agent/video/v1/estimates", fixture.estimateBody, nil)
	quote := decodeVideoEstimate(t, estimate)
	createBody := fixture.createBody(quote.QuoteToken)

	const concurrentRequests = 4
	responses := make([]*httptest.ResponseRecorder, concurrentRequests)
	var wait sync.WaitGroup
	wait.Add(concurrentRequests)
	for index := range responses {
		go func(index int) {
			defer wait.Done()
			responses[index] = authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, "/api/agent/video/v1/tasks", createBody, map[string]string{"Idempotency-Key": "concurrent-create"})
		}(index)
	}
	wait.Wait()
	taskIDs := map[uuid.UUID]struct{}{}
	accepted := 0
	for _, response := range responses {
		if response.Code == http.StatusAccepted {
			accepted++
		} else if response.Code != http.StatusOK {
			t.Fatalf("concurrent create=%d %s", response.Code, response.Body.String())
		}
		var payload struct {
			Data videotaskservice.Task `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		taskIDs[payload.Data.ID] = struct{}{}
	}
	if accepted != 1 || len(taskIDs) != 1 {
		t.Fatalf("concurrent create accepted=%d unique_tasks=%d", accepted, len(taskIDs))
	}

	var taskID uuid.UUID
	for id := range taskIDs {
		taskID = id
	}
	cancelPath := "/api/agent/video/v1/tasks/" + taskID.String() + ":cancel"
	missingKey := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, cancelPath, "", nil)
	assertVideoAPIError(t, missingKey, http.StatusBadRequest, "VIDEO_FIELD_INVALID")
	longKey := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, cancelPath, "", map[string]string{"Idempotency-Key": strings.Repeat("x", 129)})
	assertVideoAPIError(t, longKey, http.StatusBadRequest, "VIDEO_FIELD_INVALID")
	first := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, cancelPath, "", map[string]string{"Idempotency-Key": "cancel-replay"})
	second := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, cancelPath, "", map[string]string{"Idempotency-Key": "cancel-replay"})
	third := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, cancelPath, "", map[string]string{"Idempotency-Key": "cancel-other-key"})
	for _, response := range []*httptest.ResponseRecorder{first, second, third} {
		if response.Code != http.StatusAccepted {
			t.Fatalf("cancel replay=%d %s", response.Code, response.Body.String())
		}
	}
	versions := make([]int64, 0, 3)
	for _, response := range []*httptest.ResponseRecorder{first, second, third} {
		var payload struct {
			Data videotaskservice.Task `json:"data"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, payload.Data.Items[0].Version)
	}
	if versions[0] != versions[1] || versions[1] != versions[2] {
		t.Fatalf("cancel replays changed item version: %v", versions)
	}
}

func TestVideoTaskOpenAPIUsesTypedContracts(t *testing.T) {
	raw, err := os.ReadFile("../../../api/openapi/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var document struct {
		Paths map[string]map[string]struct {
			RequestBody struct {
				Content map[string]struct {
					Schema map[string]any `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"requestBody"`
			Responses map[string]struct {
				Content map[string]struct {
					Schema map[string]any `yaml:"schema"`
				} `yaml:"content"`
			} `yaml:"responses"`
		} `yaml:"paths"`
		Components struct {
			Schemas map[string]any `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{"VideoTaskEstimateRequest", "VideoTaskCreateRequest", "VideoEstimateResponse", "VideoTaskResponse", "VideoTaskPageResponse", "VideoErrorResponse"} {
		if _, ok := document.Components.Schemas[schema]; !ok {
			t.Errorf("OpenAPI missing component schema %s", schema)
		}
	}
	for _, operation := range []struct {
		path   string
		method string
		ref    string
	}{
		{path: "/api/agent/video/v1/estimates", method: "post", ref: "#/components/schemas/VideoTaskEstimateRequest"},
		{path: "/api/agent/video/v1/tasks", method: "post", ref: "#/components/schemas/VideoTaskCreateRequest"},
	} {
		schema := document.Paths[operation.path][operation.method].RequestBody.Content["application/json"].Schema
		if schema["$ref"] != operation.ref {
			t.Errorf("%s %s request schema=%#v, want ref %s", operation.method, operation.path, schema, operation.ref)
		}
	}
	for _, operation := range []struct {
		path     string
		method   string
		statuses []string
	}{
		{path: "/api/agent/video/v1/estimates", method: "post", statuses: []string{"200", "400", "404", "409", "422"}},
		{path: "/api/agent/video/v1/tasks", method: "get", statuses: []string{"200", "400"}},
		{path: "/api/agent/video/v1/tasks", method: "post", statuses: []string{"200", "202", "400", "402", "404", "409", "422"}},
		{path: "/api/agent/video/v1/tasks/{task_id}", method: "get", statuses: []string{"200", "404"}},
		{path: "/api/agent/video/v1/tasks/{task_id}:cancel", method: "post", statuses: []string{"202", "400", "404"}},
	} {
		responses := document.Paths[operation.path][operation.method].Responses
		for _, status := range operation.statuses {
			if len(responses[status].Content["application/json"].Schema) == 0 {
				t.Errorf("%s %s response %s has no typed JSON schema", operation.method, operation.path, status)
			}
		}
	}
}

type videoTaskAPIFixture struct {
	client       *repoent.Client
	auth         *authservice.Service
	owner        domainauthSession
	handler      http.Handler
	projectID    uuid.UUID
	assetID      uuid.UUID
	estimateBody string
}

func newVideoTaskAPIFixture(t *testing.T) videoTaskAPIFixture {
	t.Helper()
	ctx := t.Context()
	now := time.Date(2026, 8, 12, 12, 0, 0, 0, time.UTC)
	client, err := repoent.Open(dialect.SQLite, "file:video-task-contract-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	authSvc, owner := loginTestUser(t, "video-contract-"+uuid.NewString()+"@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	userID := claims.UserID
	project, err := client.Project.Create().SetUserID(userID).SetName("Default").SetNameKey("default").SetIsDefault(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assetID := uuid.New()
	if _, err := client.MediaAsset.Create().SetID(assetID).SetUserID(userID).SetProjectID(project.ID).SetName("hero").SetNameKey("hero").SetMediaType("image").SetSourceType("local_upload").SetStatus("ready").SetObjectKey("media/original/hero.png").SetMimeType("image/png").SetFileSizeBytes(10).Save(ctx); err != nil {
		t.Fatal(err)
	}
	account, err := client.ModelAccount.Create().SetName("MiniMax").SetAdapterType("minimax").SetAuthType("api_key").SetBaseURL("https://provider.invalid").SetStatus("enabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accountModel, err := client.ModelAccountModel.Create().SetAccountID(int64(account.ID)).SetModelCode("MiniMax-H3").SetDisplayName("MiniMax H3").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	capability := domainvideo.Capability{SchemaVersion: 1, ProviderNativeMaxN: 1, PromptMaxRunes: 7000, TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{domainvideo.TaskTypeImageToVideo: {Durations: domainvideo.DiscreteIntValues(5), Resolutions: []domainvideo.Resolution{domainvideo.Resolution2K}, AspectRatios: []domainvideo.AspectRatio{domainvideo.AspectRatioAdaptive}, AudioModes: []domainvideo.AudioMode{domainvideo.AudioModeSilent}, Inputs: map[domainvideo.InputRole]domainvideo.InputCapability{domainvideo.InputRoleFirstFrame: {Required: true, MaxCount: 1, MaxBytes: 30 << 20, MediaTypes: []string{"image"}, Formats: []string{"image/png"}}}}}}
	capabilityJSON := map[string]any{}
	raw, _ := json.Marshal(capability)
	_ = json.Unmarshal(raw, &capabilityJSON)
	if _, err := client.VideoModelCapability.Create().SetAccountModelID(int64(accountModel.ID)).SetCapabilityVersion("cap-v1").SetCapabilityJSON(capabilityJSON).SetValidationStatus("verified").SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	route, err := client.RouteModel.Create().SetCode("cinema").SetName("Cinema").SetMediaType("video").SetVisibility("public").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RouteModelCandidate.Create().SetRouteModelID(int64(route.ID)).SetAccountModelID(int64(accountModel.ID)).SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	strategy, err := client.VideoPricingStrategy.Create().SetCode("video").SetName("Video").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoPriceRule.Create().SetPricingStrategyID(int64(strategy.ID)).SetTaskType("image_to_video").SetResolution("2k").SetAudioMode("silent").SetEffectiveAt(now.Add(-time.Hour)).SetOutputSecondPoints("2.00000").SetMinimumTaskPoints("8.00000").SetReserveMarkup("1.00000").SetSafetyPoints("8.00000").SetSafetySnapshot(map[string]any{}).SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoRouteConfig.Create().SetRouteModelID(int64(route.ID)).SetTaskTypes([]string{"image_to_video"}).SetVisibleOptions(map[string]any{}).SetDefaults(map[string]any{}).SetMaxOutputCount(4).SetPricingStrategyID(int64(strategy.ID)).SetConfigVersion("route-v1").SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	billingStore := entstore.NewBillingStore(client, 5)
	if _, err := billingStore.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: userID, ChangePoints: "1000.00000", Reason: "seed"}); err != nil {
		t.Fatal(err)
	}
	configStore := entstore.NewVideoConfigStore(client)
	routing := videoroutingservice.NewService(configStore)
	quotes := videotaskservice.NewQuoteService(routing, videopricingservice.NewService(configStore, func() time.Time { return now }), []byte("test-video-quote-signing-key-32bytes"), func() time.Time { return now })
	mediaService := mediaassetservice.NewService(entstore.NewMediaStore(client), storage.NewStaticRouter(storage.NewLocalBackend(t.TempDir())), mediaassetservice.Options{Policy: domainmedia.DefaultPolicy()})
	taskService := videotaskservice.NewService(entstore.NewVideoTaskStore(client, billingStore), quotes, projectservice.NewService(entstore.NewProjectStore(client)), mediaService, func() time.Time { return now })
	api := handlers.NewAPIWithRuntimeServices(taskAPIConfig("http://provider.invalid"), authSvc, nil, nil, enabledFeatureAdmin(t, "video_creation"), nil)
	api.SetMediaAssetService(mediaService)
	api.SetVideoServices(routing, quotes, taskService)
	estimateBody := `{"project_id":"` + project.ID.String() + `","route_model_code":"cinema","task_type":"image_to_video","prompt_template":"make {{@hero}} move","reference_bindings":[{"name":"hero","asset_id":"` + assetID.String() + `"}],"duration_seconds":5,"resolution":"2k","aspect_ratio":"adaptive","audio_mode":"silent","output_count":1,"inputs":[{"asset_id":"` + assetID.String() + `","role":"first_frame","ordinal":0}]}`
	return videoTaskAPIFixture{client: client, auth: authSvc, owner: owner, handler: NewWithAPI(api), projectID: project.ID, assetID: assetID, estimateBody: estimateBody}
}

func (fixture videoTaskAPIFixture) createBody(quoteToken string) string {
	return strings.TrimSuffix(fixture.estimateBody, "}") + `,"quote_token":"` + quoteToken + `"}`
}

func (fixture videoTaskAPIFixture) createTask(t *testing.T, key string) videotaskservice.Task {
	t.Helper()
	estimate := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, "/api/agent/video/v1/estimates", fixture.estimateBody, nil)
	quote := decodeVideoEstimate(t, estimate)
	created := authenticatedMediaRequest(t, fixture.handler, fixture.owner.AccessToken, http.MethodPost, "/api/agent/video/v1/tasks", fixture.createBody(quote.QuoteToken), map[string]string{"Idempotency-Key": key})
	if created.Code != http.StatusAccepted {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	var payload struct {
		Data videotaskservice.Task `json:"data"`
	}
	if err := json.Unmarshal(created.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Data
}

func decodeVideoEstimate(t *testing.T, response *httptest.ResponseRecorder) videotaskservice.Estimate {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("estimate=%d %s", response.Code, response.Body.String())
	}
	var payload struct {
		Data videotaskservice.Estimate `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	return payload.Data
}

func assertVideoAPIError(t *testing.T, response *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if response.Code != status {
		t.Fatalf("status=%d body=%s, want %d", response.Code, response.Body.String(), status)
	}
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Error.Code != code {
		t.Fatalf("error code=%q body=%s, want %q", payload.Error.Code, response.Body.String(), code)
	}
}
