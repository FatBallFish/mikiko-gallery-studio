package imagetask

import (
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"reflect"
	"testing"
)

func TestImageRetryPreservesFullRequest(t *testing.T) {
	seed := int64(43)
	original := domainimagetask.Task{UserID: 21, ProjectID: "project", APIKeyID: 5, SourceChannel: "web", AbstractModel: "image", RouteModelCode: "route", TaskType: "image_edit", Prompt: "expanded", PromptTemplate: "{{style}} @ref", PromptTemplateVersion: 1, NegativePrompt: "avoid", SizeMode: "custom", RequestedSize: "1024x1536", BaseResolution: "1k", AspectRatio: "2:3", Quality: "high", OutputFormat: "webp", Background: "transparent", OutputCompression: 81, Moderation: "low", OutputImageCount: 3, ReferenceImageCount: 1, ReferenceAssetIDs: []string{"asset"}, ReferenceStrength: 72, Seed: &seed, ResponseMode: "async", SavePolicy: "private", PromptBindingSnapshot: domainimagetask.PromptBindingSnapshot{Variables: []domainimagetask.PromptVariableInput{{Name: "style", Value: "ink"}}, References: []domainimagetask.PromptReferenceBinding{{Name: "ref", AssetID: "asset", Index: 1}}}}
	req := imageRetryRequest(original)
	if req.TaskID != "" || req.Quality != "high" || req.OutputFormat != "webp" || req.Background != "transparent" || req.OutputCompression != 81 || req.Moderation != "low" || req.Prompt != original.PromptTemplate || req.NegativePrompt != "avoid" || req.OutputImageCount != 3 || req.ReferenceStrength != 72 || *req.Seed != 43 {
		t.Fatalf("lost request fields: %+v", req)
	}
	if !reflect.DeepEqual(req.PromptVariables, original.PromptBindingSnapshot.Variables) || len(req.ReferenceBindings) != 1 || req.ReferenceBindings[0].AssetID != "asset" {
		t.Fatalf("lost bindings: %+v", req)
	}
}
