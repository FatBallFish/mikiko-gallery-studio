package compat_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	apphttp "github.com/fatballfish/pic-gallery/internal/http/router"
	apikeyservice "github.com/fatballfish/pic-gallery/internal/service/apikey"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
)

func TestOpenAICompatGenerateRoutesToOpenRouter(t *testing.T) {
	openrouterServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["model"] != "openrouter/vision" {
			t.Fatalf("unexpected upstream model %#v", body["model"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"images":[{"image_url":{"url":"https://cdn.example.com/or.png"}}]}}]}`)
	}))
	defer openrouterServer.Close()

	cfg := compatTestConfig()
	cfg.Providers.OpenRouter.BaseURL = openrouterServer.URL
	cfg.Providers.OpenAI.BaseURL = "http://127.0.0.1:1"
	cfg.Routing.DefaultProvider = "openrouter"
	cfg.Routing.FallbackProviders = []string{"openai"}
	cfg.Routing.OpenAICompatModelMap = map[string]string{"gpt-image-2": "plus"}
	cfg.Routing.ProviderModelMap = map[string]map[string]string{
		"plus": {
			"openrouter": "openrouter/vision",
			"openai":     "gpt-image-1",
		},
	}

	handler, apiSecret := newCompatHandler(t, cfg, "compat-generate@example.com", "100.00000")

	body := bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"Generate a poster","size":"1536x1024","n":2,"quality":"high","response_format":"url"}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", body)
	req.Header.Set("Authorization", "Bearer "+apiSecret)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Created int64 `json:"created"`
		Data    []struct {
			URL string `json:"url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].URL != "https://cdn.example.com/or.png" {
		t.Fatalf("unexpected response %#v", resp)
	}
}

func TestOpenAICompatEditWithMaskRoutesToOpenAI(t *testing.T) {
	openaiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/images/edits" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseMultipartForm(4 << 20); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("model"); got != "gpt-image-1" {
			t.Fatalf("unexpected upstream model %q", got)
		}
		if got := r.FormValue("quality"); got != "4k" {
			t.Fatalf("unexpected normalized quality %q", got)
		}
		if len(r.MultipartForm.File["image"]) != 1 {
			t.Fatalf("expected one image file")
		}
		if len(r.MultipartForm.File["mask"]) != 1 {
			t.Fatalf("expected one mask file")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"created":1770000001,"data":[{"b64_json":"ZmFrZS1lZGl0"}]}`)
	}))
	defer openaiServer.Close()

	cfg := compatTestConfig()
	cfg.Providers.OpenAI.BaseURL = openaiServer.URL
	cfg.Providers.OpenRouter.BaseURL = "http://127.0.0.1:1"
	cfg.Routing.DefaultProvider = "openrouter"
	cfg.Routing.FallbackProviders = []string{"openai"}
	cfg.Routing.OpenAICompatModelMap = map[string]string{"gpt-image-2": "plus"}
	cfg.Routing.ProviderModelMap = map[string]map[string]string{
		"plus": {
			"openrouter": "openrouter/vision",
			"openai":     "gpt-image-1",
		},
	}

	handler, apiSecret := newCompatHandler(t, cfg, "compat-edit@example.com", "100.00000")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	_ = writer.WriteField("model", "gpt-image-2")
	_ = writer.WriteField("prompt", "Replace the sky")
	_ = writer.WriteField("size", "auto")
	_ = writer.WriteField("n", "1")
	_ = writer.WriteField("quality", "high")
	_ = writer.WriteField("response_format", "b64_json")
	imgPart, _ := writer.CreateFormFile("image", "input.png")
	_, _ = imgPart.Write([]byte("image"))
	maskPart, _ := writer.CreateFormFile("mask", "mask.png")
	_, _ = maskPart.Write([]byte("mask"))
	_ = writer.Close()

	req := httptest.NewRequest(http.MethodPost, "/v1/images/edits", body)
	req.Header.Set("Authorization", "Bearer "+apiSecret)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []struct {
			B64JSON string `json:"b64_json"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].B64JSON != "ZmFrZS1lZGl0" {
		t.Fatalf("unexpected response %#v", resp)
	}
}

func TestOpenAICompatModelsListsConfiguredCompatModels(t *testing.T) {
	cfg := compatTestConfig()
	cfg.Routing.OpenAICompatModelMap = map[string]string{
		"gpt-image-2":     "plus",
		"gpt-image-basic": "basic",
	}
	handler, apiSecret := newCompatHandler(t, cfg, "compat-models@example.com", "100.00000")

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	req.Header.Set("Authorization", "Bearer "+apiSecret)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Object != "list" || len(resp.Data) != 2 {
		t.Fatalf("unexpected models response %#v", resp)
	}
}

func TestOpenAICompatGenerateMapsUpstreamFailure(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"message":"provider down","code":"server_error","type":"server_error"}}`)
	}))
	defer upstream.Close()

	cfg := compatTestConfig()
	cfg.Providers.OpenRouter.BaseURL = upstream.URL
	cfg.Providers.OpenAI.Enabled = false
	cfg.Routing.DefaultProvider = "openrouter"
	cfg.Routing.OpenAICompatModelMap = map[string]string{"gpt-image-2": "plus"}
	cfg.Routing.ProviderModelMap = map[string]map[string]string{"plus": {"openrouter": "openrouter/vision"}}
	handler, apiSecret := newCompatHandler(t, cfg, "compat-upstream@example.com", "100.00000")

	req := httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewBufferString(`{"model":"gpt-image-2","prompt":"fail please","n":1,"quality":"high"}`))
	req.Header.Set("Authorization", "Bearer "+apiSecret)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected upstream failure 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if resp.Error.Code != "UPSTREAM_UNAVAILABLE" {
		t.Fatalf("expected UPSTREAM_UNAVAILABLE, got %#v", resp)
	}
}

func compatTestConfig() config.Config {
	cfg := config.Config{}
	cfg.Billing.AutoQualityDefaultByGroup = map[string]string{"basic": "1k", "plus": "2k", "pro": "4k"}
	cfg.Billing.QualityPointsByModel = map[string]map[string]string{
		"basic": {"1k": "2.00000", "2k": "4.00000", "4k": "8.00000"},
		"plus":  {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
	}
	cfg.Billing.UserGroupMultipliers = map[string]string{"basic": "1.00000", "plus": "1.00000"}
	cfg.Billing.TaskMultipliers = map[string]string{"text_to_image": "1.00000", "image_edit": "1.25000", "reference_generate": "1.15000"}
	cfg.GenerationLimits.MaxImageCount = 5
	cfg.GenerationLimits.ReferenceImageMaxCount = 4
	cfg.Providers.OpenAI.Enabled = true
	cfg.Providers.OpenAI.APIKey = "test-openai"
	cfg.Providers.OpenRouter.Enabled = true
	cfg.Providers.OpenRouter.APIKey = "test-openrouter"
	cfg.Routing.ProviderCapabilities = map[string]config.ProviderCapabilityConfig{
		"openrouter": {
			SupportedModels:        []string{"plus"},
			SupportedTaskTypes:     []string{"text_to_image", "image_edit", "reference_generate"},
			SupportedQualities:     []string{"1k", "2k", "4k"},
			SupportedAspectRatios:  []string{"1:1", "4:3", "16:9"},
			MaxImageCount:          5,
			MaxReferenceImageCount: 4,
			SupportsImageInput:     true,
			SupportsMask:           false,
			Priority:               1,
		},
		"openai": {
			SupportedModels:        []string{"plus"},
			SupportedTaskTypes:     []string{"text_to_image", "image_edit", "reference_generate"},
			SupportedQualities:     []string{"1k", "2k", "4k"},
			SupportedAspectRatios:  []string{"1:1", "4:3", "16:9"},
			MaxImageCount:          5,
			MaxReferenceImageCount: 4,
			SupportsImageInput:     true,
			SupportsMask:           true,
			Priority:               2,
		},
	}
	return cfg
}

func newCompatHandler(t *testing.T, cfg config.Config, email, balance string) (http.Handler, string) {
	t.Helper()

	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode(email, "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, _, err := authSvc.LoginWithEmailCode(email, "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       user.ID,
		ChangePoints: balance,
		Reason:       "seed balance",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}

	keySvc := apikeyservice.NewService(nil)
	created, err := keySvc.CreateKey(context.Background(), apikeyservice.CreateRequest{
		UserID:    user.ID,
		Name:      "compat",
		GroupCode: "plus",
		Secret:    "sk-compat-secret-" + email,
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc, keySvc)
	return apphttp.NewWithAPI(api), created.Secret
}

func TestOpenAICompatRejectsMissingAndInvalidBearerKey(t *testing.T) {
	cfg := compatTestConfig()
	handler, _ := newCompatHandler(t, cfg, "compat-auth@example.com", "100.00000")

	missingReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	missingRec := httptest.NewRecorder()
	handler.ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected missing bearer 401, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	invalidReq.Header.Set("Authorization", "Bearer sk-invalid")
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected invalid bearer 401, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}
