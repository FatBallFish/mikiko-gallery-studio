package canvas

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	assetservice "github.com/fatballfish/pic-gallery/internal/service/assets"
	media "github.com/fatballfish/pic-gallery/internal/service/mediaasset"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"reflect"
	"testing"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	domainvideo "github.com/fatballfish/pic-gallery/internal/domain/video"
	"github.com/google/uuid"
)

func TestImageRequestUsesDraftAndServerSideIdentityAndInputs(t *testing.T) {
	projectID := uuid.New()
	assetID := uuid.New()
	payload := json.RawMessage(`{"draft":{"abstract_model":"plus","task_type":"text_to_image","prompt":"hidden prompt","output_image_count":12,"user_group_multiplier":"0.1","reference_asset_ids":["forged"]}}`)
	submission := GenerationSubmission{
		UserID: 31, UserGroupCode: "paid", UserGroupCodes: []string{"paid", "beta"}, UserGroupMultiplier: "1.25000",
		ProjectID: projectID, CanvasID: uuid.New(), NodeID: "gen", Kind: TaskKindImage, Node: domaincanvas.Node{ID: "gen", Type: domaincanvas.NodeTypeImageGeneration, Payload: payload},
		Inputs: []GenerationInput{
			{Role: domaincanvas.InputRolePrompt, Node: domaincanvas.Node{ID: "prompt", Type: domaincanvas.NodeTypePrompt, Payload: json.RawMessage(`{"text":"connected prompt"}`)}},
			{Role: domaincanvas.InputRoleReference, Node: domaincanvas.Node{ID: "asset", Type: domaincanvas.NodeTypeImage, AssetID: assetID.String()}},
		},
	}
	request, err := imageRequest(submission)
	if err != nil {
		t.Fatal(err)
	}
	if request.UserID != 31 || request.ProjectID != projectID.String() || request.UserGroupMultiplier != "1.25000" || request.Prompt != "connected prompt" || request.OutputImageCount != 12 || len(request.ReferenceAssetIDs) != 1 || request.ReferenceAssetIDs[0] != assetID.String() {
		t.Fatalf("image request = %#v", request)
	}
	if request.TaskType != "image_edit" {
		t.Fatalf("image request with a reference must use image_edit, got %q", request.TaskType)
	}
}

func TestImageRequestDerivesTaskTypeFromConnectedReferences(t *testing.T) {
	base := GenerationSubmission{
		UserID: 31, ProjectID: uuid.New(), CanvasID: uuid.New(), NodeID: "gen", Kind: TaskKindImage,
		Node:   domaincanvas.Node{ID: "gen", Type: domaincanvas.NodeTypeImageGeneration, Payload: json.RawMessage(`{"draft":{"route_model_code":"basic","task_type":"image_edit","output_image_count":1}}`)},
		Inputs: []GenerationInput{{Role: domaincanvas.InputRolePrompt, Node: domaincanvas.Node{ID: "prompt", Type: domaincanvas.NodeTypePrompt, Payload: json.RawMessage(`{"text":"connected prompt"}`)}}},
	}
	request, err := imageRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	if request.TaskType != "text_to_image" {
		t.Fatalf("image request without references must use text_to_image, got %q", request.TaskType)
	}

	base.Node.Payload = json.RawMessage(`{"draft":{"route_model_code":"basic","task_type":"text_to_image","output_image_count":1}}`)
	base.Inputs = append(base.Inputs, GenerationInput{Role: domaincanvas.InputRoleReference, Node: domaincanvas.Node{ID: "image", Type: domaincanvas.NodeTypeImage, AssetID: uuid.NewString()}})
	request, err = imageRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	if request.TaskType != "image_edit" {
		t.Fatalf("image request with references must use image_edit, got %q", request.TaskType)
	}
}

func TestImageRequestNormalizesHiddenSizeFieldsByMode(t *testing.T) {
	base := GenerationSubmission{
		UserID: 31, ProjectID: uuid.New(), CanvasID: uuid.New(), NodeID: "gen", Kind: TaskKindImage,
		Inputs: []GenerationInput{{Role: domaincanvas.InputRolePrompt, Node: domaincanvas.Node{ID: "prompt", Type: domaincanvas.NodeTypePrompt, Payload: json.RawMessage(`{"text":"connected prompt"}`)}}},
	}
	for _, test := range []struct {
		name               string
		draft              string
		wantRequestedSize  string
		wantBaseResolution string
		wantAspectRatio    string
	}{
		{name: "auto", draft: `{"route_model_code":"basic","task_type":"text_to_image","size_mode":"auto","requested_size":"1024x1024","base_resolution":"1k","aspect_ratio":"16:9"}`},
		{name: "pixel", draft: `{"route_model_code":"basic","task_type":"text_to_image","size_mode":"pixel","requested_size":"1024x1024","base_resolution":"1k","aspect_ratio":"16:9"}`, wantRequestedSize: "1024x1024"},
		{name: "ratio", draft: `{"route_model_code":"basic","task_type":"text_to_image","size_mode":"ratio","requested_size":"1024x1024","base_resolution":"1k","aspect_ratio":"16:9"}`, wantBaseResolution: "1k", wantAspectRatio: "16:9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			base.Node = domaincanvas.Node{ID: "gen", Type: domaincanvas.NodeTypeImageGeneration, Payload: json.RawMessage(`{"draft":` + test.draft + `}`)}
			request, err := imageRequest(base)
			if err != nil {
				t.Fatal(err)
			}
			if request.RequestedSize != test.wantRequestedSize || request.BaseResolution != test.wantBaseResolution || request.AspectRatio != test.wantAspectRatio {
				t.Fatalf("normalized size fields = requested:%q base:%q ratio:%q", request.RequestedSize, request.BaseResolution, request.AspectRatio)
			}
		})
	}
}

func TestVideoRequestRebuildsCanvasSourceAndFrameInputs(t *testing.T) {
	projectID, canvasID, firstFrame := uuid.New(), uuid.New(), uuid.New()
	payload := json.RawMessage(`{"draft":{"quote_token":"quote","route_model_code":"cinema","task_type":"image_to_video","prompt_template":"hidden","duration_seconds":5,"resolution":"720p","aspect_ratio":"16:9","audio_mode":"silent","output_count":1}}`)
	submission := GenerationSubmission{UserID: 42, ProjectID: projectID, CanvasID: canvasID, NodeID: "video-gen", IdempotencyKey: "idem", Kind: TaskKindVideo, Node: domaincanvas.Node{ID: "video-gen", Type: domaincanvas.NodeTypeVideoGeneration, Payload: payload}, Inputs: []GenerationInput{
		{Role: domaincanvas.InputRolePrompt, Node: domaincanvas.Node{ID: "prompt", Type: domaincanvas.NodeTypePrompt, Payload: json.RawMessage(`{"template":"connected {{subject}}"}`)}},
		{Role: domaincanvas.InputRoleFirstFrame, Ordinal: 0, Node: domaincanvas.Node{ID: "image", Type: domaincanvas.NodeTypeImage, AssetID: firstFrame.String()}},
	}}
	request, err := videoRequest(submission)
	if err != nil {
		t.Fatal(err)
	}
	if request.UserID != 42 || request.ProjectID != projectID || request.SourceCanvasID == nil || *request.SourceCanvasID != canvasID || request.SourceCanvasNodeID != "video-gen" || request.PromptTemplate != "connected {{subject}}" || len(request.Inputs) != 1 || request.Inputs[0].AssetID != firstFrame {
		t.Fatalf("video request = %#v", request)
	}
}

func TestImageRequestRequiresAndUsesActivePromptWithBindings(t *testing.T) {
	projectID, assetID := uuid.New(), uuid.New()
	promptA := domaincanvas.Node{ID: "prompt-a", Type: domaincanvas.NodeTypePrompt, Payload: json.RawMessage(`{"text":"first prompt"}`)}
	promptB := domaincanvas.Node{ID: "prompt-b", Type: domaincanvas.NodeTypePrompt, Payload: json.RawMessage(`{"text":"让 {{@主体}} 穿着 {{$服装}}","variables":{"服装":"蓝色风衣"}}`)}
	asset := domaincanvas.Node{ID: "asset", Type: domaincanvas.NodeTypeImage, AssetID: assetID.String(), Payload: json.RawMessage(`{"name":"主体"}`)}
	base := GenerationSubmission{
		UserID: 31, ProjectID: projectID, CanvasID: uuid.New(), NodeID: "gen", Kind: TaskKindImage,
		Node:   domaincanvas.Node{ID: "gen", Type: domaincanvas.NodeTypeImageGeneration, Payload: json.RawMessage(`{"draft":{"abstract_model":"plus","task_type":"image_edit","output_image_count":1}}`)},
		Inputs: []GenerationInput{{Role: domaincanvas.InputRolePrompt, Node: promptA}, {Role: domaincanvas.InputRolePrompt, Node: promptB}, {Role: domaincanvas.InputRoleReference, Node: asset}},
	}
	if _, err := imageRequest(base); err == nil || err.Error() != "image generation node must select one connected prompt" {
		t.Fatalf("multiple prompts without an active selection error = %v", err)
	}
	base.Node.Payload = json.RawMessage(`{"active_prompt_node_id":"prompt-b","draft":{"abstract_model":"plus","task_type":"image_edit","output_image_count":1}}`)
	request, err := imageRequest(base)
	if err != nil {
		t.Fatal(err)
	}
	if request.Prompt != "让 {{@主体}} 穿着 {{$服装}}" || len(request.PromptVariables) != 1 || request.PromptVariables[0].Name != "服装" || request.PromptVariables[0].Value != "蓝色风衣" {
		t.Fatalf("selected prompt and variables = %#v", request)
	}
	if len(request.ReferenceBindings) != 1 || request.ReferenceBindings[0].Name != "主体" || request.ReferenceBindings[0].AssetID != assetID.String() {
		t.Fatalf("reference bindings = %#v", request.ReferenceBindings)
	}
	base.Inputs[1].Node.Payload = json.RawMessage(`{"text":"让 {{@主体}} 穿着 {{$服装}}","variables":{"服装":" "}}`)
	if _, err := imageRequest(base); err == nil || err.Error() != `prompt variable "服装" is not filled` {
		t.Fatalf("blank prompt variable error = %v", err)
	}
	base.Inputs[1].Node.Payload = json.RawMessage(`{"text":"让 {{@缺失}} 出现","variables":{}}`)
	if _, err := imageRequest(base); err == nil || err.Error() != `prompt reference "缺失" has no connected asset with the same name` {
		t.Fatalf("missing prompt reference error = %v", err)
	}
	base.Inputs[1].Node.Payload = promptB.Payload
	base.Inputs = append(base.Inputs, GenerationInput{Role: domaincanvas.InputRoleReference, Node: domaincanvas.Node{ID: "asset-copy", Type: domaincanvas.NodeTypeImage, AssetID: uuid.NewString(), Payload: asset.Payload}})
	if _, err := imageRequest(base); err == nil || err.Error() != `multiple connected assets are named "主体"` {
		t.Fatalf("ambiguous prompt reference error = %v", err)
	}
}

func TestVideoRequestCarriesCanvasPromptBindings(t *testing.T) {
	projectID, canvasID, firstFrame := uuid.New(), uuid.New(), uuid.New()
	submission := GenerationSubmission{
		UserID: 42, ProjectID: projectID, CanvasID: canvasID, NodeID: "video-gen", IdempotencyKey: "idem", Kind: TaskKindVideo,
		Node: domaincanvas.Node{ID: "video-gen", Type: domaincanvas.NodeTypeVideoGeneration, Payload: json.RawMessage(`{"active_prompt_node_id":"prompt","draft":{"quote_token":"quote","route_model_code":"cinema","task_type":"image_to_video","duration_seconds":5,"resolution":"720p","aspect_ratio":"16:9","audio_mode":"silent","output_count":1}}`)},
		Inputs: []GenerationInput{
			{Role: domaincanvas.InputRolePrompt, Node: domaincanvas.Node{ID: "prompt", Type: domaincanvas.NodeTypePrompt, Payload: json.RawMessage(`{"text":"让 {{@首帧}} 进入 {{$场景}}","variables":{"场景":"森林"}}`)}},
			{Role: domaincanvas.InputRoleFirstFrame, Node: domaincanvas.Node{ID: "image", Type: domaincanvas.NodeTypeImage, AssetID: firstFrame.String(), Payload: json.RawMessage(`{"name":"首帧"}`)}},
		},
	}
	request, err := videoRequest(submission)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.PromptVariables) != 1 || request.PromptVariables[0].Name != "场景" || request.PromptVariables[0].Value != "森林" {
		t.Fatalf("video prompt variables = %#v", request.PromptVariables)
	}
	if len(request.ReferenceBindings) != 1 || request.ReferenceBindings[0].Name != "首帧" || request.ReferenceBindings[0].AssetID != firstFrame {
		t.Fatalf("video reference bindings = %#v", request.ReferenceBindings)
	}
}

func TestVideoRequestCarriesReferenceMediaInputs(t *testing.T) {
	projectID, imageID, videoID, audioID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	payload := json.RawMessage(`{"draft":{"quote_token":"quote","route_model_code":"cinema","task_type":"reference_to_video","prompt_template":"hidden","duration_seconds":8,"resolution":"720p","aspect_ratio":"adaptive","audio_mode":"silent","output_count":1}}`)
	submission := GenerationSubmission{UserID: 7, ProjectID: projectID, CanvasID: uuid.New(), NodeID: "video-gen", IdempotencyKey: "idem-ref", Kind: TaskKindVideo, Node: domaincanvas.Node{ID: "video-gen", Type: domaincanvas.NodeTypeVideoGeneration, Payload: payload}, Inputs: []GenerationInput{
		{Role: domaincanvas.InputRoleReferenceImage, Ordinal: 0, Node: domaincanvas.Node{ID: "img", Type: domaincanvas.NodeTypeImage, AssetID: imageID.String()}},
		{Role: domaincanvas.InputRoleReferenceVideo, Ordinal: 1, Node: domaincanvas.Node{ID: "vid", Type: domaincanvas.NodeTypeVideo, AssetID: videoID.String()}},
		{Role: domaincanvas.InputRoleReferenceAudio, Ordinal: 2, Node: domaincanvas.Node{ID: "aud", Type: domaincanvas.NodeTypeAudio, AssetID: audioID.String()}},
	}}
	request, err := videoRequest(submission)
	if err != nil {
		t.Fatal(err)
	}
	if len(request.Inputs) != 3 {
		t.Fatalf("reference inputs = %#v", request.Inputs)
	}
	want := []struct {
		assetID uuid.UUID
		role    domainvideo.InputRole
		ordinal int
	}{{imageID, domainvideo.InputRoleReferenceImage, 0}, {videoID, domainvideo.InputRoleReferenceVideo, 1}, {audioID, domainvideo.InputRoleReferenceAudio, 2}}
	for index, expect := range want {
		got := request.Inputs[index]
		if got.AssetID != expect.assetID || got.Role != expect.role || got.Ordinal != expect.ordinal {
			t.Fatalf("input[%d] = %#v, want asset %s role %s", index, got, expect.assetID, expect.role)
		}
	}
}

func TestIntegratedInlineDraftVariablesAndStableReferenceAliases(t *testing.T) {
	assetID := uuid.NewString()
	submission := GenerationSubmission{Node: domaincanvas.Node{Type: domaincanvas.NodeTypeImageV2, Payload: json.RawMessage(`{"title":"display","draft":{"route_model_code":"model","prompt_template":"${subject} with {{@stable}}","variables":{"subject":"cat"}}}`)}, Inputs: []GenerationInput{{Role: domaincanvas.InputRoleReference, Node: domaincanvas.Node{Type: domaincanvas.NodeTypeImageV2, AssetID: assetID, Payload: json.RawMessage(`{"name":"stable","title":"renamed display"}`)}}}}
	request, err := imageRequest(submission)
	if err != nil {
		t.Fatal(err)
	}
	if request.PromptTemplate != "{{$subject}} with {{@stable}}" || len(request.PromptVariables) != 1 || request.PromptVariables[0].Value != "cat" || len(request.ReferenceBindings) != 1 || request.ReferenceBindings[0].AssetID != assetID {
		t.Fatalf("inline request %#v", request)
	}
	submission.Node.Type = domaincanvas.NodeTypeVideoV2
	submission.Node.Payload = json.RawMessage(`{"draft":{"quote_token":"real-quote","prompt_template":"${subject}","variables":{"subject":"cat"}}}`)
	video, err := videoRequest(submission)
	if err != nil {
		t.Fatal(err)
	}
	if video.QuoteToken != "real-quote" || video.PromptTemplate != "{{$subject}}" || len(video.PromptVariables) != 1 {
		t.Fatalf("video inline %#v", video)
	}
	submission.Inputs = append(submission.Inputs, GenerationInput{Role: domaincanvas.InputRoleReference, Node: domaincanvas.Node{AssetID: uuid.NewString(), Payload: json.RawMessage(`{"name":"stable"}`)}})
	submission.Node.Type = domaincanvas.NodeTypeImageV2
	submission.Node.Payload = json.RawMessage(`{"draft":{"route_model_code":"model","prompt_template":"{{@stable}}"}}`)
	if _, err := imageRequest(submission); err == nil {
		t.Fatal("ambiguous aliases accepted")
	}
}

type canvasNoReadBackend struct {
	storage.Backend
	reads int
}

func (b *canvasNoReadBackend) Get(context.Context, string) ([]byte, error) {
	b.reads++
	return nil, errors.New("quote attempted binary download")
}

type canvasAliasStore struct {
	asset   media.Asset
	alias   domainassets.ReferenceAsset
	imports int
}

func (s *canvasAliasStore) GetByUserAndHash(context.Context, int64, string) (domainassets.ReferenceAsset, error) {
	return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
}
func (s *canvasAliasStore) Save(context.Context, int64, domainassets.ReferenceAsset) error {
	return nil
}
func (s *canvasAliasStore) DeleteByUserAndID(context.Context, int64, string) error { return nil }
func (s *canvasAliasStore) GetByUserAndID(_ context.Context, user int64, id string) (domainassets.ReferenceAsset, error) {
	if user != s.asset.UserID || id != s.alias.ID || id == "" {
		return domainassets.ReferenceAsset{}, repoerr.ErrNotFound
	}
	return s.alias, nil
}
func (s *canvasAliasStore) ImportMediaAssetAlias(_ context.Context, user int64, a media.Asset) (domainassets.ReferenceAsset, error) {
	s.imports++
	if s.alias.ID == "" {
		s.alias = domainassets.ReferenceAsset{ID: uuid.NewString(), MediaAssetID: a.ID.String(), Status: "ready", MimeType: a.MIMEType, ObjectKey: a.ObjectKey, Name: "different persisted name"}
	}
	return s.alias, nil
}
func (s *canvasAliasStore) GetAsset(_ context.Context, user int64, id uuid.UUID) (media.Asset, error) {
	if user != s.asset.UserID || id != s.asset.ID {
		return media.Asset{}, repoerr.ErrNotFound
	}
	return s.asset, nil
}

type canvasImagePreflight struct{ estimated, created image.CreateRequest }

func (p *canvasImagePreflight) PreflightMetadata(_ context.Context, r image.CreateRequest) (domainbilling.EstimateResult, error) {
	p.estimated = r
	return domainbilling.EstimateResult{}, nil
}
func (p *canvasImagePreflight) CreateTask(_ context.Context, r image.CreateRequest) (image.Task, error) {
	p.created = r
	return image.Task{ID: uuid.NewString(), Status: image.StatusQueued}, nil
}
func (p *canvasImagePreflight) GetByID(context.Context, int64, string) (image.Task, error) {
	return image.Task{}, nil
}
func TestCanvasCanonicalReferenceAliasEstimateGenerateParity(t *testing.T) {
	ctx := context.Background()
	store := &canvasAliasStore{asset: media.Asset{ID: uuid.New(), UserID: 31, MediaType: "image", Status: "ready", MIMEType: "image/png", ObjectKey: "canonical.png"}}
	backend := &canvasNoReadBackend{Backend: storage.NewLocalBackend(t.TempDir())}
	refs := assetservice.NewServiceWithStoreAndBackend(config.StorageConfig{}, config.GenerationLimitsConfig{}, store, backend)
	images := &canvasImagePreflight{}
	g := NewTaskGenerator(images, nil, nil)
	g.SetReferenceAssets(refs, store)
	submission := GenerationSubmission{UserID: 31, Kind: TaskKindImage, Node: domaincanvas.Node{Type: domaincanvas.NodeTypeImageV2, Payload: json.RawMessage(`{"draft":{"route_model_code":"model","prompt_template":"${subject} with {{@stable}}","variables":{"subject":"cat"}}}`)}, Inputs: []GenerationInput{{Role: domaincanvas.InputRoleReference, Node: domaincanvas.Node{AssetID: store.asset.ID.String(), Payload: json.RawMessage(`{"name":"stable","title":"display rename"}`)}}}}
	if _, err := g.Estimate(ctx, submission); err != nil {
		t.Fatal(err)
	}
	if _, err := g.Generate(ctx, submission); err != nil {
		t.Fatal(err)
	}
	if backend.reads != 0 {
		t.Fatalf("estimate loaded binary objects %d times", backend.reads)
	}
	a, b := images.estimated, images.created
	a.TaskID = ""
	b.TaskID = ""
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("estimate/create differ: %#v %#v", a, b)
	}
	if a.ReferenceAssetIDs[0] == store.asset.ID.String() || a.ReferenceAssetIDs[0] != store.alias.ID || a.ReferenceBindings[0].AssetID != store.alias.ID || a.ReferenceBindings[0].Name != "stable" {
		t.Fatalf("canonical alias mapping failed: %#v", a)
	}
	for _, tc := range []struct {
		name    string
		user    int64
		status  string
		missing bool
		code    int
	}{{"foreign", 32, "ready", false, 404}, {"missing", 31, "ready", true, 404}, {"unavailable", 31, "processing", false, 409}} {
		t.Run(tc.name, func(t *testing.T) {
			s := submission
			s.UserID = tc.user
			store.asset.Status = tc.status
			if tc.missing {
				s.Inputs = append([]GenerationInput(nil), s.Inputs...)
				s.Inputs[0].Node.AssetID = uuid.NewString()
			}
			_, err := g.Estimate(ctx, s)
			var e *errs.Error
			if !errors.As(err, &e) || e.StatusCode != tc.code || e.Details["field"] != "reference_asset_ids" {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}
