package router

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/worker"
)

func TestAgentTaskCreateAndQueryEndpoints(t *testing.T) {
	providerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"images":[{"image_url":{"url":"https://cdn.example.com/task.png"}}]}}]}`)
	}))
	defer providerServer.Close()

	cfg := taskAPIConfig(providerServer.URL)
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("task@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := authSvc.LoginWithEmailCode("task@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	createBody := bytes.NewBufferString(`{"task_type":"text_to_image","prompt":"Generate a banner","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"reference_image_count":0,"response_mode":"sync"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", createBody)
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Results []struct {
				URL string `json:"url"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.ID == "" || createResp.Data.Status != "queued" {
		t.Fatalf("unexpected create response %#v", createResp)
	}

	runner := worker.NewRunner(taskSvc, worker.Config{
		Owner:              "router-test-worker",
		LeaseTTL:           30 * time.Second,
		PreferredProviders: []string{"openrouter"},
	})
	processed, err := runner.ProcessOnce(createReq.Context())
	if err != nil {
		t.Fatalf("ProcessOnce: %v", err)
	}
	if !processed {
		t.Fatal("expected queued task to be processed by worker")
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks", nil)
	listReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)

	if listRec.Code != http.StatusOK {
		t.Fatalf("expected list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode list response: %v", err)
	}
	if len(listResp.Data) != 1 || listResp.Data[0].ID != createResp.Data.ID {
		t.Fatalf("unexpected list response %#v", listResp)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/"+createResp.Data.ID, nil)
	detailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)

	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data struct {
			ID      string `json:"id"`
			Status  string `json:"status"`
			Results []struct {
				URL string `json:"url"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode detail response: %v", err)
	}
	if detailResp.Data.ID != createResp.Data.ID || detailResp.Data.Status != "succeeded" || len(detailResp.Data.Results) != 1 || detailResp.Data.Results[0].URL == "" {
		t.Fatalf("unexpected detail response %#v", detailResp)
	}

	historyListReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/history/tasks", nil)
	historyListReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	historyListRec := httptest.NewRecorder()
	handler.ServeHTTP(historyListRec, historyListReq)

	if historyListRec.Code != http.StatusOK {
		t.Fatalf("expected history list 200, got %d body=%s", historyListRec.Code, historyListRec.Body.String())
	}

	historyDetailReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/history/tasks/"+createResp.Data.ID, nil)
	historyDetailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	historyDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(historyDetailRec, historyDetailReq)

	if historyDetailRec.Code != http.StatusOK {
		t.Fatalf("expected history detail 200, got %d body=%s", historyDetailRec.Code, historyDetailRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/agent/image/v1/history/tasks/"+createResp.Data.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)

	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected delete 204, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	historyListRec = httptest.NewRecorder()
	handler.ServeHTTP(historyListRec, historyListReq)
	if historyListRec.Code != http.StatusOK {
		t.Fatalf("expected history list after delete 200, got %d body=%s", historyListRec.Code, historyListRec.Body.String())
	}
	var historyListResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(historyListRec.Body).Decode(&historyListResp); err != nil {
		t.Fatalf("decode history list response: %v", err)
	}
	if len(historyListResp.Data) != 0 {
		t.Fatalf("expected empty history list after delete, got %#v", historyListResp)
	}

	historyDetailRec = httptest.NewRecorder()
	handler.ServeHTTP(historyDetailRec, historyDetailReq)
	if historyDetailRec.Code != http.StatusNotFound {
		t.Fatalf("expected history detail 404 after delete, got %d body=%s", historyDetailRec.Code, historyDetailRec.Body.String())
	}
}

func taskAPIConfig(openrouterBaseURL string) config.Config {
	cfg := config.Config{}
	cfg.Billing.AutoQualityDefaultByGroup = map[string]string{"plus": "2k"}
	cfg.Billing.QualityPointsByModel = map[string]map[string]string{
		"plus": {"1k": "5.00000", "2k": "8.00000", "4k": "16.00000"},
	}
	cfg.Billing.UserGroupMultipliers = map[string]string{"basic": "1.00000", "plus": "1.00000"}
	cfg.Billing.TaskMultipliers = map[string]string{"text_to_image": "1.00000", "image_edit": "1.25000", "reference_generate": "1.15000"}
	cfg.GenerationLimits.MaxImageCount = 5
	cfg.GenerationLimits.ReferenceImageMaxCount = 4
	cfg.Providers.OpenRouter.Enabled = true
	cfg.Providers.OpenRouter.BaseURL = openrouterBaseURL
	cfg.Providers.OpenRouter.APIKey = "or-key"
	cfg.Providers.OpenAI.Enabled = true
	cfg.Providers.OpenAI.BaseURL = "http://127.0.0.1:1"
	cfg.Providers.OpenAI.APIKey = "oa-key"
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
	cfg.Routing.ProviderModelMap = map[string]map[string]string{
		"plus": {"openrouter": "openrouter/vision", "openai": "gpt-image-1"},
	}
	cfg.App.Name = "pic-gallery"
	return cfg
}

func TestRedeemCodeRejectsUnknownCodeWithoutCreditingBalance(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("redeem@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, session, err := authSvc.LoginWithEmailCode("redeem@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	redeemReq := httptest.NewRequest(http.MethodPost, "/api/agent/billing/v1/redeem-codes/redeem", bytes.NewBufferString(`{"code":"ANYTHING"}`))
	redeemReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	redeemReq.Header.Set("Content-Type", "application/json")
	redeemReq.Header.Set("Idempotency-Key", "redeem-unknown")
	redeemRec := httptest.NewRecorder()
	handler.ServeHTTP(redeemRec, redeemReq)
	if redeemRec.Code != http.StatusNotFound {
		t.Fatalf("expected unknown redeem code 404, got %d body=%s", redeemRec.Code, redeemRec.Body.String())
	}

	balance, err := billingSvc.GetBalance(context.Background(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "0.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("expected redeem failure to leave balance unchanged, got %#v", balance)
	}
}

func TestAgentTaskCreateQueuesTaskForWorker(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("queue@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := authSvc.LoginWithEmailCode("queue@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	createBody := bytes.NewBufferString(`{"task_type":"text_to_image","prompt":"Queue a banner","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", createBody)
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)

	if createRec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.ID == "" || createResp.Data.Status != "queued" {
		t.Fatalf("expected queued create response, got %#v", createResp)
	}
}

func TestAgentBillingBalanceAndLedgerEndpoints(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("billing@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := authSvc.LoginWithEmailCode("billing@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "50.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	if _, err := billingSvc.ReserveTask(context.Background(), domainbilling.ReserveRequest{UserID: 1, TaskID: "44444444-4444-4444-4444-444444444444", EstimatedPoints: "8.00000", Reason: "reserve"}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	if _, err := billingSvc.FinalizeTask(context.Background(), domainbilling.FinalizeRequest{UserID: 1, TaskID: "44444444-4444-4444-4444-444444444444", EstimatedPoints: "8.00000", ActualPoints: "5.00000", Reason: "finalize"}); err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}

	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("expected balance 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp struct {
		Data struct {
			AvailablePoints string `json:"available_points"`
			FrozenPoints    string `json:"frozen_points"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance response: %v", err)
	}
	if balanceResp.Data.AvailablePoints != "45.00000" || balanceResp.Data.FrozenPoints != "0.00000" {
		t.Fatalf("unexpected balance response %#v", balanceResp)
	}
	if balanceResp.Meta.RequestID == "" || balanceRec.Header().Get("X-Request-Id") == "" {
		t.Fatalf("expected balance response to include request id metadata, got body=%#v headers=%v", balanceResp, balanceRec.Header())
	}

	ledgerReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/ledger?page=1&page_size=10", nil)
	ledgerReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	ledgerRec := httptest.NewRecorder()
	handler.ServeHTTP(ledgerRec, ledgerReq)
	if ledgerRec.Code != http.StatusOK {
		t.Fatalf("expected ledger 200, got %d body=%s", ledgerRec.Code, ledgerRec.Body.String())
	}
	var ledgerResp struct {
		Data struct {
			Items []struct {
				ID           int64  `json:"id"`
				TaskID       string `json:"task_id"`
				LedgerType   string `json:"ledger_type"`
				ChangePoints string `json:"change_points"`
				BalanceAfter string `json:"balance_after"`
				FrozenAfter  string `json:"frozen_after"`
				Reason       string `json:"reason"`
			} `json:"items"`
			Pagination struct {
				Page     int `json:"page"`
				PageSize int `json:"page_size"`
				Total    int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
		Meta struct {
			RequestID string `json:"request_id"`
		} `json:"meta"`
	}
	if err := json.NewDecoder(ledgerRec.Body).Decode(&ledgerResp); err != nil {
		t.Fatalf("decode ledger response: %v", err)
	}
	if ledgerResp.Data.Pagination.Page != 1 || ledgerResp.Data.Pagination.PageSize != 10 || ledgerResp.Data.Pagination.Total != 4 || len(ledgerResp.Data.Items) != 4 {
		t.Fatalf("unexpected ledger pagination %#v", ledgerResp)
	}
	if ledgerResp.Data.Items[0].LedgerType != "refund" {
		t.Fatalf("unexpected ledger order %#v", ledgerResp.Data.Items)
	}
	if ledgerResp.Data.Items[0].ID == 0 || ledgerResp.Data.Items[0].TaskID == "" || ledgerResp.Data.Items[0].Reason == "" || ledgerResp.Data.Items[0].FrozenAfter == "" || ledgerResp.Data.Items[0].BalanceAfter == "" {
		t.Fatalf("expected ledger entry contract fields to be populated, got %#v", ledgerResp.Data.Items[0])
	}
	if ledgerResp.Meta.RequestID == "" || ledgerRec.Header().Get("X-Request-Id") == "" {
		t.Fatalf("expected ledger response to include request id metadata, got body=%#v headers=%v", ledgerResp, ledgerRec.Header())
	}

	invalidLedgerReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/ledger?page=oops&page_size=-5", nil)
	invalidLedgerReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	invalidLedgerRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidLedgerRec, invalidLedgerReq)
	if invalidLedgerRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid ledger pagination to return 400, got %d body=%s", invalidLedgerRec.Code, invalidLedgerRec.Body.String())
	}

	unauthorizedBalanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	unauthorizedBalanceRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthorizedBalanceRec, unauthorizedBalanceReq)
	if unauthorizedBalanceRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected unauthorized balance request to return 401, got %d body=%s", unauthorizedBalanceRec.Code, unauthorizedBalanceRec.Body.String())
	}

	methodNotAllowedReq := httptest.NewRequest(http.MethodPost, "/api/agent/billing/v1/balance", nil)
	methodNotAllowedReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	methodNotAllowedRec := httptest.NewRecorder()
	handler.ServeHTTP(methodNotAllowedRec, methodNotAllowedReq)
	if methodNotAllowedRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected non-GET balance request to return 405, got %d body=%s", methodNotAllowedRec.Code, methodNotAllowedRec.Body.String())
	}
	var methodNotAllowedResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(methodNotAllowedRec.Body).Decode(&methodNotAllowedResp); err != nil {
		t.Fatalf("decode balance 405 response: %v", err)
	}
	if methodNotAllowedResp.Error.Code == "" {
		t.Fatalf("expected structured JSON error for 405 response, got %#v", methodNotAllowedResp)
	}

	methodNotAllowedLedgerReq := httptest.NewRequest(http.MethodPost, "/api/agent/billing/v1/ledger", nil)
	methodNotAllowedLedgerReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	methodNotAllowedLedgerRec := httptest.NewRecorder()
	handler.ServeHTTP(methodNotAllowedLedgerRec, methodNotAllowedLedgerReq)
	if methodNotAllowedLedgerRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected non-GET ledger request to return 405, got %d body=%s", methodNotAllowedLedgerRec.Code, methodNotAllowedLedgerRec.Body.String())
	}
}

func TestAgentLedgerEndpointReturnsEmptyArrayForUsersWithoutHistory(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("empty-ledger@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := authSvc.LoginWithEmailCode("empty-ledger@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	ledgerReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/ledger", nil)
	ledgerReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	ledgerRec := httptest.NewRecorder()
	handler.ServeHTTP(ledgerRec, ledgerReq)
	if ledgerRec.Code != http.StatusOK {
		t.Fatalf("expected empty ledger request to return 200, got %d body=%s", ledgerRec.Code, ledgerRec.Body.String())
	}
	var ledgerResp struct {
		Data struct {
			Items []struct{} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(ledgerRec.Body).Decode(&ledgerResp); err != nil {
		t.Fatalf("decode empty ledger response: %v", err)
	}
	if ledgerResp.Data.Items == nil {
		t.Fatalf("expected empty ledger items to marshal as [] not null, got %#v", ledgerResp)
	}
	if len(ledgerResp.Data.Items) != 0 {
		t.Fatalf("expected no ledger items, got %#v", ledgerResp.Data.Items)
	}
}

func TestAgentBillingEndpointsReuseTaskServiceBillingBackend(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("shared-billing@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := authSvc.LoginWithEmailCode("shared-billing@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       1,
		ChangePoints: "20.00000",
		Reason:       "seed balance",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}

	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	if _, err := taskSvc.CreateTask(context.Background(), domainimagetask.CreateRequest{
		UserID:              1,
		UserGroupCode:       "basic",
		UserGroupMultiplier: "1.00000",
		AbstractModel:       "plus",
		TaskType:            "text_to_image",
		Prompt:              "Create a campaign key visual",
		RequestedSize:       "1536x1024",
		RequestedQuality:    "auto",
		OutputImageCount:    1,
		ReferenceImageCount: 0,
		ResponseMode:        "async",
		SavePolicy:          "private",
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	expectedBalance, err := billingSvc.GetBalance(context.Background(), 1, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if expectedBalance.AvailablePoints != "12.00000" || expectedBalance.FrozenPoints != "8.00000" {
		t.Fatalf("unexpected reserved balance %#v", expectedBalance)
	}

	api := handlers.NewAPIWithTaskService(cfg, authSvc, nil, taskSvc)
	handler := NewWithAPI(api)

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("expected balance 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}

	var balanceResp struct {
		Data struct {
			AvailablePoints string `json:"available_points"`
			FrozenPoints    string `json:"frozen_points"`
		} `json:"data"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance response: %v", err)
	}
	if balanceResp.Data.AvailablePoints != expectedBalance.AvailablePoints || balanceResp.Data.FrozenPoints != expectedBalance.FrozenPoints {
		t.Fatalf("expected API balance to reuse taskSvc billing backend, got %#v want %#v", balanceResp.Data, expectedBalance)
	}
}

func TestAgentEstimateEndpointUsesSnakeCaseAndRejectsInvalidCounts(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("estimate@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := authSvc.LoginWithEmailCode("estimate@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	req := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/estimate?task_type=text_to_image&abstract_model=plus&requested_quality=auto&requested_size=1536x1024&requested_output_image_count=2&reference_image_count=1", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected estimate 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode estimate response: %v", err)
	}
	if _, ok := resp.Data["resolved_quality_bucket"]; !ok {
		t.Fatalf("expected snake_case keys, got %#v", resp.Data)
	}
	if _, ok := resp.Data["PricingSnapshot"]; ok {
		t.Fatalf("expected pricing snapshot to stay internal, got %#v", resp.Data)
	}

	invalidReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/estimate?task_type=text_to_image&abstract_model=plus&requested_quality=auto&requested_output_image_count=oops", nil)
	invalidReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	invalidRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidRec, invalidReq)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid estimate request to return 400, got %d body=%s", invalidRec.Code, invalidRec.Body.String())
	}
}

func TestAgentTaskCreateIsIdempotentWithIdempotencyKey(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("idem@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := authSvc.LoginWithEmailCode("idem@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{UserID: 1, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}
	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, taskSvc, nil, billingSvc)
	handler := NewWithAPI(api)

	body := `{"task_type":"text_to_image","prompt":"Queue once","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`
	firstReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", bytes.NewBufferString(body))
	firstReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	firstReq.Header.Set("Content-Type", "application/json")
	firstReq.Header.Set("Idempotency-Key", "same-request")
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusAccepted {
		t.Fatalf("expected first create 202, got %d body=%s", firstRec.Code, firstRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", bytes.NewBufferString(body))
	secondReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	secondReq.Header.Set("Content-Type", "application/json")
	secondReq.Header.Set("Idempotency-Key", "same-request")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusAccepted {
		t.Fatalf("expected second create 202, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}

	var firstResp, secondResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(firstRec.Body).Decode(&firstResp); err != nil {
		t.Fatalf("decode first response: %v", err)
	}
	if err := json.NewDecoder(secondRec.Body).Decode(&secondResp); err != nil {
		t.Fatalf("decode second response: %v", err)
	}
	if firstResp.Data.ID == "" || firstResp.Data.ID != secondResp.Data.ID {
		t.Fatalf("expected same task id for idempotent create, got first=%#v second=%#v", firstResp, secondResp)
	}

	summary, err := billingSvc.GetBalance(context.Background(), 1, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if summary.AvailablePoints != "92.00000" || summary.FrozenPoints != "8.00000" {
		t.Fatalf("expected single reserve after idempotent retry, got %#v", summary)
	}

	thirdReq := httptest.NewRequest(http.MethodPost, "/api/agent/image/v1/tasks", bytes.NewBufferString(`{"task_type":"text_to_image","prompt":"Queue changed","abstract_model":"plus","requested_quality":"auto","requested_size":"1536x1024","requested_output_image_count":1,"response_mode":"async"}`))
	thirdReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	thirdReq.Header.Set("Content-Type", "application/json")
	thirdReq.Header.Set("Idempotency-Key", "same-request")
	thirdRec := httptest.NewRecorder()
	handler.ServeHTTP(thirdRec, thirdReq)
	if thirdRec.Code != http.StatusAccepted {
		t.Fatalf("expected changed-body create 202, got %d body=%s", thirdRec.Code, thirdRec.Body.String())
	}
	var thirdResp struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(thirdRec.Body).Decode(&thirdResp); err != nil {
		t.Fatalf("decode third response: %v", err)
	}
	if thirdResp.Data.ID == firstResp.Data.ID {
		t.Fatalf("expected different task id when body changes under same key, got first=%#v third=%#v", firstResp, thirdResp)
	}
}
