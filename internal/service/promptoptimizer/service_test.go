package promptoptimizer_test

import (
	"context"
	"errors"
	"regexp"
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

func TestServiceSelectsSystemPromptByMediaType(t *testing.T) {
	if prompt := promptoptimizer.SystemPromptForMedia("", ""); prompt != promptoptimizer.SystemPromptForMedia("image", "") {
		t.Fatalf("default media type must use the image system prompt")
	}
	video := promptoptimizer.SystemPromptForMedia("video", "")
	for _, want := range []string{"video-generation", "镜头 1", "one camera move", "不要字幕", "placeholder", "never modify"} {
		if !strings.Contains(video, want) {
			t.Fatalf("video system prompt missing %q: %q", want, video)
		}
	}
	if strings.Contains(promptoptimizer.SystemPromptForMedia("image", ""), "video-generation") {
		t.Fatalf("image system prompt must not carry video instructions")
	}
}

func TestVideoGuideFollowsRouteModelCode(t *testing.T) {
	tests := []struct {
		code string
		want promptoptimizer.VideoGuideKind
	}{
		{code: "", want: promptoptimizer.VideoGuideUniversal},
		{code: "gpt-video-x", want: promptoptimizer.VideoGuideUniversal},
		{code: "seedance-2.5", want: promptoptimizer.VideoGuideSeedance25},
		{code: "Seedance 2.5", want: promptoptimizer.VideoGuideSeedance25},
		{code: "doubao-seedance-2-5-260628", want: promptoptimizer.VideoGuideSeedance25},
		{code: "seedance-2.0", want: promptoptimizer.VideoGuideSeedance20},
		{code: "seedance-2.0-mini", want: promptoptimizer.VideoGuideSeedance20},
		{code: "minimax-h3", want: promptoptimizer.VideoGuideMiniMaxH3},
		{code: "minimax-h3-max", want: promptoptimizer.VideoGuideMiniMaxH3},
		{code: "MiniMax-H3", want: promptoptimizer.VideoGuideMiniMaxH3},
		{code: "wan3.0-video", want: promptoptimizer.VideoGuideWan30},
	}
	for _, test := range tests {
		if got := promptoptimizer.VideoGuideForRouteModel(test.code); got != test.want {
			t.Fatalf("VideoGuideForRouteModel(%q) = %q, want %q", test.code, got, test.want)
		}
	}
	prompts := make(map[string]struct{})
	for _, guide := range []promptoptimizer.VideoGuideKind{
		promptoptimizer.VideoGuideUniversal, promptoptimizer.VideoGuideSeedance25,
		promptoptimizer.VideoGuideSeedance20, promptoptimizer.VideoGuideWan30, promptoptimizer.VideoGuideMiniMaxH3,
	} {
		prompt := promptoptimizer.SystemPromptForVideo(guide)
		for _, want := range []string{"placeholder", "never modify", "2000 characters", "compress deliberately"} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("guide %q prompt missing %q", guide, want)
			}
		}
		prompts[prompt] = struct{}{}
	}
	if len(prompts) != 5 {
		t.Fatalf("guide prompts must be distinct, got %d unique", len(prompts))
	}
}

func TestServiceOptimizeSelectsGuidePromptByRouteModel(t *testing.T) {
	tests := []struct {
		name     string
		model    string
		contains string
		guide    promptoptimizer.VideoGuideKind
	}{
		{name: "minimax", model: "minimax-h3-max", contains: "MiniMax H3", guide: promptoptimizer.VideoGuideMiniMaxH3},
		{name: "seedance25", model: "seedance-2.5", contains: "Seedance 2.5", guide: promptoptimizer.VideoGuideSeedance25},
		{name: "seedance20", model: "seedance-2.0", contains: "Seedance 2.0", guide: promptoptimizer.VideoGuideSeedance20},
		{name: "wan", model: "wan3.0-video", contains: "万相3.0", guide: promptoptimizer.VideoGuideWan30},
		{name: "fallback", model: "future-model", contains: "director-style structured shooting brief", guide: promptoptimizer.VideoGuideUniversal},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := t.Context()
			textStore := textmodelservice.NewMemoryStore()
			textService := textmodelservice.NewService(textStore, "encryption-key")
			configureDefaultTextModel(t, ctx, textService, "gpt-guide")
			optimizer := &fakeOptimizer{result: textprovider.OptimizeResponse{Text: "优化后的提示词"}}
			svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) { return optimizer, nil })
			estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 49, Prompt: "一只猫在屋顶跳跃", MediaType: "video", RouteModelCode: test.model})
			if err != nil {
				t.Fatal(err)
			}
			if estimate.Guide != string(test.guide) {
				t.Fatalf("estimate guide = %q, want %q", estimate.Guide, test.guide)
			}
			result, err := svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 49, Prompt: "一只猫在屋顶跳跃", Quote: estimate.Quote, MediaType: "video", RouteModelCode: test.model})
			if err != nil {
				t.Fatal(err)
			}
			if result.Guide != string(test.guide) {
				t.Fatalf("optimize guide = %q, want %q", result.Guide, test.guide)
			}
			if !strings.Contains(optimizer.request.SystemPrompt, test.contains) {
				t.Fatalf("route model %q must select the %q guide prompt, got %q", test.model, test.contains, optimizer.request.SystemPrompt)
			}
		})
	}
}

func TestImageRequestsCarryNoGuide(t *testing.T) {
	ctx := t.Context()
	textStore := textmodelservice.NewMemoryStore()
	textService := textmodelservice.NewService(textStore, "encryption-key")
	configureDefaultTextModel(t, ctx, textService, "gpt-image-guide")
	optimizer := &fakeOptimizer{result: textprovider.OptimizeResponse{Text: "optimized image prompt"}}
	svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) { return optimizer, nil })
	estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 50, Prompt: "a portrait in rain", MediaType: "image", RouteModelCode: "seedance-2.5"})
	if err != nil {
		t.Fatal(err)
	}
	if estimate.Guide != "" {
		t.Fatalf("image estimate must not carry a guide, got %q", estimate.Guide)
	}
	result, err := svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 50, Prompt: "a portrait in rain", Quote: estimate.Quote, MediaType: "image", RouteModelCode: "seedance-2.5"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Guide != "" || strings.Contains(optimizer.request.SystemPrompt, "video-generation") {
		t.Fatalf("image optimize must not carry a guide or video prompt, guide %q system %q", result.Guide, optimizer.request.SystemPrompt)
	}
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
		if !strings.Contains(request.SystemPrompt, "placeholder") || !strings.Contains(request.SystemPrompt, "never modify") {
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

// The upstream rewriter legitimately re-references the same asset across
// shots, so a repeated sentinel must survive restore as a repeated template
// placeholder — this exact shape failed real optimizations on 2026-10-02.
func TestServiceRestoresRepeatedAssetReferences(t *testing.T) {
	ctx := t.Context()
	textStore := textmodelservice.NewMemoryStore()
	textService := textmodelservice.NewService(textStore, "encryption-key")
	configureDefaultTextModel(t, ctx, textService, "gpt-repeat-ref")
	optimizer := &fakeOptimizer{optimize: func(request textprovider.OptimizeRequest) textprovider.OptimizeResponse {
		// Echo the protected prompt with the LAST sentinel copied into a
		// second shot paragraph, mirroring the real model output.
		start := strings.LastIndex(request.Prompt, "⟦MGS_TOKEN_")
		end := strings.Index(request.Prompt[start:], "⟧") + start + len("⟧")
		sentinel := request.Prompt[start:end]
		return textprovider.OptimizeResponse{Text: "镜头1：" + request.Prompt + " 镜头2继续延续 " + sentinel + " 的光影。"}
	}}
	svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) { return optimizer, nil })
	template := "让 {{@主体}} 奔跑，延续 {{@参考视频}} 的光影"
	estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 47, Prompt: template})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 47, Prompt: template, Quote: estimate.Quote})
	if err != nil {
		t.Fatalf("repeated reference must restore: %v", err)
	}
	want := "镜头1：让 {{@主体}} 奔跑，延续 {{@参考视频}} 的光影 镜头2继续延续 {{@参考视频}} 的光影。"
	if result.OptimizedPrompt != want {
		t.Fatalf("optimized prompt = %q, want %q", result.OptimizedPrompt, want)
	}
	if strings.Contains(result.OptimizedPrompt, "MGS_TOKEN") {
		t.Fatalf("sentinel leaked into restored prompt: %q", result.OptimizedPrompt)
	}
}

func TestServiceStillRestoresExactSingleReferences(t *testing.T) {
	ctx := t.Context()
	textStore := textmodelservice.NewMemoryStore()
	textService := textmodelservice.NewService(textStore, "encryption-key")
	configureDefaultTextModel(t, ctx, textService, "gpt-exact-ref")
	optimizer := &fakeOptimizer{optimize: func(request textprovider.OptimizeRequest) textprovider.OptimizeResponse {
		return textprovider.OptimizeResponse{Text: "优化后的 " + request.Prompt}
	}}
	svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) { return optimizer, nil })
	template := "使用 {{@主体}} 和 {{$地点}} 生成画面"
	estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 48, Prompt: template})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 48, Prompt: template, Quote: estimate.Quote})
	if err != nil || result.OptimizedPrompt != "优化后的 "+template {
		t.Fatalf("exact restore = %q, err %v", result.OptimizedPrompt, err)
	}
}

// A real 3000+ character prompt referenced the same asset 17 times; the
// protector must collapse repeated placeholders onto one sentinel so the
// rewriter only has to preserve a single marker per asset.
func TestServiceSharesOneSentinelAcrossRepeatedReferences(t *testing.T) {
	ctx := t.Context()
	textStore := textmodelservice.NewMemoryStore()
	textService := textmodelservice.NewService(textStore, "encryption-key")
	configureDefaultTextModel(t, ctx, textService, "gpt-shared-sentinel")
	var providerPrompt string
	optimizer := &fakeOptimizer{optimize: func(request textprovider.OptimizeRequest) textprovider.OptimizeResponse {
		providerPrompt = request.Prompt
		return textprovider.OptimizeResponse{Text: "优化后 " + request.Prompt}
	}}
	svc := promptoptimizer.NewService(textService, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) { return optimizer, nil })
	template := "让 {{@主体}} 奔跑，{{@主体}} 跳跃，再让 {{@主体}} 落地，场景在 {{$地点}}"
	estimate, err := svc.Estimate(ctx, promptoptimizer.EstimateRequest{UserID: 51, Prompt: template})
	if err != nil {
		t.Fatal(err)
	}
	result, err := svc.Optimize(ctx, promptoptimizer.OptimizeRequest{UserID: 51, Prompt: template, Quote: estimate.Quote})
	if err != nil {
		t.Fatal(err)
	}
	sentinelSet := map[string]struct{}{}
	for _, found := range regexp.MustCompile(`⟦MGS_TOKEN_[a-z0-9]+_\d{4}⟧`).FindAllString(providerPrompt, -1) {
		sentinelSet[found] = struct{}{}
	}
	if len(sentinelSet) != 2 {
		t.Fatalf("three {{@主体}} + one {{$地点}} must share two distinct sentinels, got %d in %q", len(sentinelSet), providerPrompt)
	}
	if got := strings.Count(result.OptimizedPrompt, "{{@主体}}"); got != 3 {
		t.Fatalf("restored prompt must keep all three references, got %d: %q", got, result.OptimizedPrompt)
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
