package canvas

import (
	"encoding/json"
	"testing"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
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
