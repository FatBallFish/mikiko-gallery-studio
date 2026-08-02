package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	textprovider "github.com/fatballfish/pic-gallery/internal/provider/text"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	promptoptimizer "github.com/fatballfish/pic-gallery/internal/service/promptoptimizer"
	textmodelservice "github.com/fatballfish/pic-gallery/internal/service/textmodel"
)

type routerTextOptimizer struct{}

func (routerTextOptimizer) Optimize(context.Context, textprovider.OptimizeRequest) (textprovider.OptimizeResponse, error) {
	return textprovider.OptimizeResponse{Text: "A polished cinematic image prompt", InputTokens: 10, OutputTokens: 6, RequestID: "router-text-request"}, nil
}

func TestTextModelAdminAndPromptOptimizationAPIs(t *testing.T) {
	cfg := adminConfigAPIConfig()
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour,
		Issuer: "test", AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("prompt-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, userSession, err := authSvc.LoginWithEmailCode("prompt-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email: "root-text@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role: domainadminauth.RoleSuperAdmin, Status: "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	textStore := textmodelservice.NewMemoryStore()
	textSvc := textmodelservice.NewServiceWithOptimizerFactory(textStore, "text-encryption-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) {
		return routerTextOptimizer{}, nil
	})
	promptSvc := promptoptimizer.NewService(textSvc, textStore, "quote-signing-key", func(domaintextmodel.AccountRecord, string) (textprovider.Optimizer, error) {
		return routerTextOptimizer{}, nil
	})
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, nil, nil, adminAuth, nil)
	api.SetTextModelServices(textSvc, promptSvc)
	handler := NewWithAPI(api)
	adminToken := loginAdminWithCredentials(t, handler, "root-text@example.com", "password")

	accountBody := `{"name":"Primary","platform_type":"openai_compatible","api_style":"responses","base_url":"https://text.example.com","enabled":true,"secrets":{"api_key":"admin-secret-value"}}`
	accountReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/text-model-accounts", bytes.NewBufferString(accountBody))
	accountReq.Header.Set("Authorization", "Bearer "+adminToken)
	accountReq.Header.Set("Content-Type", "application/json")
	accountRec := httptest.NewRecorder()
	handler.ServeHTTP(accountRec, accountReq)
	if accountRec.Code != http.StatusCreated {
		t.Fatalf("expected account create 201, got %d body=%s", accountRec.Code, accountRec.Body.String())
	}
	if bytes.Contains(accountRec.Body.Bytes(), []byte("admin-secret-value")) {
		t.Fatalf("account response leaked secret: %s", accountRec.Body.String())
	}
	var accountResp struct {
		Data domaintextmodel.Account `json:"data"`
	}
	if err := json.NewDecoder(accountRec.Body).Decode(&accountResp); err != nil {
		t.Fatalf("decode account: %v", err)
	}

	modelBody := `{"model_code":"gpt-test","display_name":"GPT Test","input_price_per_million_tokens":"1.250000","output_price_per_million_tokens":"10.000000","currency":"USD","enabled":true}`
	modelReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/text-model-accounts/"+jsonNumber(accountResp.Data.ID)+"/models", bytes.NewBufferString(modelBody))
	modelReq.Header.Set("Authorization", "Bearer "+adminToken)
	modelReq.Header.Set("Content-Type", "application/json")
	modelRec := httptest.NewRecorder()
	handler.ServeHTTP(modelRec, modelReq)
	if modelRec.Code != http.StatusCreated {
		t.Fatalf("expected model create 201, got %d body=%s", modelRec.Code, modelRec.Body.String())
	}
	var modelResp struct {
		Data domaintextmodel.Model `json:"data"`
	}
	if err := json.NewDecoder(modelRec.Body).Decode(&modelResp); err != nil {
		t.Fatalf("decode model: %v", err)
	}
	testReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/text-models/"+jsonNumber(modelResp.Data.ID)+":test", nil)
	testReq.Header.Set("Authorization", "Bearer "+adminToken)
	testRec := httptest.NewRecorder()
	handler.ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK || !bytes.Contains(testRec.Body.Bytes(), []byte(`"status":"success"`)) || bytes.Contains(testRec.Body.Bytes(), []byte("admin-secret-value")) {
		t.Fatalf("unexpected connection test response %d body=%s", testRec.Code, testRec.Body.String())
	}
	if !modelResp.Data.IsDefault {
		t.Fatalf("first enabled model should be returned as default: %#v", modelResp.Data)
	}

	estimateReq := httptest.NewRequest(http.MethodPost, "/api/agent/text/v1/prompt-optimizations/estimate", bytes.NewBufferString(`{"prompt":"a portrait in summer rain"}`))
	estimateReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	estimateReq.Header.Set("Content-Type", "application/json")
	estimateRec := httptest.NewRecorder()
	handler.ServeHTTP(estimateRec, estimateReq)
	if estimateRec.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d body=%s", estimateRec.Code, estimateRec.Body.String())
	}
	var estimateResp struct {
		Data promptoptimizer.EstimateResult `json:"data"`
	}
	if err := json.NewDecoder(estimateRec.Body).Decode(&estimateResp); err != nil {
		t.Fatalf("decode estimate: %v", err)
	}
	if estimateResp.Data.EstimatedPoints != "0.00000" || estimateResp.Data.Quote == "" {
		t.Fatalf("unexpected estimate %#v", estimateResp.Data)
	}

	optimizeBody, _ := json.Marshal(map[string]string{"prompt": "a portrait in summer rain", "quote": estimateResp.Data.Quote})
	optimizeReq := httptest.NewRequest(http.MethodPost, "/api/agent/text/v1/prompt-optimizations", bytes.NewReader(optimizeBody))
	optimizeReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	optimizeReq.Header.Set("Content-Type", "application/json")
	optimizeRec := httptest.NewRecorder()
	handler.ServeHTTP(optimizeRec, optimizeReq)
	if optimizeRec.Code != http.StatusOK || !bytes.Contains(optimizeRec.Body.Bytes(), []byte("A polished cinematic image prompt")) {
		t.Fatalf("unexpected optimize response %d body=%s", optimizeRec.Code, optimizeRec.Body.String())
	}

	legacyDefault, err := textStore.GetModel(t.Context(), modelResp.Data.ID)
	if err != nil {
		t.Fatalf("GetModel default: %v", err)
	}
	legacyDefault.IsDefault = false
	legacyDefault.Version++
	if _, err := textStore.UpdateModel(t.Context(), legacyDefault); err != nil {
		t.Fatalf("clear legacy default: %v", err)
	}
	if _, err := textStore.CreateModel(t.Context(), domaintextmodel.Model{
		AccountID: accountResp.Data.ID, ModelCode: "gpt-second", DisplayName: "GPT Second",
		InputPricePerMTok: "0.000000", OutputPricePerMTok: "0.000000", Currency: "USD", Enabled: true, Version: 1,
	}); err != nil {
		t.Fatalf("seed second legacy candidate: %v", err)
	}
	ambiguousReq := httptest.NewRequest(http.MethodPost, "/api/agent/text/v1/prompt-optimizations/estimate", bytes.NewBufferString(`{"prompt":"ambiguous configuration"}`))
	ambiguousReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	ambiguousReq.Header.Set("Content-Type", "application/json")
	ambiguousRec := httptest.NewRecorder()
	handler.ServeHTTP(ambiguousRec, ambiguousReq)
	if ambiguousRec.Code != http.StatusConflict ||
		!bytes.Contains(ambiguousRec.Body.Bytes(), []byte(`"code":"TEXT_MODEL_DEFAULT_REQUIRED"`)) ||
		!bytes.Contains(ambiguousRec.Body.Bytes(), []byte(`"next_suggestion":"select a default text model and retry"`)) {
		t.Fatalf("expected actionable default-required response, got %d body=%s", ambiguousRec.Code, ambiguousRec.Body.String())
	}
}
