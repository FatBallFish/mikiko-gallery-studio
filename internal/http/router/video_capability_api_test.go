package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"

	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminvideoservice "github.com/fatballfish/pic-gallery/internal/service/adminvideo"
	videopricingservice "github.com/fatballfish/pic-gallery/internal/service/videopricing"
	videoroutingservice "github.com/fatballfish/pic-gallery/internal/service/videorouting"
	videotaskservice "github.com/fatballfish/pic-gallery/internal/service/videotask"
)

func TestVideoCapabilityAndEstimateUseCompleteVerifiedCandidate(t *testing.T) {
	ctx := t.Context()
	now := time.Date(2026, 8, 12, 8, 0, 0, 0, time.UTC)
	client, err := repoent.Open(dialect.SQLite, "file:video-capability-api-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatal(err)
	}
	account, err := client.ModelAccount.Create().SetName("Seedance").SetAdapterType("seedance").SetAuthType("api_key").SetBaseURL("https://provider.invalid").SetStatus("enabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	accountModel, err := client.ModelAccountModel.Create().SetAccountID(int64(account.ID)).SetModelCode("doubao-seedance-2-5-260815").SetDisplayName("Seedance 2.5").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	capability := domainvideo.Capability{
		SchemaVersion: 1, ProviderNativeMaxN: 1, PromptMaxRunes: 2000,
		TaskTypes: map[domainvideo.TaskType]domainvideo.TaskCapability{
			domainvideo.TaskTypeTextToVideo: {
				Durations: domainvideo.DiscreteIntValues(5, 10), Resolutions: []domainvideo.Resolution{domainvideo.Resolution720P},
				AspectRatios: []domainvideo.AspectRatio{domainvideo.AspectRatio16x9, domainvideo.AspectRatio9x16}, AudioModes: []domainvideo.AudioMode{domainvideo.AudioModeSilent},
			},
		},
	}
	capabilityJSON := map[string]any{}
	encoded, _ := json.Marshal(capability)
	_ = json.Unmarshal(encoded, &capabilityJSON)
	if _, err := client.VideoModelCapability.Create().SetAccountModelID(int64(accountModel.ID)).SetCapabilityVersion("cap-v1").SetCapabilityJSON(capabilityJSON).
		SetValidationStatus("verified").SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	route, err := client.RouteModel.Create().SetCode("cinema").SetName("电影感视频").SetDescription("稳定的短视频生成").SetMediaType("video").SetVisibility("groups").SetEnabled(true).Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	basicGroup, err := client.UserGroup.Create().SetGroupCode("basic").SetGroupName("Basic").SetStatus("enabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RouteModelVisibilityGroup.Create().SetRouteModelID(int64(route.ID)).SetGroupID(int64(basicGroup.ID)).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.RouteModelCandidate.Create().SetRouteModelID(int64(route.ID)).SetAccountModelID(int64(accountModel.ID)).SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoModelRateCard.Create().SetAccountModelID(int64(accountModel.ID)).SetProviderCode("seedance").SetPricingSchema("seedance_token_v1").SetRateVersion(1).SetCurrency("CNY").
		SetRateConfig(map[string]any{"resolutions": map[string]any{"720p": map[string]any{"without_input_video_million_tokens_cny": "46"}}}).SetSourceReference("test fixture").SetEffectiveAt(now.Add(-time.Hour)).SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := client.VideoRouteConfig.Create().SetRouteModelID(int64(route.ID)).SetTaskTypes([]string{string(domainvideo.TaskTypeTextToVideo)}).
		SetVisibleOptions(map[string]any{}).SetDefaults(map[string]any{}).SetMaxOutputCount(4).SetMinimumTaskPoints("8.00000").SetRoundingStepPoints(1).SetConfigVersion("route-v2").SetEnabled(true).Save(ctx); err != nil {
		t.Fatal(err)
	}

	configStore := entstore.NewVideoConfigStore(client)
	routing := videoroutingservice.NewService(configStore)
	pricing := videopricingservice.NewService(adminvideoservice.NewService(entstore.NewAdminVideoStore(client)), func() time.Time { return now })
	quotes := videotaskservice.NewQuoteService(routing, pricing, []byte("test-video-quote-signing-key-32bytes"), func() time.Time { return now })
	authSvc, session := loginTestUser(t, "video-capability@example.com")
	api := handlers.NewAPIWithRuntimeServices(taskAPIConfig("http://provider.invalid"), authSvc, nil, nil, enabledFeatureAdmin(t, "video_creation"), nil)
	api.SetVideoServices(routing, quotes)
	handler := NewWithAPI(api)

	capabilities := authenticatedMediaRequest(t, handler, session.AccessToken, http.MethodGet, "/api/agent/video/v1/capabilities?route_model_code=cinema", "", nil)
	if capabilities.Code != http.StatusOK || !bytes.Contains(capabilities.Body.Bytes(), []byte(`"capability_version":"cap-v1"`)) || !bytes.Contains(capabilities.Body.Bytes(), []byte(`"duration_seconds":5`)) {
		t.Fatalf("capabilities=%d %s", capabilities.Code, capabilities.Body.String())
	}
	groups := authenticatedMediaRequest(t, handler, session.AccessToken, http.MethodGet, "/api/agent/video/v1/capabilities", "", nil)
	if groups.Code != http.StatusOK || !bytes.Contains(groups.Body.Bytes(), []byte(`"groups"`)) || !bytes.Contains(groups.Body.Bytes(), []byte(`"route_model_code":"cinema"`)) {
		t.Fatalf("capability groups=%d %s", groups.Code, groups.Body.String())
	}
	estimateBody := `{"route_model_code":"cinema","task_type":"text_to_video","prompt":"a quiet lake","duration_seconds":5,"resolution":"720p","aspect_ratio":"16:9","audio_mode":"silent","output_count":2}`
	estimate := authenticatedMediaRequest(t, handler, session.AccessToken, http.MethodPost, "/api/agent/video/v1/estimates", estimateBody, nil)
	if estimate.Code != http.StatusOK || !bytes.Contains(estimate.Body.Bytes(), []byte(`"unit_points":"497.00000"`)) || !bytes.Contains(estimate.Body.Bytes(), []byte(`"estimated_points":"994.00000"`)) || !bytes.Contains(estimate.Body.Bytes(), []byte(`"quote_token"`)) {
		t.Fatalf("estimate=%d %s", estimate.Code, estimate.Body.String())
	}
	if _, err := client.RouteModelVisibilityGroup.Delete().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	otherGroup, err := client.UserGroup.Create().SetGroupCode("other").SetGroupName("Other").SetStatus("enabled").Save(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.RouteModelVisibilityGroup.Create().SetRouteModelID(int64(route.ID)).SetGroupID(int64(otherGroup.ID)).Save(ctx); err != nil {
		t.Fatal(err)
	}
	forbidden := authenticatedMediaRequest(t, handler, session.AccessToken, http.MethodGet, "/api/agent/video/v1/capabilities?route_model_code=cinema", "", nil)
	if forbidden.Code != http.StatusForbidden || !bytes.Contains(forbidden.Body.Bytes(), []byte(`"code":"MODEL_ROUTE_NOT_VISIBLE"`)) {
		t.Fatalf("forbidden capability=%d %s", forbidden.Code, forbidden.Body.String())
	}
}
