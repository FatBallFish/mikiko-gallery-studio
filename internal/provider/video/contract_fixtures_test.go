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

func TestPricingContractFixtureUsesProtectedLaunchRates(t *testing.T) {
	path := filepath.Join("..", "..", "..", "docs", "tech", "contracts", "video-provider-pricing-v1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read pricing contract fixture: %v", err)
	}

	contract, err := ParsePricingContract(raw)
	if err != nil {
		t.Fatalf("ParsePricingContract() error = %v", err)
	}
	if contract.TargetGrossMarginRate != "0.25000" || contract.ProviderCostBufferRate != "0.10000" {
		t.Fatalf("pricing protection = margin %s buffer %s", contract.TargetGrossMarginRate, contract.ProviderCostBufferRate)
	}
	if contract.MinimumNetPointIncomeCNY != "0.25260" {
		t.Fatalf("minimum_net_point_income_cny = %s, want 0.25260", contract.MinimumNetPointIncomeCNY)
	}

	wantRates := map[string]string{
		"MiniMax-H3/768p":                  "3.10000",
		"MiniMax-H3/2k":                    "4.80000",
		"doubao-seedance-2-5-260628/480p":  "4.00000",
		"doubao-seedance-2-5-260628/720p":  "8.90000",
		"doubao-seedance-2-0-260128/480p":  "2.80000",
		"doubao-seedance-2-0-260128/720p":  "5.90000",
		"doubao-seedance-2-0-260128/1080p": "14.60000",
		"doubao-seedance-2-0-260128/4k":    "29.50000",
	}
	for _, rate := range contract.InitialSalesRates {
		key := rate.ModelCode + "/" + rate.Resolution
		want, ok := wantRates[key]
		if !ok {
			continue
		}
		if rate.OutputSecondPoints != want {
			t.Errorf("%s output_second_points = %s, want %s", key, rate.OutputSecondPoints, want)
		}
		if rate.ValidationStatus != ValidationUntested || rate.Enabled {
			t.Errorf("%s must remain disabled and untested before real-account cost PoC", key)
		}
		delete(wantRates, key)
	}
	for key := range wantRates {
		t.Errorf("launch pricing rate %q missing", key)
	}
}
