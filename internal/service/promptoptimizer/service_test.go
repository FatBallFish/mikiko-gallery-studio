package promptoptimizer_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	textprovider "github.com/fatballfish/pic-gallery/internal/provider/text"
	promptoptimizer "github.com/fatballfish/pic-gallery/internal/service/promptoptimizer"
	textmodelservice "github.com/fatballfish/pic-gallery/internal/service/textmodel"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type fakeOptimizer struct {
	calls    int
	request  textprovider.OptimizeRequest
	optimize func(textprovider.OptimizeRequest) textprovider.OptimizeResponse
	result   textprovider.OptimizeResponse
	err      error
}

func (f *fakeOptimizer) Optimize(_ context.Context, request textprovider.OptimizeRequest) (textprovider.OptimizeResponse, error) {
	f.calls++
	f.request = request
	if f.optimize != nil {
		return f.optimize(request), f.err
	}
	return f.result, f.err
}

func TestServiceProtectsAndRestoresPromptTemplateTokens(t *testing.T) {
	ctx := t.Context()
	textStore := textmodelservice.NewMemoryStore()
	textService := textmodelservice.NewService(textStore, "encryption-key")
	configureDefaultTextModel(t, ctx, textService, "gpt-template")
	optimizer := &fakeOptimizer{optimize: func(request textprovider.OptimizeRequest) textprovider.OptimizeResponse {
		if strings.Contains(request.Prompt, "{{@") || strings.Contains(request.Prompt, "{{$") || !strings.Contains(request.Prompt, "MGS_TOKEN") {
			t.Fatalf("provider prompt is not protected: %q", request.Prompt)
		}
		if !strings.Contains(request.SystemPrompt, "placeholder") || !strings.Contains(request.SystemPrompt, "must not") {
			t.Fatalf("system prompt does not protect sentinels: %q", request.SystemPrompt)
		}
		return textprovider.OptimizeResponse{Text: "优化后的 " + request.Prompt}
	}}
	svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) { return optimizer, nil })
	template := "让 {{@主体}} 位于 {{$地点}}，再次参考 {{@主体}}"
	estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 43, Prompt: template})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 43, Prompt: template, Quote: estimate.Quote})
	if err != nil {
		t.Fatal(err)
	}
	if result.OptimizedPrompt != "优化后的 "+template || strings.Contains(result.OptimizedPrompt, "MGS_TOKEN") {
		t.Fatalf("optimized prompt = %q", result.OptimizedPrompt)
	}
}

func TestServiceRejectsOptimizationThatDamagesOrInjectsTemplateSentinels(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(string) string
	}{
		{name: "delete", mutate: func(value string) string {
			start := strings.Index(value, "⟦MGS_TOKEN_")
			end := strings.Index(value[start:], "⟧")
			return value[:start] + value[start+end+len("⟧"):]
		}},
		{name: "duplicate", mutate: func(value string) string {
			start := strings.Index(value, "⟦MGS_TOKEN_")
			end := strings.Index(value[start:], "⟧") + start + len("⟧")
			return value + " " + value[start:end]
		}},
		{name: "modify", mutate: func(value string) string { return strings.Replace(value, "MGS_TOKEN_", "MGS_BROKEN_", 1) }},
		{name: "inject", mutate: func(value string) string { return value + " ⟦MGS_TOKEN_UNKNOWN_9999⟧" }},
		{name: "inject raw placeholder", mutate: func(value string) string { return value + " {{@额外资源}}" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			textStore := textmodelservice.NewMemoryStore()
			textService := textmodelservice.NewService(textStore, "encryption-key")
			configureDefaultTextModel(t, ctx, textService, "gpt-"+test.name)
			optimizer := &fakeOptimizer{optimize: func(request textprovider.OptimizeRequest) textprovider.OptimizeResponse {
				return textprovider.OptimizeResponse{Text: test.mutate(request.Prompt)}
			}}
			svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) { return optimizer, nil })
			template := "使用 {{@主体}} 和 {{$地点}} 生成画面"
			estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 44, Prompt: template})
			if err != nil {
				t.Fatal(err)
			}
			_, err = svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 44, Prompt: template, Quote: estimate.Quote})
			var appErr *errs.Error
			if !errors.As(err, &appErr) || appErr.Code != "INVALID_OPTIMIZATION_RESULT" {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestServiceRejectsOptimizationResultOverGenerationPromptLimit(t *testing.T) {
	ctx := t.Context()
	textStore := textmodelservice.NewMemoryStore()
	textService := textmodelservice.NewService(textStore, "encryption-key")
	configureDefaultTextModel(t, ctx, textService, "gpt-too-long")
	optimizer := &fakeOptimizer{result: textprovider.OptimizeResponse{Text: strings.Repeat("a", 4001)}}
	svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) {
		return optimizer, nil
	})
	estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 45, Prompt: "a concise portrait prompt"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 45, Prompt: "a concise portrait prompt", Quote: estimate.Quote})
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.Code != "INVALID_OPTIMIZATION_RESULT" {
		t.Fatalf("error = %#v", err)
	}
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
