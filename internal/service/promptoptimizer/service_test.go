package promptoptimizer_test

import (
	"context"
	"testing"

	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	textprovider "github.com/fatballfish/pic-gallery/internal/provider/text"
	promptoptimizer "github.com/fatballfish/pic-gallery/internal/service/promptoptimizer"
	textmodelservice "github.com/fatballfish/pic-gallery/internal/service/textmodel"
)

type fakeOptimizer struct {
	calls  int
	result textprovider.OptimizeResponse
	err    error
}

func (f *fakeOptimizer) Optimize(context.Context, textprovider.OptimizeRequest) (textprovider.OptimizeResponse, error) {
	f.calls++
	return f.result, f.err
}

func TestServiceEstimatesZeroAndPersistsSuccessfulOptimization(t *testing.T) {
	ctx := context.Background()
	textStore := textmodelservice.NewMemoryStore()
	textService := textmodelservice.NewService(textStore, "encryption-key")
	configureDefaultTextModel(t, ctx, textService, "gpt-test")
	optimizer := &fakeOptimizer{result: textprovider.OptimizeResponse{Text: "A detailed cinematic portrait", InputTokens: 12, OutputTokens: 8, RequestID: "req-1"}}
	svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) {
		return optimizer, nil
	})

	estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 42, Prompt: "a portrait in rain"})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if estimate.EstimatedPoints != "0.00000" || estimate.Quote == "" || estimate.Model.ModelCode != "gpt-test" {
		t.Fatalf("unexpected estimate %#v", estimate)
	}
	result, err := svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 42, Prompt: "a portrait in rain", Quote: estimate.Quote})
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}
	if result.OptimizedPrompt != "A detailed cinematic portrait" || result.ActualPoints != "0.00000" || optimizer.calls != 1 {
		t.Fatalf("unexpected result %#v calls=%d", result, optimizer.calls)
	}
	run, err := textStore.GetOptimizationRun(ctx, result.RunID)
	if err != nil {
		t.Fatalf("GetOptimizationRun: %v", err)
	}
	if run.Status != "succeeded" || run.InputTokens != 12 || run.OutputTokens != 8 || run.ProviderRequestID != "req-1" || run.ActualPoints != "0.00000" {
		t.Fatalf("unexpected persisted run %#v", run)
	}
}

func TestServiceRejectsChangedPromptAndStaleDefault(t *testing.T) {
	ctx := context.Background()
	textStore := textmodelservice.NewMemoryStore()
	textService := textmodelservice.NewService(textStore, "encryption-key")
	configureDefaultTextModel(t, ctx, textService, "model-a")
	optimizer := &fakeOptimizer{result: textprovider.OptimizeResponse{Text: "optimized"}}
	svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) {
		return optimizer, nil
	})
	estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 7, Prompt: "original prompt"})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if _, err := svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 7, Prompt: "changed prompt", Quote: estimate.Quote}); err == nil {
		t.Fatal("expected changed prompt to invalidate quote")
	}
	configureDefaultTextModel(t, ctx, textService, "model-b")
	if _, err := svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 7, Prompt: "original prompt", Quote: estimate.Quote}); err == nil {
		t.Fatal("expected changed default model to invalidate quote")
	}
	if optimizer.calls != 0 {
		t.Fatalf("optimizer must not run for invalid quote, calls=%d", optimizer.calls)
	}
}

func configureDefaultTextModel(t *testing.T, ctx context.Context, svc *textmodelservice.Service, modelCode string) {
	t.Helper()
	accounts, err := svc.ListAccounts(ctx)
	if err != nil {
		t.Fatalf("ListAccounts: %v", err)
	}
	accountID := int64(0)
	if len(accounts) == 0 {
		account, err := svc.CreateAccount(ctx, domaintextmodel.AccountWriteRequest{
			Name: "Primary", PlatformType: domaintextmodel.PlatformOpenAICompatible,
			APIStyle: domaintextmodel.APIStyleResponses, BaseURL: "https://text.example.com",
			Enabled: true, Secrets: map[string]string{"api_key": "test-secret"},
		})
		if err != nil {
			t.Fatalf("CreateAccount: %v", err)
		}
		accountID = account.ID
	} else {
		accountID = accounts[0].ID
	}
	model, err := svc.CreateModel(ctx, domaintextmodel.ModelWriteRequest{
		AccountID: accountID, ModelCode: modelCode, DisplayName: modelCode,
		InputPricePerMTok: "0", OutputPricePerMTok: "0", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateModel: %v", err)
	}
	if _, err := svc.SetDefaultModel(ctx, model.ID); err != nil {
		t.Fatalf("SetDefaultModel: %v", err)
	}
}
