package promptoptimizer

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/domain/prompttemplate"
	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	textprovider "github.com/fatballfish/pic-gallery/internal/provider/text"
	textopenai "github.com/fatballfish/pic-gallery/internal/provider/text/openai"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/google/uuid"
)

const (
	zeroPoints = "0.00000"
	quoteTTL   = 5 * time.Minute
)

type DefaultModelResolver interface {
	ResolveDefaultModel(ctx context.Context) (domaintextmodel.AccountRecord, domaintextmodel.Model, string, error)
}

type RunStore interface {
	SaveOptimizationRun(ctx context.Context, run domaintextmodel.OptimizationRun) (domaintextmodel.OptimizationRun, error)
}

type OptimizerFactory func(account domaintextmodel.AccountRecord, apiKey string) (textprovider.Optimizer, error)

type Service struct {
	models   DefaultModelResolver
	runs     RunStore
	quoteKey []byte
	factory  OptimizerFactory
	now      func() time.Time
}

type EstimateRequest struct {
	UserID int64  `json:"-"`
	Prompt string `json:"prompt"`
}

type ModelSummary struct {
	ID          int64  `json:"id"`
	ModelCode   string `json:"model_code"`
	DisplayName string `json:"display_name"`
	APIStyle    string `json:"api_style"`
}

type EstimateResult struct {
	Quote           string       `json:"quote"`
	ExpiresAt       time.Time    `json:"expires_at"`
	EstimatedPoints string       `json:"estimated_points"`
	Model           ModelSummary `json:"model"`
}

type OptimizeRequest struct {
	UserID int64  `json:"-"`
	Prompt string `json:"prompt"`
	Quote  string `json:"quote"`
}

type OptimizeResult struct {
	RunID           string `json:"run_id"`
	OptimizedPrompt string `json:"optimized_prompt"`
	InputTokens     int    `json:"input_tokens"`
	OutputTokens    int    `json:"output_tokens"`
	EstimatedPoints string `json:"estimated_points"`
	ActualPoints    string `json:"actual_points"`
}

type quotePayload struct {
	UserID         int64  `json:"u"`
	PromptSHA256   string `json:"p"`
	AccountID      int64  `json:"a"`
	AccountVersion int64  `json:"av"`
	ModelID        int64  `json:"m"`
	ModelVersion   int64  `json:"mv"`
	ExpiresUnix    int64  `json:"exp"`
}

func NewService(models DefaultModelResolver, runs RunStore, quoteKey string, factory OptimizerFactory) *Service {
	if factory == nil {
		factory = func(account domaintextmodel.AccountRecord, apiKey string) (textprovider.Optimizer, error) {
			return textopenai.NewClient(textopenai.Config{BaseURL: account.BaseURL, APIKey: apiKey, APIStyle: account.APIStyle})
		}
	}
	return &Service{models: models, runs: runs, quoteKey: []byte(strings.TrimSpace(quoteKey)), factory: factory, now: time.Now}
}

func (s *Service) Estimate(ctx context.Context, req EstimateRequest) (EstimateResult, error) {
	prompt, err := validatePrompt(req.UserID, req.Prompt)
	if err != nil {
		return EstimateResult{}, err
	}
	account, model, _, err := s.models.ResolveDefaultModel(ctx)
	if err != nil {
		return EstimateResult{}, err
	}
	expiresAt := s.now().UTC().Add(quoteTTL)
	payload := quotePayload{
		UserID: req.UserID, PromptSHA256: promptDigest(prompt), AccountID: account.ID,
		AccountVersion: account.Version, ModelID: model.ID, ModelVersion: model.Version, ExpiresUnix: expiresAt.Unix(),
	}
	quote, err := s.signQuote(payload)
	if err != nil {
		return EstimateResult{}, err
	}
	return EstimateResult{
		Quote: quote, ExpiresAt: expiresAt, EstimatedPoints: zeroPoints,
		Model: ModelSummary{ID: model.ID, ModelCode: model.ModelCode, DisplayName: model.DisplayName, APIStyle: account.APIStyle},
	}, nil
}

func (s *Service) Optimize(ctx context.Context, req OptimizeRequest) (OptimizeResult, error) {
	prompt, err := validatePrompt(req.UserID, req.Prompt)
	if err != nil {
		return OptimizeResult{}, err
	}
	quote, err := s.verifyQuote(req.Quote)
	if err != nil || quote.UserID != req.UserID || quote.PromptSHA256 != promptDigest(prompt) || s.now().UTC().Unix() > quote.ExpiresUnix {
		return OptimizeResult{}, errs.New(409, errs.CodeConflict, "prompt optimization estimate is stale; request a new estimate")
	}
	account, model, apiKey, err := s.models.ResolveDefaultModel(ctx)
	if err != nil {
		return OptimizeResult{}, err
	}
	if account.ID != quote.AccountID || account.Version != quote.AccountVersion || model.ID != quote.ModelID || model.Version != quote.ModelVersion {
		return OptimizeResult{}, errs.New(409, errs.CodeConflict, "prompt optimization estimate is stale; request a new estimate")
	}
	client, err := s.factory(account, apiKey)
	if err != nil {
		return OptimizeResult{}, errs.New(409, errs.CodeConflict, "default text model is not available")
	}
	run := domaintextmodel.OptimizationRun{
		ID: uuid.NewString(), UserID: req.UserID, AccountID: account.ID, ModelID: model.ID,
		ModelCode: model.ModelCode, APIStyle: account.APIStyle, ConfigVersion: model.Version,
		PromptSHA256: promptDigest(prompt), Status: "running", EstimatedPoints: zeroPoints, ActualPoints: zeroPoints,
	}
	if _, err := s.runs.SaveOptimizationRun(ctx, run); err != nil {
		return OptimizeResult{}, fmt.Errorf("persist prompt optimization run: %w", err)
	}
	protected, err := protectPromptTemplate(prompt)
	if err != nil {
		run.Status = "failed"
		run.ErrorCode = prompttemplate.CodeInvalid
		run.ErrorMessage = "prompt template is invalid"
		_, _ = s.runs.SaveOptimizationRun(ctx, run)
		return OptimizeResult{}, promptTemplateValidationError(err)
	}
	response, optimizeErr := client.Optimize(ctx, textprovider.OptimizeRequest{
		Model:        model.ModelCode,
		SystemPrompt: "Rewrite the user's image-generation prompt with precise subject, composition, lighting, style, and constraints. Preserve intent. Strings shaped like MGS_TOKEN markers are protected placeholders: you must not modify, translate, duplicate, remove, split, or add them. Return only the rewritten prompt.",
		Prompt:       protected.Text, MaxOutputTokens: 2000,
	})
	if optimizeErr != nil {
		run.Status = "failed"
		run.ErrorCode = providerErrorCode(optimizeErr)
		run.ErrorMessage = "text model request failed"
		_, _ = s.runs.SaveOptimizationRun(ctx, run)
		return OptimizeResult{}, errs.New(502, "PROMPT_OPTIMIZATION_FAILED", "prompt optimization failed; the original prompt was not changed")
	}
	optimized, restoreErr := protected.Restore(strings.TrimSpace(response.Text))
	if restoreErr != nil || optimized == "" || len([]rune(optimized)) > prompttemplate.DefaultLimits().MaxExpandedRunes {
		run.Status = "failed"
		run.ErrorCode = "INVALID_OPTIMIZATION_RESULT"
		run.ErrorMessage = "text model returned an invalid optimization result"
		_, _ = s.runs.SaveOptimizationRun(ctx, run)
		return OptimizeResult{}, errs.New(502, "INVALID_OPTIMIZATION_RESULT", "prompt optimization returned an invalid result")
	}
	run.Status = "succeeded"
	run.InputTokens = response.InputTokens
	run.OutputTokens = response.OutputTokens
	run.ProviderRequestID = response.RequestID
	if _, err := s.runs.SaveOptimizationRun(ctx, run); err != nil {
		return OptimizeResult{}, fmt.Errorf("persist prompt optimization result: %w", err)
	}
	return OptimizeResult{
		RunID: run.ID, OptimizedPrompt: optimized, InputTokens: response.InputTokens, OutputTokens: response.OutputTokens,
		EstimatedPoints: zeroPoints, ActualPoints: zeroPoints,
	}, nil
}

type protectedPrompt struct {
	Text         string
	replacements map[string]string
	expected     map[string]int
}

func protectPromptTemplate(prompt string) (protectedPrompt, error) {
	document, err := prompttemplate.Parse(prompt, prompttemplate.DefaultLimits())
	if err != nil {
		return protectedPrompt{}, err
	}
	nonce := strings.ReplaceAll(uuid.NewString(), "-", "")
	result := protectedPrompt{
		replacements: make(map[string]string, len(document.Occurrences)),
		expected:     make(map[string]int, len(document.Occurrences)),
	}
	builder := strings.Builder{}
	tokenIndex := 0
	for _, segment := range document.Segments {
		if segment.Kind == prompttemplate.KindText {
			builder.WriteString(segment.Source)
			continue
		}
		tokenIndex++
		sentinel := fmt.Sprintf("⟦MGS_TOKEN_%s_%04d⟧", nonce, tokenIndex)
		placeholder := "{{@" + segment.Name + "}}"
		if segment.Kind == prompttemplate.KindVariable {
			placeholder = "{{$" + segment.Name + "}}"
		}
		result.replacements[sentinel] = placeholder
		result.expected[promptTokenKey(segment.Kind, segment.Name)]++
		builder.WriteString(sentinel)
	}
	result.Text = builder.String()
	return result, nil
}

func (p protectedPrompt) Restore(raw string) (string, error) {
	restored := raw
	for sentinel, placeholder := range p.replacements {
		if strings.Count(restored, sentinel) != 1 {
			return "", errors.New("protected prompt sentinel mismatch")
		}
		restored = strings.Replace(restored, sentinel, placeholder, 1)
	}
	if strings.Contains(restored, "⟦MGS_TOKEN_") {
		return "", errors.New("unknown protected prompt sentinel")
	}
	document, err := prompttemplate.Parse(restored, prompttemplate.DefaultLimits())
	if err != nil {
		return "", err
	}
	actual := make(map[string]int, len(document.Occurrences))
	for _, occurrence := range document.Occurrences {
		actual[promptTokenKey(occurrence.Kind, occurrence.Name)]++
	}
	if len(actual) != len(p.expected) {
		return "", errors.New("protected prompt placeholder set mismatch")
	}
	for key, expected := range p.expected {
		if actual[key] != expected {
			return "", errors.New("protected prompt placeholder set mismatch")
		}
	}
	return document.Canonical, nil
}

func promptTokenKey(kind prompttemplate.Kind, name string) string {
	return string(kind) + "\x00" + name
}

func promptTemplateValidationError(err error) error {
	var templateErr *prompttemplate.Error
	if !errors.As(err, &templateErr) {
		return errs.BadRequest("prompt template is invalid")
	}
	details := map[string]any{"field": templateErr.Field, "rule": templateErr.Rule, "offset": templateErr.Offset}
	if templateErr.Name != "" {
		details["name"] = templateErr.Name
	}
	return errs.WithDetails(errs.New(400, templateErr.Code, templateErr.Message), details)
}

func validatePrompt(userID int64, raw string) (string, error) {
	prompt := strings.TrimSpace(raw)
	length := len([]rune(prompt))
	if userID <= 0 || length < 8 || length > 4000 {
		return "", errs.BadRequest("prompt must contain between 8 and 4000 characters")
	}
	return prompt, nil
}

func promptDigest(prompt string) string {
	sum := sha256.Sum256([]byte(prompt))
	return hex.EncodeToString(sum[:])
}

func (s *Service) signQuote(payload quotePayload) (string, error) {
	if len(s.quoteKey) == 0 {
		return "", errs.Internal("prompt optimization quote signing key is not configured")
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal prompt optimization quote: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	signature := hmac.New(sha256.New, s.quoteKey)
	_, _ = signature.Write([]byte(encoded))
	return encoded + "." + base64.RawURLEncoding.EncodeToString(signature.Sum(nil)), nil
}

func (s *Service) verifyQuote(raw string) (quotePayload, error) {
	parts := strings.Split(raw, ".")
	if len(parts) != 2 || len(s.quoteKey) == 0 {
		return quotePayload{}, errors.New("invalid quote")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return quotePayload{}, errors.New("invalid quote")
	}
	expected := hmac.New(sha256.New, s.quoteKey)
	_, _ = expected.Write([]byte(parts[0]))
	if !hmac.Equal(signature, expected.Sum(nil)) {
		return quotePayload{}, errors.New("invalid quote")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return quotePayload{}, errors.New("invalid quote")
	}
	var quote quotePayload
	if err := json.Unmarshal(payload, &quote); err != nil {
		return quotePayload{}, errors.New("invalid quote")
	}
	return quote, nil
}

func providerErrorCode(err error) string {
	var providerErr *textprovider.Error
	if errors.As(err, &providerErr) && providerErr.Code != "" {
		return providerErr.Code
	}
	return "TEXT_PROVIDER_ERROR"
}
