package imagetask

import (
	"encoding/json"
	"testing"
)

func TestTaskPromptTemplateRuntimeFieldsStayInternal(t *testing.T) {
	task := Task{
		Prompt:                "穿着 蓝色风衣",
		PromptTemplate:        "穿着 {{$服装}}",
		PromptTemplateVersion: 1,
		PromptBindingSnapshot: PromptBindingSnapshot{
			References:    []PromptReferenceBinding{{Name: "主体", AssetID: "asset-1", Index: 1}},
			VariableNames: []string{"服装"},
		},
		NegativePrompt: "historical internal value",
	}
	payload, err := json.Marshal(task)
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	for _, key := range []string{"prompt_template", "prompt_template_version", "prompt_binding_snapshot", "negative_prompt"} {
		if _, exists := decoded[key]; exists {
			t.Fatalf("runtime field %s must not be exposed in task JSON", key)
		}
	}
}
