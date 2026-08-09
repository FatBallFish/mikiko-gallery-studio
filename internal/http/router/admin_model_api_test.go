package router

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainaudit "github.com/fatballfish/pic-gallery/internal/domain/audit"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	adminuserservice "github.com/fatballfish/pic-gallery/internal/service/adminuser"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	modeladminservice "github.com/fatballfish/pic-gallery/internal/service/modeladmin"
	_ "github.com/mattn/go-sqlite3"
)

const adminModelTestTinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAFgwJ/lqR5DQAAAABJRU5ErkJggg=="

func TestAdminModelManagementEndpoints(t *testing.T) {
	cfg := adminConfigAPIConfig()
	client, err := repoent.Open(dialect.SQLite, "file:admin-model-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	authSvc := authservice.NewServiceWithStore(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"}, entstore.NewAuthStore(client))
	adminStore := entstore.NewAdminAuthStore(client)
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "admin-model@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingStore := entstore.NewBillingStore(client, 5)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, billingStore)
	adminUsers := adminuserservice.NewServiceWithStore(entstore.NewAdminUserStore(client, billingStore), billingSvc)
	auditSvc := auditservice.NewService(entstore.NewAuditStore(client))
	modelAdminSvc := modeladminservice.NewServiceWithStore(entstore.NewModelAdminStore(client))
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, auditSvc, adminUsers, nil, nil, modelAdminSvc)
	handler := NewWithAPI(api)

	adminToken := loginAdminForModelTest(t, handler)

	createAccountReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/model-accounts", bytes.NewBufferString(`{"name":"Current Image Account","adapter_type":"openai_compatible","auth_type":"api_key","base_url":"https://images.example.com","credentials":{"api_key":"test-key"},"status":"enabled","priority":1,"weight":100,"concurrency_limit":2,"timeout_ms":45000}`))
	createAccountReq.Header.Set("Authorization", "Bearer "+adminToken)
	createAccountReq.Header.Set("Content-Type", "application/json")
	createAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(createAccountRec, createAccountReq)
	if createAccountRec.Code != http.StatusCreated {
		t.Fatalf("expected model account create 201, got %d body=%s", createAccountRec.Code, createAccountRec.Body.String())
	}
	var accountResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createAccountRec.Body).Decode(&accountResp); err != nil {
		t.Fatalf("decode model account response: %v", err)
	}

	createAccountModelReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/model-accounts/"+jsonNumber(accountResp.Data.ID)+"/models", bytes.NewBufferString(`{"model_code":"gpt-image-current","display_name":"Current Image","task_types":["text_to_image","image_edit"],"base_resolution":["1k","2k"],"quality":["auto","high"],"size_modes":["auto","ratio","pixel"],"supported_ratios":["1:1","16:9"],"supported_pixel_sizes":["1024x1024"],"supports_custom_ratio":true,"supported_backgrounds":["auto","opaque","transparent"],"supports_custom_size":true,"min_width":512,"max_width":2048,"min_height":640,"max_height":1536,"max_image_count":2,"max_reference_image_count":3,"cost_per_image":"0.04000","currency":"USD","enabled":true}`))
	createAccountModelReq.Header.Set("Authorization", "Bearer "+adminToken)
	createAccountModelReq.Header.Set("Content-Type", "application/json")
	createAccountModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createAccountModelRec, createAccountModelReq)
	if createAccountModelRec.Code != http.StatusCreated {
		t.Fatalf("expected account model create 201, got %d body=%s", createAccountModelRec.Code, createAccountModelRec.Body.String())
	}
	var accountModelResp struct {
		Data struct {
			ID                     int64    `json:"id"`
			BaseResolution         []string `json:"base_resolution"`
			SizeModes              []string `json:"size_modes"`
			SupportedRatios        []string `json:"supported_ratios"`
			SupportedPixelSizes    []string `json:"supported_pixel_sizes"`
			SupportsCustomRatio    bool     `json:"supports_custom_ratio"`
			SupportedBackgrounds   []string `json:"supported_backgrounds"`
			MinWidth               int      `json:"min_width"`
			MaxWidth               int      `json:"max_width"`
			MinHeight              int      `json:"min_height"`
			MaxHeight              int      `json:"max_height"`
			MaxImageCount          int      `json:"max_image_count"`
			MaxReferenceImageCount int      `json:"max_reference_image_count"`
			SupportsCustomSize     bool     `json:"supports_custom_size"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createAccountModelRec.Body).Decode(&accountModelResp); err != nil {
		t.Fatalf("decode account model response: %v", err)
	}
	if !reflect.DeepEqual(accountModelResp.Data.BaseResolution, []string{"1k", "2k"}) || !reflect.DeepEqual(accountModelResp.Data.SizeModes, []string{"auto", "ratio", "pixel"}) ||
		!reflect.DeepEqual(accountModelResp.Data.SupportedRatios, []string{"1:1", "16:9"}) || !reflect.DeepEqual(accountModelResp.Data.SupportedPixelSizes, []string{"1024x1024"}) ||
		!accountModelResp.Data.SupportsCustomRatio || !reflect.DeepEqual(accountModelResp.Data.SupportedBackgrounds, []string{"auto", "opaque", "transparent"}) ||
		accountModelResp.Data.MinWidth != 512 || accountModelResp.Data.MaxWidth != 2048 || accountModelResp.Data.MinHeight != 640 || accountModelResp.Data.MaxHeight != 1536 ||
		accountModelResp.Data.MaxImageCount != 2 || accountModelResp.Data.MaxReferenceImageCount != 3 || !accountModelResp.Data.SupportsCustomSize {
		t.Fatalf("account model capabilities were not preserved: %#v", accountModelResp.Data)
	}

	listAccountModelsReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/model-accounts/"+jsonNumber(accountResp.Data.ID)+"/models", nil)
	listAccountModelsReq.Header.Set("Authorization", "Bearer "+adminToken)
	listAccountModelsRec := httptest.NewRecorder()
	handler.ServeHTTP(listAccountModelsRec, listAccountModelsReq)
	if listAccountModelsRec.Code != http.StatusOK {
		t.Fatalf("expected account model list 200, got %d body=%s", listAccountModelsRec.Code, listAccountModelsRec.Body.String())
	}
	var accountModelListResp struct {
		Data struct {
			Items []struct {
				ID                     int64    `json:"id"`
				BaseResolution         []string `json:"base_resolution"`
				SizeModes              []string `json:"size_modes"`
				SupportedRatios        []string `json:"supported_ratios"`
				SupportedPixelSizes    []string `json:"supported_pixel_sizes"`
				SupportsCustomRatio    bool     `json:"supports_custom_ratio"`
				SupportedBackgrounds   []string `json:"supported_backgrounds"`
				MinWidth               int      `json:"min_width"`
				MaxWidth               int      `json:"max_width"`
				MinHeight              int      `json:"min_height"`
				MaxHeight              int      `json:"max_height"`
				MaxImageCount          int      `json:"max_image_count"`
				MaxReferenceImageCount int      `json:"max_reference_image_count"`
				SupportsCustomSize     bool     `json:"supports_custom_size"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listAccountModelsRec.Body).Decode(&accountModelListResp); err != nil {
		t.Fatalf("decode account model list response: %v", err)
	}
	if len(accountModelListResp.Data.Items) != 1 || accountModelListResp.Data.Items[0].ID != accountModelResp.Data.ID ||
		!reflect.DeepEqual(accountModelListResp.Data.Items[0].BaseResolution, []string{"1k", "2k"}) || !reflect.DeepEqual(accountModelListResp.Data.Items[0].SizeModes, []string{"auto", "ratio", "pixel"}) ||
		!reflect.DeepEqual(accountModelListResp.Data.Items[0].SupportedRatios, []string{"1:1", "16:9"}) || !reflect.DeepEqual(accountModelListResp.Data.Items[0].SupportedPixelSizes, []string{"1024x1024"}) ||
		!accountModelListResp.Data.Items[0].SupportsCustomRatio || !reflect.DeepEqual(accountModelListResp.Data.Items[0].SupportedBackgrounds, []string{"auto", "opaque", "transparent"}) ||
		accountModelListResp.Data.Items[0].MinWidth != 512 || accountModelListResp.Data.Items[0].MaxWidth != 2048 || accountModelListResp.Data.Items[0].MinHeight != 640 || accountModelListResp.Data.Items[0].MaxHeight != 1536 ||
		accountModelListResp.Data.Items[0].MaxImageCount != 2 || accountModelListResp.Data.Items[0].MaxReferenceImageCount != 3 || !accountModelListResp.Data.Items[0].SupportsCustomSize {
		t.Fatalf("account model list lost capabilities: %#v", accountModelListResp.Data.Items)
	}

	updateAccountModelReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/model-accounts/"+jsonNumber(accountResp.Data.ID)+"/models/"+jsonNumber(accountModelResp.Data.ID), bytes.NewBufferString(`{"model_code":"gpt-image-current","display_name":"Current Image","task_types":["text_to_image"],"base_resolution":["1k"],"quality":["auto"],"size_modes":["ratio","pixel"],"supported_ratios":["9:16"],"supported_pixel_sizes":["1024x1024"],"supported_backgrounds":["auto","opaque"],"supports_custom_size":true,"min_width":768,"max_width":1920,"min_height":512,"max_height":1600,"max_image_count":1,"max_reference_image_count":0,"cost_per_image":"0.05000","currency":"USD","enabled":true}`))
	updateAccountModelReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateAccountModelReq.Header.Set("Content-Type", "application/json")
	updateAccountModelRec := httptest.NewRecorder()
	handler.ServeHTTP(updateAccountModelRec, updateAccountModelReq)
	if updateAccountModelRec.Code != http.StatusOK {
		t.Fatalf("expected account model update 200, got %d body=%s", updateAccountModelRec.Code, updateAccountModelRec.Body.String())
	}
	var updatedAccountModelResp struct {
		Data struct {
			BaseResolution         []string `json:"base_resolution"`
			SizeModes              []string `json:"size_modes"`
			SupportedRatios        []string `json:"supported_ratios"`
			SupportedPixelSizes    []string `json:"supported_pixel_sizes"`
			SupportsCustomRatio    bool     `json:"supports_custom_ratio"`
			SupportedBackgrounds   []string `json:"supported_backgrounds"`
			MinWidth               int      `json:"min_width"`
			MaxWidth               int      `json:"max_width"`
			MinHeight              int      `json:"min_height"`
			MaxHeight              int      `json:"max_height"`
			MaxImageCount          int      `json:"max_image_count"`
			MaxReferenceImageCount int      `json:"max_reference_image_count"`
			SupportsCustomSize     bool     `json:"supports_custom_size"`
		} `json:"data"`
	}
	if err := json.NewDecoder(updateAccountModelRec.Body).Decode(&updatedAccountModelResp); err != nil {
		t.Fatalf("decode updated account model response: %v", err)
	}
	if !reflect.DeepEqual(updatedAccountModelResp.Data.BaseResolution, []string{"1k"}) || !reflect.DeepEqual(updatedAccountModelResp.Data.SizeModes, []string{"ratio", "pixel"}) ||
		!reflect.DeepEqual(updatedAccountModelResp.Data.SupportedRatios, []string{"9:16"}) || !reflect.DeepEqual(updatedAccountModelResp.Data.SupportedPixelSizes, []string{"1024x1024"}) ||
		updatedAccountModelResp.Data.SupportsCustomRatio || !reflect.DeepEqual(updatedAccountModelResp.Data.SupportedBackgrounds, []string{"auto", "opaque"}) ||
		updatedAccountModelResp.Data.MinWidth != 768 || updatedAccountModelResp.Data.MaxWidth != 1920 || updatedAccountModelResp.Data.MinHeight != 512 || updatedAccountModelResp.Data.MaxHeight != 1600 ||
		updatedAccountModelResp.Data.MaxImageCount != 1 || updatedAccountModelResp.Data.MaxReferenceImageCount != 0 || !updatedAccountModelResp.Data.SupportsCustomSize {
		t.Fatalf("account model update lost capabilities: %#v", updatedAccountModelResp.Data)
	}

	createProviderReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/model-providers", bytes.NewBufferString(`{"provider_code":"OpenAI","provider_type":"openai","auth_config_encrypted":"cipher","health_status":"healthy","enabled":true}`))
	createProviderReq.Header.Set("Authorization", "Bearer "+adminToken)
	createProviderReq.Header.Set("Content-Type", "application/json")
	createProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderRec, createProviderReq)
	if createProviderRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d body=%s", createProviderRec.Code, createProviderRec.Body.String())
	}
	var providerResp struct {
		Data struct {
			ID           int64  `json:"id"`
			ProviderCode string `json:"provider_code"`
			Enabled      bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createProviderRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}
	if providerResp.Data.ID <= 0 || providerResp.Data.ProviderCode != "openai" || !providerResp.Data.Enabled {
		t.Fatalf("unexpected provider response %#v", providerResp)
	}

	listProviderReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/model-providers?page=1&page_size=10&provider_type=openai&enabled=true", nil)
	listProviderReq.Header.Set("Authorization", "Bearer "+adminToken)
	listProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(listProviderRec, listProviderReq)
	if listProviderRec.Code != http.StatusOK || !bytes.Contains(listProviderRec.Body.Bytes(), []byte("openai")) {
		t.Fatalf("expected provider list 200 with openai, got %d body=%s", listProviderRec.Code, listProviderRec.Body.String())
	}

	updateProviderReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/model-providers/openai", bytes.NewBufferString(`{"provider_code":"openai","provider_type":"openai","auth_config_encrypted":"cipher-2","health_status":"degraded","enabled":false}`))
	updateProviderReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateProviderReq.Header.Set("Content-Type", "application/json")
	updateProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(updateProviderRec, updateProviderReq)
	if updateProviderRec.Code != http.StatusOK {
		t.Fatalf("expected provider update 200, got %d body=%s", updateProviderRec.Code, updateProviderRec.Body.String())
	}

	createProviderModelReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/provider-models", bytes.NewBufferString(`{"provider_code":"openai","model_code":"gpt-image-1","compat_mode":"openai_images","supports_image_input":true,"supports_mask":true,"supported_base_resolution":["1k","2k"],"supported_ratios":["1:1","16:9"],"max_image_count":4,"max_reference_image_count":2,"timeout_ms":45000,"input_cost":"0.12","output_cost":"0.34","currency":"USD","health_status":"healthy","enabled":true}`))
	createProviderModelReq.Header.Set("Authorization", "Bearer "+adminToken)
	createProviderModelReq.Header.Set("Content-Type", "application/json")
	createProviderModelRec := httptest.NewRecorder()
	handler.ServeHTTP(createProviderModelRec, createProviderModelReq)
	if createProviderModelRec.Code != http.StatusCreated {
		t.Fatalf("expected provider model create 201, got %d body=%s", createProviderModelRec.Code, createProviderModelRec.Body.String())
	}
	var providerModelResp struct {
		Data struct {
			ID           int64  `json:"id"`
			ProviderCode string `json:"provider_code"`
			ModelCode    string `json:"model_code"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createProviderModelRec.Body).Decode(&providerModelResp); err != nil {
		t.Fatalf("decode provider model response: %v", err)
	}
	if providerModelResp.Data.ID <= 0 || providerModelResp.Data.ProviderCode != "openai" || providerModelResp.Data.ModelCode != "gpt-image-1" {
		t.Fatalf("unexpected provider model response %#v", providerModelResp)
	}

	listProviderModelsReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/provider-models?page=1&page_size=10&provider_code=openai", nil)
	listProviderModelsReq.Header.Set("Authorization", "Bearer "+adminToken)
	listProviderModelsRec := httptest.NewRecorder()
	handler.ServeHTTP(listProviderModelsRec, listProviderModelsReq)
	if listProviderModelsRec.Code != http.StatusOK || !bytes.Contains(listProviderModelsRec.Body.Bytes(), []byte("gpt-image-1")) {
		t.Fatalf("expected provider model list 200 with model, got %d body=%s", listProviderModelsRec.Code, listProviderModelsRec.Body.String())
	}

	createRouteReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/model-routes", bytes.NewBufferString(`{"group_code":"Plus","task_type":"text_to_image","provider_code":"openai","priority":1,"weight_percent":100,"fallback_order":1,"enabled":true}`))
	createRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	createRouteReq.Header.Set("Content-Type", "application/json")
	createRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(createRouteRec, createRouteReq)
	if createRouteRec.Code != http.StatusCreated {
		t.Fatalf("expected route create 201, got %d body=%s", createRouteRec.Code, createRouteRec.Body.String())
	}
	var routeResp struct {
		Data struct {
			ID           int64  `json:"id"`
			GroupCode    string `json:"group_code"`
			TaskType     string `json:"task_type"`
			ProviderCode string `json:"provider_code"`
			Enabled      bool   `json:"enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRouteRec.Body).Decode(&routeResp); err != nil {
		t.Fatalf("decode route response: %v", err)
	}
	if routeResp.Data.ID <= 0 || routeResp.Data.GroupCode != "plus" || routeResp.Data.ProviderCode != "openai" || !routeResp.Data.Enabled {
		t.Fatalf("unexpected route response %#v", routeResp)
	}

	listRouteReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/model-routes?page=1&page_size=10&group_code=plus&task_type=text_to_image&provider_code=openai&enabled=true", nil)
	listRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(listRouteRec, listRouteReq)
	if listRouteRec.Code != http.StatusOK || !bytes.Contains(listRouteRec.Body.Bytes(), []byte("text_to_image")) {
		t.Fatalf("expected route list 200 with route, got %d body=%s", listRouteRec.Code, listRouteRec.Body.String())
	}

	updateRouteReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/model-routes/"+jsonNumber(routeResp.Data.ID), bytes.NewBufferString(`{"group_code":"plus","task_type":"text_to_image","provider_code":"openai","priority":2,"weight_percent":50,"fallback_order":2,"enabled":false}`))
	updateRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateRouteReq.Header.Set("Content-Type", "application/json")
	updateRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRouteRec, updateRouteReq)
	if updateRouteRec.Code != http.StatusOK {
		t.Fatalf("expected route update 200, got %d body=%s", updateRouteRec.Code, updateRouteRec.Body.String())
	}

	updateProviderModelReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/provider-models/"+jsonNumber(providerModelResp.Data.ID), bytes.NewBufferString(`{"provider_code":"openai","model_code":"gpt-image-1","compat_mode":"openai_images","supports_image_input":true,"supports_mask":true,"supported_base_resolution":["1k","2k","4k"],"supported_ratios":["1:1","16:9"],"max_image_count":5,"max_reference_image_count":2,"timeout_ms":50000,"input_cost":"0.22","output_cost":"0.44","currency":"USD","health_status":"degraded","enabled":false}`))
	updateProviderModelReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateProviderModelReq.Header.Set("Content-Type", "application/json")
	updateProviderModelRec := httptest.NewRecorder()
	handler.ServeHTTP(updateProviderModelRec, updateProviderModelReq)
	if updateProviderModelRec.Code != http.StatusOK {
		t.Fatalf("expected provider model update 200, got %d body=%s", updateProviderModelRec.Code, updateProviderModelRec.Body.String())
	}

	deleteProviderWithRouteReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/model-providers/openai", nil)
	deleteProviderWithRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteProviderWithRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteProviderWithRouteRec, deleteProviderWithRouteReq)
	if deleteProviderWithRouteRec.Code != http.StatusConflict {
		t.Fatalf("expected provider delete with route 409, got %d body=%s", deleteProviderWithRouteRec.Code, deleteProviderWithRouteRec.Body.String())
	}

	deleteRouteReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/model-routes/"+jsonNumber(routeResp.Data.ID), nil)
	deleteRouteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteRouteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRouteRec, deleteRouteReq)
	if deleteRouteRec.Code != http.StatusNoContent {
		t.Fatalf("expected route delete 204, got %d body=%s", deleteRouteRec.Code, deleteRouteRec.Body.String())
	}

	deleteProviderModelReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/provider-models/"+jsonNumber(providerModelResp.Data.ID), nil)
	deleteProviderModelReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteProviderModelRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteProviderModelRec, deleteProviderModelReq)
	if deleteProviderModelRec.Code != http.StatusNoContent {
		t.Fatalf("expected provider model delete 204, got %d body=%s", deleteProviderModelRec.Code, deleteProviderModelRec.Body.String())
	}

	deleteProviderReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/model-providers/openai", nil)
	deleteProviderReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteProviderRec, deleteProviderReq)
	if deleteProviderRec.Code != http.StatusNoContent {
		t.Fatalf("expected provider delete 204, got %d body=%s", deleteProviderRec.Code, deleteProviderRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/audit-logs?page=1&page_size=10&actor_type=admin", nil)
	auditReq.Header.Set("Authorization", "Bearer "+adminToken)
	auditRec := httptest.NewRecorder()
	handler.ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK {
		t.Fatalf("expected audit list 200, got %d body=%s", auditRec.Code, auditRec.Body.String())
	}
	if !bytes.Contains(auditRec.Body.Bytes(), []byte(`"pagination"`)) {
		t.Fatalf("expected audit pagination, got body=%s", auditRec.Body.String())
	}
	for _, action := range []string{"model_provider.create", "model_provider.update", "model_provider.delete", "provider_model.create", "provider_model.update", "provider_model.delete", "model_route.create", "model_route.update", "model_route.delete"} {
		if !bytes.Contains(auditRec.Body.Bytes(), []byte(action)) {
			t.Fatalf("expected audit log action %q, got body=%s", action, auditRec.Body.String())
		}
	}
}

func TestAdminModelLifecycleReportsDependenciesAndAuditsDeletionState(t *testing.T) {
	cfg := adminConfigAPIConfig()
	client, err := repoent.Open(dialect.SQLite, "file:admin-model-lifecycle-api?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	authSvc := authservice.NewServiceWithStore(config.AuthConfig{
		AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test",
		AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"}, entstore.NewAuthStore(client))
	adminStore := entstore.NewAdminAuthStore(client)
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "admin-model@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingStore := entstore.NewBillingStore(client, 5)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, billingStore)
	adminUsers := adminuserservice.NewServiceWithStore(entstore.NewAdminUserStore(client, billingStore), billingSvc)
	auditSvc := auditservice.NewService(entstore.NewAuditStore(client))
	modelStore := entstore.NewModelAdminStore(client)
	modelAdminSvc := modeladminservice.NewServiceWithStore(modelStore)
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, auditSvc, adminUsers, nil, nil, modelAdminSvc)
	handler := NewWithAPI(api)
	adminToken := loginAdminForModelTest(t, handler)

	account, err := modelStore.CreateModelAccount(t.Context(), domainmodeladmin.ModelAccountWriteRequest{
		Name: "Lifecycle account", AdapterType: "openai_compatible", AuthType: "api_key", BaseURL: "https://images.example.com",
		Credentials: map[string]string{"api_key": "secret"}, Status: "enabled", Priority: 1, Weight: 100, ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	model, err := modelStore.CreateModelAccountModel(t.Context(), domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: account.ID, ModelCode: "gpt-image-lifecycle", DisplayName: "Lifecycle model", TaskTypes: []string{"text_to_image"},
		BaseResolution: []string{"1k"}, Quality: []string{"auto"}, MaxImageCount: 1, SizeModes: []string{"auto"}, CostPerImage: "0.04", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create account model: %v", err)
	}

	deleteAccount := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/model-accounts/"+jsonNumber(account.ID), nil)
	deleteAccount.Header.Set("Authorization", "Bearer "+adminToken)
	deleteAccountRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteAccountRec, deleteAccount)
	if deleteAccountRec.Code != http.StatusConflict {
		t.Fatalf("expected account dependency conflict 409, got %d body=%s", deleteAccountRec.Code, deleteAccountRec.Body.String())
	}
	var conflict struct {
		Error struct {
			Code    string         `json:"code"`
			Details map[string]any `json:"details"`
		} `json:"error"`
	}
	if err := json.NewDecoder(deleteAccountRec.Body).Decode(&conflict); err != nil {
		t.Fatalf("decode dependency conflict: %v", err)
	}
	if conflict.Error.Code != "configuration_in_use" || conflict.Error.Details["dependency"] != "account_models" || conflict.Error.Details["count"] != float64(1) {
		t.Fatalf("unexpected dependency conflict: %#v", conflict.Error)
	}

	deleteModel := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/model-accounts/"+jsonNumber(account.ID)+"/models/"+jsonNumber(model.ID), nil)
	deleteModel.Header.Set("Authorization", "Bearer "+adminToken)
	deleteModel.Header.Set("X-Request-Id", "lifecycle-delete-request")
	deleteModelRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteModelRec, deleteModel)
	if deleteModelRec.Code != http.StatusNoContent {
		t.Fatalf("expected model delete 204, got %d body=%s", deleteModelRec.Code, deleteModelRec.Body.String())
	}

	auditPage, err := auditSvc.List(t.Context(), domainaudit.ListRequest{Page: 1, PageSize: 20, Action: "model_account_model.delete", TargetID: jsonNumber(model.ID)})
	if err != nil {
		t.Fatalf("list deletion audit: %v", err)
	}
	if len(auditPage.Items) != 1 {
		t.Fatalf("expected one deletion audit, got %d", len(auditPage.Items))
	}
	metadata := auditPage.Items[0].Metadata
	if metadata["request_id"] == "" || metadata["before"] == nil || metadata["after"] == nil || metadata["account_id"] != float64(account.ID) {
		t.Fatalf("deletion audit missing lifecycle metadata: %#v", metadata)
	}

	repeatDeleteModel := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/model-accounts/"+jsonNumber(account.ID)+"/models/"+jsonNumber(model.ID), nil)
	repeatDeleteModel.Header.Set("Authorization", "Bearer "+adminToken)
	repeatDeleteModelRec := httptest.NewRecorder()
	handler.ServeHTTP(repeatDeleteModelRec, repeatDeleteModel)
	if repeatDeleteModelRec.Code != http.StatusNoContent {
		t.Fatalf("expected repeated model delete 204, got %d body=%s", repeatDeleteModelRec.Code, repeatDeleteModelRec.Body.String())
	}

	model2, err := modelStore.CreateModelAccountModel(t.Context(), domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: account.ID, ModelCode: "gpt-image-route", DisplayName: "Route model", TaskTypes: []string{"text_to_image"},
		BaseResolution: []string{"1k"}, Quality: []string{"auto"}, MaxImageCount: 1, SizeModes: []string{"auto"}, CostPerImage: "0.04", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("create second account model: %v", err)
	}
	route, err := modelStore.CreateRouteModel(t.Context(), domainmodeladmin.RouteModelWriteRequest{Code: "route-lifecycle", Name: "Lifecycle route", Visibility: "public", Enabled: true})
	if err != nil {
		t.Fatalf("create route model: %v", err)
	}
	candidate, err := modelStore.CreateRouteModelCandidate(t.Context(), domainmodeladmin.RouteModelCandidateWriteRequest{RouteModelID: route.ID, AccountModelID: model2.ID, Priority: 1, Weight: 100, FallbackOrder: 1, Enabled: true})
	if err != nil {
		t.Fatalf("create route candidate: %v", err)
	}
	price, err := modelStore.CreateRouteModelPrice(t.Context(), domainmodeladmin.RouteModelPriceWriteRequest{RouteModelID: route.ID, TaskType: "text_to_image", BaseResolution: "1k", BasePoints: "1", ReferenceMultiplier: "1", Enabled: true})
	if err != nil {
		t.Fatalf("create route price: %v", err)
	}

	assertDelete := func(path, requestID string, wantStatus int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodDelete, path, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		if requestID != "" {
			req.Header.Set("X-Request-Id", requestID)
		}
		req.Header.Set("User-Agent", "task8-lifecycle-test")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != wantStatus {
			t.Fatalf("DELETE %s: expected %d, got %d body=%s", path, wantStatus, rec.Code, rec.Body.String())
		}
	}

	assertDelete("/api/ops/admin/v1/route-models/"+jsonNumber(route.ID), "", http.StatusConflict)
	assertDelete("/api/ops/admin/v1/model-accounts/"+jsonNumber(account.ID)+"/models/"+jsonNumber(model2.ID), "", http.StatusConflict)
	assertDelete("/api/ops/admin/v1/route-models/"+jsonNumber(route.ID)+"/candidates/"+jsonNumber(candidate.ID), "delete-candidate-request", http.StatusNoContent)
	assertDelete("/api/ops/admin/v1/route-models/"+jsonNumber(route.ID)+"/candidates/"+jsonNumber(candidate.ID), "", http.StatusNoContent)
	assertDelete("/api/ops/admin/v1/route-models/"+jsonNumber(route.ID), "", http.StatusConflict)
	assertDelete("/api/ops/admin/v1/route-model-prices/"+jsonNumber(price.ID), "delete-price-request", http.StatusNoContent)
	assertDelete("/api/ops/admin/v1/route-model-prices/"+jsonNumber(price.ID), "", http.StatusNoContent)
	assertDelete("/api/ops/admin/v1/route-models/"+jsonNumber(route.ID), "delete-route-request", http.StatusNoContent)
	assertDelete("/api/ops/admin/v1/route-models/"+jsonNumber(route.ID), "", http.StatusNoContent)
	assertDelete("/api/ops/admin/v1/model-accounts/"+jsonNumber(account.ID)+"/models/"+jsonNumber(model2.ID), "delete-model-request", http.StatusNoContent)
	assertDelete("/api/ops/admin/v1/model-accounts/"+jsonNumber(account.ID), "delete-account-request", http.StatusNoContent)
	assertDelete("/api/ops/admin/v1/model-accounts/"+jsonNumber(account.ID), "", http.StatusNoContent)

	for _, expected := range []struct {
		action, targetID, requestID string
	}{
		{"route_model_candidate.delete", jsonNumber(candidate.ID), "delete-candidate-request"},
		{"route_model_price.delete", jsonNumber(price.ID), "delete-price-request"},
		{"route_model.delete", jsonNumber(route.ID), "delete-route-request"},
		{"model_account_model.delete", jsonNumber(model2.ID), "delete-model-request"},
		{"model_account.delete", jsonNumber(account.ID), "delete-account-request"},
	} {
		page, listErr := auditSvc.List(t.Context(), domainaudit.ListRequest{Page: 1, PageSize: 20, Action: expected.action, TargetID: expected.targetID})
		if listErr != nil || len(page.Items) != 1 {
			t.Fatalf("expected one %s audit, got page=%#v err=%v", expected.action, page, listErr)
		}
		got := page.Items[0].Metadata
		if got["request_id"] != expected.requestID || got["before"] == nil || got["after"] == nil {
			t.Fatalf("%s audit missing lifecycle metadata: %#v", expected.action, got)
		}
		log := page.Items[0]
		if log.ActorType != "admin" || log.ActorID != "1" || log.TargetID != expected.targetID || log.IPAddr == "" || log.UserAgent != "task8-lifecycle-test" {
			t.Fatalf("%s audit missing actor/target/request identity: %#v", expected.action, log)
		}
	}
}

func TestAdminModelAccountTestImageUsesDirectAccountAndActualParams(t *testing.T) {
	var upstreamReq struct {
		Model             string `json:"model"`
		Prompt            string `json:"prompt"`
		Size              string `json:"size"`
		Quality           string `json:"quality"`
		OutputFormat      string `json:"output_format"`
		OutputCompression int    `json:"output_compression"`
		Moderation        string `json:"moderation"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/images/generations" {
			t.Fatalf("unexpected upstream request %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&upstreamReq); err != nil {
			t.Fatalf("decode upstream request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("x-request-id", "req-admin-model-test")
		_, _ = w.Write([]byte(`{"created":1770000100,"data":[{"b64_json":"` + adminModelTestTinyPNGBase64 + `"}]}`))
	}))
	defer upstream.Close()

	cfg := adminConfigAPIConfig()
	cfg.Storage.LocalRoot = t.TempDir()
	client, err := repoent.Open(dialect.SQLite, "file:admin-model-test-image?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	authSvc := authservice.NewServiceWithStore(cfg.Auth, cfg.Billing.UserGroupMultipliers, entstore.NewAuthStore(client))
	adminStore := entstore.NewAdminAuthStore(client)
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "admin-model@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingStore := entstore.NewBillingStore(client, 5)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, billingStore)
	modelAdminSvc := modeladminservice.NewServiceWithStore(entstore.NewModelAdminStore(client))
	account, err := modelAdminSvc.CreateModelAccount(t.Context(), domainmodeladmin.ModelAccountWriteRequest{
		Name:             "OpenAI Test",
		AdapterType:      domainmodeladmin.AdapterTypeOpenAICompatible,
		AuthType:         domainmodeladmin.AuthTypeAPIKey,
		BaseURL:          upstream.URL,
		Credentials:      map[string]string{"api_key": "test-key"},
		Status:           domainmodeladmin.ModelAccountStatusEnabled,
		ConcurrencyLimit: 1,
		Extra:            map[string]any{"source_mode": "codex_responses"},
	})
	if err != nil {
		t.Fatalf("CreateModelAccount: %v", err)
	}
	model, err := modelAdminSvc.CreateModelAccountModel(t.Context(), domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID:                 account.ID,
		ModelCode:                 "gpt-image-2",
		DisplayName:               "GPT Image 2",
		TaskTypes:                 []string{"text_to_image"},
		BaseResolution:            []string{"1k", "2k", "4k"},
		Quality:                   []string{"auto", "high"},
		OutputFormat:              []string{"png", "webp"},
		MaxImageCount:             1,
		SupportsOutputCompression: true,
		Moderation:                []string{"auto", "low"},
		CostPerImage:              "0.00000",
		Currency:                  "USD",
		Enabled:                   true,
	})
	if err != nil {
		t.Fatalf("CreateModelAccountModel: %v", err)
	}
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil, nil, nil, nil, modelAdminSvc)
	handler := NewWithAPI(api)
	adminToken := loginAdminForModelTest(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/model-accounts/"+jsonNumber(account.ID)+"/test-image", bytes.NewBufferString(`{"model_id":`+jsonNumber(model.ID)+`,"prompt":"admin smoke image","source_mode":"codex_responses","quality":"high","output_format":"webp","output_compression":72,"moderation":"low"}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected model test image 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Status            string            `json:"status"`
			ImageURL          string            `json:"image_url"`
			Width             int               `json:"width"`
			Height            int               `json:"height"`
			ProviderRequestID string            `json:"provider_request_id"`
			ActualParams      map[string]string `json:"actual_params"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if upstreamReq.Model != "gpt-image-2" || upstreamReq.Quality != "auto" || upstreamReq.Size != "1024x1024" || upstreamReq.OutputFormat != "webp" || upstreamReq.OutputCompression != 72 || upstreamReq.Moderation != "low" {
		t.Fatalf("unexpected upstream params %#v", upstreamReq)
	}
	if resp.Data.Status != "succeeded" || resp.Data.ProviderRequestID != "req-admin-model-test" || resp.Data.Width != 1 || resp.Data.Height != 1 {
		t.Fatalf("unexpected response data %#v", resp.Data)
	}
	if resp.Data.ActualParams["quality"] != "auto" || resp.Data.ActualParams["size"] != "1024x1024" || resp.Data.ActualParams["output_format"] != "webp" || resp.Data.ActualParams["output_compression"] != "72" || resp.Data.ActualParams["moderation"] != "low" {
		t.Fatalf("unexpected actual params %#v", resp.Data.ActualParams)
	}
	if resp.Data.ImageURL == "" || !bytes.Contains([]byte(resp.Data.ImageURL), []byte("/api/ops/admin/v1/image-reviews/")) {
		t.Fatalf("expected admin preview image url, got %q", resp.Data.ImageURL)
	}
}

func TestAdminModelAccountTestImageMapsUpstreamTransportError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			t.Fatalf("response writer does not support hijacking")
		}
		conn, _, err := hijacker.Hijack()
		if err != nil {
			t.Fatalf("hijack upstream connection: %v", err)
		}
		_ = conn.Close()
	}))
	defer upstream.Close()

	cfg := adminConfigAPIConfig()
	cfg.Storage.LocalRoot = t.TempDir()
	client, err := repoent.Open(dialect.SQLite, "file:admin-model-test-image-upstream-error?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	authSvc := authservice.NewServiceWithStore(cfg.Auth, cfg.Billing.UserGroupMultipliers, entstore.NewAuthStore(client))
	adminStore := entstore.NewAdminAuthStore(client)
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{Email: "admin-model@example.com", PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"), Role: "super_admin", Status: "active"}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingStore := entstore.NewBillingStore(client, 5)
	billingSvc := billingservice.NewServiceWithStore(cfg.Billing, billingStore)
	modelAdminSvc := modeladminservice.NewServiceWithStore(entstore.NewModelAdminStore(client))
	account, err := modelAdminSvc.CreateModelAccount(t.Context(), domainmodeladmin.ModelAccountWriteRequest{
		Name:             "OpenAI Test",
		AdapterType:      domainmodeladmin.AdapterTypeOpenAICompatible,
		AuthType:         domainmodeladmin.AuthTypeAPIKey,
		BaseURL:          upstream.URL,
		Credentials:      map[string]string{"api_key": "test-key"},
		Status:           domainmodeladmin.ModelAccountStatusEnabled,
		ConcurrencyLimit: 1,
		TimeoutMS:        120000,
		Extra:            map[string]any{"source_mode": "codex_responses"},
	})
	if err != nil {
		t.Fatalf("CreateModelAccount: %v", err)
	}
	model, err := modelAdminSvc.CreateModelAccountModel(t.Context(), domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID:      account.ID,
		ModelCode:      "gpt-image-2",
		DisplayName:    "GPT Image 2",
		TaskTypes:      []string{"text_to_image"},
		BaseResolution: []string{"1k", "2k", "4k"},
		MaxImageCount:  1,
		CostPerImage:   "0.00000",
		Currency:       "USD",
		Enabled:        true,
	})
	if err != nil {
		t.Fatalf("CreateModelAccountModel: %v", err)
	}
	api := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil, nil, nil, nil, modelAdminSvc)
	handler := NewWithAPI(api)
	adminToken := loginAdminForModelTest(t, handler)

	req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/model-accounts/"+jsonNumber(account.ID)+"/test-image", bytes.NewBufferString(`{"model_id":`+jsonNumber(model.ID)+`,"prompt":"admin smoke image","source_mode":"codex_responses"}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected upstream unavailable 503, got %d body=%s", rec.Code, rec.Body.String())
	}
	body, _ := io.ReadAll(rec.Body)
	if !bytes.Contains(body, []byte(`"code":"UPSTREAM_UNAVAILABLE"`)) || bytes.Contains(body, []byte(`"code":"INTERNAL_ERROR"`)) {
		t.Fatalf("expected upstream unavailable error body, got %s", string(body))
	}
}

func loginAdminForModelTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin-model@example.com","password":"password"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected admin login 200, got %d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.NewDecoder(loginRec.Body).Decode(&loginResp); err != nil {
		t.Fatalf("decode admin login response: %v", err)
	}
	if loginResp.Data.AccessToken == "" {
		t.Fatalf("expected admin access token")
	}
	return loginResp.Data.AccessToken
}
