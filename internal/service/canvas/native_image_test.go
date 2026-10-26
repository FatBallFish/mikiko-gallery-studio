package canvas

import (
	"encoding/json"
	"testing"

	domaincanvas "github.com/fatballfish/pic-gallery/internal/domain/canvas"
	"github.com/fatballfish/pic-gallery/internal/domain/nativeimage"
	"github.com/google/uuid"
)

func TestCanvasImageNativeOptionsPreserved(t *testing.T) {
	submission := GenerationSubmission{
		Kind:   TaskKindImage,
		Node:   domaincanvas.Node{Type: domaincanvas.NodeTypeImageV2, Payload: json.RawMessage(`{"draft":{"route_model_code":"seedream","operation":"edit","prompt":"change this object","native_options":{"watermark":false,"seedream":{"optimize_prompt_mode":"fast"},"annotations":[{"reference_index":1,"point":[400,500]}]}}}`)},
		Inputs: []GenerationInput{{Role: domaincanvas.InputRoleReference, Node: domaincanvas.Node{AssetID: uuid.NewString()}}},
	}
	req, err := imageRequest(submission)
	if err != nil {
		t.Fatal(err)
	}
	if req.Operation != nativeimage.OperationEdit || req.TaskType != "image_edit" || req.NativeOptions == nil || req.NativeOptions.Watermark == nil || *req.NativeOptions.Watermark || req.NativeOptions.Seedream == nil || req.NativeOptions.Seedream.OptimizePromptMode != "fast" || len(req.NativeOptions.Annotations) != 1 {
		t.Fatalf("native draft was lost: %#v", req)
	}
	if req.NativeOptions.Annotations[0].ReferenceIndex != 1 {
		t.Fatal("reference order was lost")
	}
}

func TestCanvasLayerDecompositionAllowsNoPrompt(t *testing.T) {
	submission := GenerationSubmission{Kind: TaskKindImage, Node: domaincanvas.Node{Type: domaincanvas.NodeTypeImageV2, Payload: json.RawMessage(`{"draft":{"route_model_code":"seedream","operation":"layer_decomposition"}}`)}, Inputs: []GenerationInput{{Role: domaincanvas.InputRoleReference, Node: domaincanvas.Node{AssetID: uuid.NewString()}}}}
	req, err := imageRequest(submission)
	if err != nil {
		t.Fatal(err)
	}
	if req.Operation != nativeimage.OperationLayerDecomposition || req.OutputImageCount != 1 || req.TaskType != "image_edit" || req.Prompt != "" {
		t.Fatalf("unexpected decomposition request: %#v", req)
	}
}
