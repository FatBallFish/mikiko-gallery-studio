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
	payload := json.RawMessage(`{"draft":{"abstract_model":"plus","task_type":"text_to_image","prompt":"hidden prompt","output_image_count":2,"user_group_multiplier":"0.1","reference_asset_ids":["forged"]}}`)
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
	if request.UserID != 31 || request.ProjectID != projectID.String() || request.UserGroupMultiplier != "1.25000" || request.Prompt != "connected prompt" || len(request.ReferenceAssetIDs) != 1 || request.ReferenceAssetIDs[0] != assetID.String() {
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
