package video

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCapabilityContractFixtureContainsLaunchModels(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "tech", "contracts", "video-provider-capabilities-v1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read capability contract fixture: %v", err)
	}

	contract, err := ParseCapabilityContract(raw)
	if err != nil {
		t.Fatalf("ParseCapabilityContract() error = %v", err)
	}
	if contract.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", contract.SchemaVersion)
	}

	wantModels := map[string]bool{
		"doubao-seedance-2-5-260628": false,
		"doubao-seedance-2-0-260128": false,
		"MiniMax-H3":                 false,
	}
	for _, model := range contract.Models {
		if _, ok := wantModels[model.ModelCode]; !ok {
			continue
		}
		wantModels[model.ModelCode] = true
		if model.ValidationStatus != ValidationUntested {
			t.Errorf("model %q validation_status = %q, want %q until a real-account PoC passes", model.ModelCode, model.ValidationStatus, ValidationUntested)
		}
		if model.ProviderNativeMaxN < 1 || model.ProviderNativeMaxN > 10 {
			t.Errorf("model %q provider_native_max_n = %d, want 1..10", model.ModelCode, model.ProviderNativeMaxN)
		}
		if len(model.TaskTypes) == 0 {
			t.Errorf("model %q has no task types", model.ModelCode)
		}
		if len(model.OutputFormats) == 0 {
			t.Errorf("model %q has no output formats", model.ModelCode)
		}
		if model.ExecutionMode != ExecutionModePoll && model.ExecutionMode != ExecutionModeCallbackPoll {
			t.Errorf("model %q execution_mode = %q", model.ModelCode, model.ExecutionMode)
		}
	}
	for modelCode, found := range wantModels {
		if !found {
			t.Errorf("launch model %q missing from fixture", modelCode)
		}
	}
}
