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
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	auditservice "github.com/fatballfish/pic-gallery/internal/service/audit"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
)

func TestGalleryPublishReviewAndPublicListFlow(t *testing.T) {
	imageBytes := tinyPNG(t)
	var generationServer *httptest.Server
	generationServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"images":[{"image_url":{"url":"`+generationServer.URL+`/images/gallery.png"}}]}}]}`)
		case "/images/gallery.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
		default:
			t.Fatalf("unexpected generation path %s", r.URL.Path)
		}
	}))
	defer generationServer.Close()

	moderationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/moderations" {
			t.Fatalf("unexpected moderation path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"flagged":false,"categories":{"violence":false}}]}`)
	}))
	defer moderationServer.Close()

	cfg := taskAPIConfig(generationServer.URL)
	cfg.Providers.OpenAI.Enabled = true
	cfg.Providers.OpenAI.BaseURL = moderationServer.URL
	cfg.Providers.OpenAI.APIKey = "oa-key"

	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "gallery@example.com")

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       1,
		ChangePoints: "100.00000",
		Reason:       "seed balance",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}

	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	adminCfgSvc := adminconfigservice.NewService(cfg)
	if _, err := adminCfgSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:    "public_gallery",
		Version:   1,
		UpdatedBy: 1,
		Items: []domainadminconfig.Item{
			{ConfigCategory: "public_gallery", ConfigKey: "publish_request_enabled", ConfigValue: map[string]any{"value": true}, Scope: "global"},
			{ConfigCategory: "public_gallery", ConfigKey: "gallery_enabled", ConfigValue: map[string]any{"value": true}, Scope: "global"},
		},
	}); err != nil {
		t.Fatalf("UpdateTab public_gallery: %v", err)
	}

	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(context.Background(), domainadminauth.AdminUser{
		Email:        "admin-gallery@example.com",
		PasswordHash: adminauthservice.HashPassword("password"),
		Role:         "super_admin",
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "admin-secret",
		RefreshCookieName: "pg_admin_refresh",
	}, adminStore)

	api := handlers.NewAPIWithAdminServices(cfg, authSvc, nil, taskSvc, adminCfgSvc, billingSvc, nil, adminAuth, auditservice.NewService(nil), nil, nil)
	handler := NewWithAPI(api)

	taskID := createAndProcessAgentTask(t, handler, taskSvc, session.AccessToken)
	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/"+taskID, nil)
	detailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("task detail: %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data struct {
			Results []struct {
				ID string `json:"id"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode task detail: %v", err)
	}
	if len(detailResp.Data.Results) != 1 || detailResp.Data.Results[0].ID == "" {
		t.Fatalf("unexpected detail response %#v", detailResp)
	}
	imageID := detailResp.Data.Results[0].ID

	publishReq := httptest.NewRequest(http.MethodPost, "/api/agent/gallery/v1/images/"+imageID+"/publish", nil)
	publishReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	publishRec := httptest.NewRecorder()
	handler.ServeHTTP(publishRec, publishReq)
	if publishRec.Code != http.StatusAccepted {
		t.Fatalf("publish request: %d body=%s", publishRec.Code, publishRec.Body.String())
	}
	if !bytes.Contains(publishRec.Body.Bytes(), []byte(`"visibility_status":"pending_review"`)) {
		t.Fatalf("expected pending review body=%s", publishRec.Body.String())
	}

	adminToken := loginAdminForGalleryTest(t, handler)

	reviewListReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/image-reviews?page=1&page_size=10&status=pending_review", nil)
	reviewListReq.Header.Set("Authorization", "Bearer "+adminToken)
	reviewListRec := httptest.NewRecorder()
	handler.ServeHTTP(reviewListRec, reviewListReq)
	if reviewListRec.Code != http.StatusOK {
		t.Fatalf("review list: %d body=%s", reviewListRec.Code, reviewListRec.Body.String())
	}
	if !bytes.Contains(reviewListRec.Body.Bytes(), []byte(imageID)) {
		t.Fatalf("expected review list to contain image body=%s", reviewListRec.Body.String())
	}

	approveReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/image-reviews/"+imageID+":approve", bytes.NewBufferString(`{}`))
	approveReq.Header.Set("Authorization", "Bearer "+adminToken)
	approveReq.Header.Set("Content-Type", "application/json")
	approveRec := httptest.NewRecorder()
	handler.ServeHTTP(approveRec, approveReq)
	if approveRec.Code != http.StatusOK {
		t.Fatalf("approve request: %d body=%s", approveRec.Code, approveRec.Body.String())
	}
	if !bytes.Contains(approveRec.Body.Bytes(), []byte(`"visibility_status":"approved"`)) {
		t.Fatalf("expected approved body=%s", approveRec.Body.String())
	}

	dashboardReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/metrics/dashboard", nil)
	dashboardReq.Header.Set("Authorization", "Bearer "+adminToken)
	dashboardRec := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRec, dashboardReq)
	if dashboardRec.Code != http.StatusOK {
		t.Fatalf("dashboard request: %d body=%s", dashboardRec.Code, dashboardRec.Body.String())
	}
	if !bytes.Contains(dashboardRec.Body.Bytes(), []byte(`"metrics"`)) || !bytes.Contains(dashboardRec.Body.Bytes(), []byte(`"queue"`)) {
		t.Fatalf("expected dashboard payload body=%s", dashboardRec.Body.String())
	}

	publicListReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images?page=1&page_size=10", nil)
	publicListRec := httptest.NewRecorder()
	handler.ServeHTTP(publicListRec, publicListReq)
	if publicListRec.Code != http.StatusOK {
		t.Fatalf("public list: %d body=%s", publicListRec.Code, publicListRec.Body.String())
	}
	if !bytes.Contains(publicListRec.Body.Bytes(), []byte(imageID)) {
		t.Fatalf("expected public list to contain image body=%s", publicListRec.Body.String())
	}

	publicDetailReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images/"+imageID, nil)
	publicDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(publicDetailRec, publicDetailReq)
	if publicDetailRec.Code != http.StatusOK {
		t.Fatalf("public detail: %d body=%s", publicDetailRec.Code, publicDetailRec.Body.String())
	}
	if !bytes.Contains(publicDetailRec.Body.Bytes(), []byte(`"visibility_status":"approved"`)) {
		t.Fatalf("expected approved detail body=%s", publicDetailRec.Body.String())
	}
}

func TestGalleryPublishRejectedByModeration(t *testing.T) {
	imageBytes := tinyPNG(t)
	var generationServer *httptest.Server
	generationServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"images":[{"image_url":{"url":"`+generationServer.URL+`/images/reject.png"}}]}}]}`)
		case "/images/reject.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
		default:
			t.Fatalf("unexpected generation path %s", r.URL.Path)
		}
	}))
	defer generationServer.Close()

	moderationServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"results":[{"flagged":true,"categories":{"violence":true,"self-harm":false}}]}`)
	}))
	defer moderationServer.Close()

	cfg := taskAPIConfig(generationServer.URL)
	cfg.Providers.OpenAI.Enabled = true
	cfg.Providers.OpenAI.BaseURL = moderationServer.URL
	cfg.Providers.OpenAI.APIKey = "oa-key"

	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "gallery-reject@example.com")

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       1,
		ChangePoints: "100.00000",
		Reason:       "seed balance",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}

	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	adminCfgSvc := adminconfigservice.NewService(cfg)
	if _, err := adminCfgSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:    "public_gallery",
		Version:   1,
		UpdatedBy: 1,
		Items: []domainadminconfig.Item{
			{ConfigCategory: "public_gallery", ConfigKey: "publish_request_enabled", ConfigValue: map[string]any{"value": true}, Scope: "global"},
			{ConfigCategory: "public_gallery", ConfigKey: "gallery_enabled", ConfigValue: map[string]any{"value": true}, Scope: "global"},
		},
	}); err != nil {
		t.Fatalf("UpdateTab public_gallery: %v", err)
	}

	api := handlers.NewAPIWithServices(cfg, authSvc, nil, taskSvc, adminCfgSvc)
	handler := NewWithAPI(api)

	taskID := createAndProcessAgentTask(t, handler, taskSvc, session.AccessToken)
	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/image/v1/tasks/"+taskID, nil)
	detailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("task detail: %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data struct {
			Results []struct {
				ID string `json:"id"`
			} `json:"results"`
		} `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode task detail: %v", err)
	}
	imageID := detailResp.Data.Results[0].ID

	publishReq := httptest.NewRequest(http.MethodPost, "/api/agent/gallery/v1/images/"+imageID+"/publish", nil)
	publishReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	publishRec := httptest.NewRecorder()
	handler.ServeHTTP(publishRec, publishReq)
	if publishRec.Code != http.StatusOK {
		t.Fatalf("publish request: %d body=%s", publishRec.Code, publishRec.Body.String())
	}
	if !bytes.Contains(publishRec.Body.Bytes(), []byte(`"visibility_status":"rejected"`)) {
		t.Fatalf("expected rejected status body=%s", publishRec.Body.String())
	}
	if !bytes.Contains(publishRec.Body.Bytes(), []byte(`auto_moderation_blocked:violence`)) {
		t.Fatalf("expected moderation reason body=%s", publishRec.Body.String())
	}
}

func loginAdminForGalleryTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"admin-gallery@example.com","password":"password"}`))
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
