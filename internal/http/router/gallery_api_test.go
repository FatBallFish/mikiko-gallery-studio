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

	groupReq := httptest.NewRequest(http.MethodPut, "/api/agent/gallery/v1/images/"+imageID+"/group", bytes.NewBufferString(`{"image_group":"客户素材"}`))
	groupReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	groupReq.Header.Set("Content-Type", "application/json")
	groupRec := httptest.NewRecorder()
	handler.ServeHTTP(groupRec, groupReq)
	if groupRec.Code != http.StatusOK {
		t.Fatalf("set image group: %d body=%s", groupRec.Code, groupRec.Body.String())
	}
	if !bytes.Contains(groupRec.Body.Bytes(), []byte(`"image_group":"客户素材"`)) {
		t.Fatalf("expected image group in response body=%s", groupRec.Body.String())
	}

	privateListReq := httptest.NewRequest(http.MethodGet, "/api/agent/gallery/v1/images?page=1&page_size=10", nil)
	privateListReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	privateListRec := httptest.NewRecorder()
	handler.ServeHTTP(privateListRec, privateListReq)
	if privateListRec.Code != http.StatusOK {
		t.Fatalf("private gallery list: %d body=%s", privateListRec.Code, privateListRec.Body.String())
	}
	if !bytes.Contains(privateListRec.Body.Bytes(), []byte(`"image_group":"客户素材"`)) {
		t.Fatalf("expected persisted image group in list body=%s", privateListRec.Body.String())
	}

	adminToken := loginAdminForGalleryTest(t, handler)
	prePublishListReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/image-reviews?page=1&page_size=10", nil)
	prePublishListReq.Header.Set("Authorization", "Bearer "+adminToken)
	prePublishListRec := httptest.NewRecorder()
	handler.ServeHTTP(prePublishListRec, prePublishListReq)
	if prePublishListRec.Code != http.StatusOK {
		t.Fatalf("pre-publish review list: %d body=%s", prePublishListRec.Code, prePublishListRec.Body.String())
	}
	if bytes.Contains(prePublishListRec.Body.Bytes(), []byte(imageID)) {
		t.Fatalf("private image should not appear in review queue body=%s", prePublishListRec.Body.String())
	}

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
	cancelReq := httptest.NewRequest(http.MethodDelete, "/api/agent/gallery/v1/images/"+imageID+"/publish", nil)
	cancelReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	cancelRec := httptest.NewRecorder()
	handler.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK || !bytes.Contains(cancelRec.Body.Bytes(), []byte(`"visibility_status":"private"`)) {
		t.Fatalf("cancel pending publish: status=%d body=%s", cancelRec.Code, cancelRec.Body.String())
	}
	canceledReviewReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/image-reviews?page=1&page_size=10&status=pending_review", nil)
	canceledReviewReq.Header.Set("Authorization", "Bearer "+adminToken)
	canceledReviewRec := httptest.NewRecorder()
	handler.ServeHTTP(canceledReviewRec, canceledReviewReq)
	if canceledReviewRec.Code != http.StatusOK || bytes.Contains(canceledReviewRec.Body.Bytes(), []byte(imageID)) {
		t.Fatalf("canceled image must leave review queue: status=%d body=%s", canceledReviewRec.Code, canceledReviewRec.Body.String())
	}
	reapplyReq := httptest.NewRequest(http.MethodPost, "/api/agent/gallery/v1/images/"+imageID+"/publish", nil)
	reapplyReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	reapplyRec := httptest.NewRecorder()
	handler.ServeHTTP(reapplyRec, reapplyReq)
	if reapplyRec.Code != http.StatusAccepted || !bytes.Contains(reapplyRec.Body.Bytes(), []byte(`"visibility_status":"pending_review"`)) {
		t.Fatalf("reapply publish: status=%d body=%s", reapplyRec.Code, reapplyRec.Body.String())
	}

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

	reviewImageReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/image-reviews/"+imageID+"/image?access_token="+adminToken, nil)
	reviewImageRec := httptest.NewRecorder()
	handler.ServeHTTP(reviewImageRec, reviewImageReq)
	if reviewImageRec.Code != http.StatusOK {
		t.Fatalf("review image: %d body=%s", reviewImageRec.Code, reviewImageRec.Body.String())
	}
	if got := reviewImageRec.Body.Bytes(); !bytes.Equal(got, imageBytes) {
		t.Fatalf("review image bytes mismatch: got %d bytes want %d", len(got), len(imageBytes))
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
	if bytes.Contains(publicListRec.Body.Bytes(), []byte(`Generate a downloadable banner`)) {
		t.Fatalf("guest public list should not expose full prompt body=%s", publicListRec.Body.String())
	}
	if bytes.Contains(publicListRec.Body.Bytes(), []byte(`comment_count`)) {
		t.Fatalf("public list should not expose comment_count because comments are not a product capability body=%s", publicListRec.Body.String())
	}
	var publicListPayload struct {
		Data struct {
			Items []struct {
				Prompt        string `json:"prompt"`
				PromptExcerpt string `json:"prompt_excerpt"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(publicListRec.Body.Bytes())).Decode(&publicListPayload); err != nil {
		t.Fatalf("decode public list: %v", err)
	}
	if len(publicListPayload.Data.Items) == 0 {
		t.Fatalf("expected public list item body=%s", publicListRec.Body.String())
	}
	if publicListPayload.Data.Items[0].Prompt != "" {
		t.Fatalf("guest public list should redact prompt, got %q body=%s", publicListPayload.Data.Items[0].Prompt, publicListRec.Body.String())
	}
	if publicListPayload.Data.Items[0].PromptExcerpt == "" {
		t.Fatalf("guest public list should expose prompt excerpt body=%s", publicListRec.Body.String())
	}
	if publicListPayload.Data.Items[0].PromptExcerpt == "Generate a downloadable banner" || len([]rune(publicListPayload.Data.Items[0].PromptExcerpt)) > 40 {
		t.Fatalf("guest prompt excerpt should be short and not full prompt, got %q", publicListPayload.Data.Items[0].PromptExcerpt)
	}

	queryListReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images?page=1&page_size=10&query=downloadable", nil)
	queryListRec := httptest.NewRecorder()
	handler.ServeHTTP(queryListRec, queryListReq)
	if queryListRec.Code != http.StatusOK {
		t.Fatalf("public list filtered by query: %d body=%s", queryListRec.Code, queryListRec.Body.String())
	}
	if !bytes.Contains(queryListRec.Body.Bytes(), []byte(imageID)) {
		t.Fatalf("expected matching public gallery query to include image body=%s", queryListRec.Body.String())
	}
	if bytes.Contains(queryListRec.Body.Bytes(), []byte(`Generate a downloadable banner`)) {
		t.Fatalf("public gallery query list should still redact full prompt body=%s", queryListRec.Body.String())
	}

	queryMissReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images?page=1&page_size=10&query=not-a-gallery-keyword", nil)
	queryMissRec := httptest.NewRecorder()
	handler.ServeHTTP(queryMissRec, queryMissReq)
	if queryMissRec.Code != http.StatusOK {
		t.Fatalf("public list filtered by missing query: %d body=%s", queryMissRec.Code, queryMissRec.Body.String())
	}
	if bytes.Contains(queryMissRec.Body.Bytes(), []byte(imageID)) {
		t.Fatalf("expected non-matching public gallery query to exclude image body=%s", queryMissRec.Body.String())
	}

	filteredByModelReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images?page=1&page_size=10&route_model_code=plus&task_type=text_to_image", nil)
	filteredByModelRec := httptest.NewRecorder()
	handler.ServeHTTP(filteredByModelRec, filteredByModelReq)
	if filteredByModelRec.Code != http.StatusOK {
		t.Fatalf("public list filtered by model and task type: %d body=%s", filteredByModelRec.Code, filteredByModelRec.Body.String())
	}
	if !bytes.Contains(filteredByModelRec.Body.Bytes(), []byte(imageID)) {
		t.Fatalf("expected matching route_model_code/task_type filter to include image body=%s", filteredByModelRec.Body.String())
	}

	filteredOutReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images?page=1&page_size=10&route_model_code=missing&task_type=text_to_image", nil)
	filteredOutRec := httptest.NewRecorder()
	handler.ServeHTTP(filteredOutRec, filteredOutReq)
	if filteredOutRec.Code != http.StatusOK {
		t.Fatalf("public list filtered out by model: %d body=%s", filteredOutRec.Code, filteredOutRec.Body.String())
	}
	if bytes.Contains(filteredOutRec.Body.Bytes(), []byte(imageID)) {
		t.Fatalf("expected non-matching route_model_code filter to exclude image body=%s", filteredOutRec.Body.String())
	}

	guestDetailReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images/"+imageID, nil)
	guestDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(guestDetailRec, guestDetailReq)
	if guestDetailRec.Code != http.StatusUnauthorized {
		t.Fatalf("guest public detail should require login, got %d body=%s", guestDetailRec.Code, guestDetailRec.Body.String())
	}
	operationsDashboardReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/metrics/dashboard", nil)
	operationsDashboardReq.Header.Set("Authorization", "Bearer "+adminToken)
	operationsDashboardRec := httptest.NewRecorder()
	handler.ServeHTTP(operationsDashboardRec, operationsDashboardReq)
	if operationsDashboardRec.Code != http.StatusOK {
		t.Fatalf("operations dashboard request: %d body=%s", operationsDashboardRec.Code, operationsDashboardRec.Body.String())
	}
	var operationsDashboard struct {
		Data struct {
			Operations struct {
				PublicGalleryListViews         uint64         `json:"public_gallery_list_views"`
				PublicGalleryDetailLoginBlocks uint64         `json:"public_gallery_detail_login_blocks"`
				PaymentSuccessRate             string         `json:"payment_success_rate"`
				PreflightFailuresByErrorCode   map[string]int `json:"preflight_failures_by_error_code"`
			} `json:"operations"`
			Metrics []struct {
				Key string `json:"key"`
			} `json:"metrics"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(operationsDashboardRec.Body.Bytes())).Decode(&operationsDashboard); err != nil {
		t.Fatalf("decode operations dashboard: %v", err)
	}
	if operationsDashboard.Data.Operations.PublicGalleryListViews == 0 {
		t.Fatalf("expected public gallery list views in dashboard body=%s", operationsDashboardRec.Body.String())
	}
	if operationsDashboard.Data.Operations.PublicGalleryDetailLoginBlocks == 0 {
		t.Fatalf("expected public gallery login blocks in dashboard body=%s", operationsDashboardRec.Body.String())
	}
	if operationsDashboard.Data.Operations.PaymentSuccessRate == "" || operationsDashboard.Data.Operations.PreflightFailuresByErrorCode == nil {
		t.Fatalf("expected operations payment/preflight fields body=%s", operationsDashboardRec.Body.String())
	}
	metricKeys := map[string]bool{}
	for _, metric := range operationsDashboard.Data.Metrics {
		metricKeys[metric.Key] = true
	}
	for _, key := range []string{"payment_success_rate", "failed_webhook_count", "signup_trial_users", "preflight_failures", "public_gallery_views", "mock_payment"} {
		if !metricKeys[key] {
			t.Fatalf("expected dashboard metric key %q body=%s", key, operationsDashboardRec.Body.String())
		}
	}

	viewerDetailReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images/"+imageID+"?access_token="+session.AccessToken, nil)
	viewerDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(viewerDetailRec, viewerDetailReq)
	if viewerDetailRec.Code != http.StatusOK {
		t.Fatalf("viewer public detail: %d body=%s", viewerDetailRec.Code, viewerDetailRec.Body.String())
	}
	if !bytes.Contains(viewerDetailRec.Body.Bytes(), []byte(`Generate a downloadable banner`)) {
		t.Fatalf("viewer public detail should expose full prompt body=%s", viewerDetailRec.Body.String())
	}
	if bytes.Contains(viewerDetailRec.Body.Bytes(), []byte(`comment_count`)) {
		t.Fatalf("viewer public detail should not expose comment_count because comments are not a product capability body=%s", viewerDetailRec.Body.String())
	}

	likeReq := httptest.NewRequest(http.MethodPost, "/api/agent/gallery/v1/images/"+imageID+"/like", bytes.NewBufferString(`{"active":true}`))
	likeReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	likeReq.Header.Set("Content-Type", "application/json")
	likeRec := httptest.NewRecorder()
	handler.ServeHTTP(likeRec, likeReq)
	if likeRec.Code != http.StatusOK {
		t.Fatalf("like public image: %d body=%s", likeRec.Code, likeRec.Body.String())
	}
	if !bytes.Contains(likeRec.Body.Bytes(), []byte(`"like_count":1`)) || !bytes.Contains(likeRec.Body.Bytes(), []byte(`"liked_by_viewer":true`)) {
		t.Fatalf("expected liked response body=%s", likeRec.Body.String())
	}
	if bytes.Contains(likeRec.Body.Bytes(), []byte(`comment_count`)) {
		t.Fatalf("like response should not expose comment_count because comments are not a product capability body=%s", likeRec.Body.String())
	}

	favoriteReq := httptest.NewRequest(http.MethodPost, "/api/agent/gallery/v1/images/"+imageID+"/favorite", bytes.NewBufferString(`{"active":true}`))
	favoriteReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	favoriteReq.Header.Set("Content-Type", "application/json")
	favoriteRec := httptest.NewRecorder()
	handler.ServeHTTP(favoriteRec, favoriteReq)
	if favoriteRec.Code != http.StatusOK {
		t.Fatalf("favorite public image: %d body=%s", favoriteRec.Code, favoriteRec.Body.String())
	}
	if !bytes.Contains(favoriteRec.Body.Bytes(), []byte(`"favorite_count":1`)) || !bytes.Contains(favoriteRec.Body.Bytes(), []byte(`"favorited_by_viewer":true`)) {
		t.Fatalf("expected favorited response body=%s", favoriteRec.Body.String())
	}
	if bytes.Contains(favoriteRec.Body.Bytes(), []byte(`comment_count`)) {
		t.Fatalf("favorite response should not expose comment_count because comments are not a product capability body=%s", favoriteRec.Body.String())
	}

	likedListReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images?page=1&page_size=10&sort=hot&liked=true&access_token="+session.AccessToken, nil)
	likedListRec := httptest.NewRecorder()
	handler.ServeHTTP(likedListRec, likedListReq)
	if likedListRec.Code != http.StatusOK {
		t.Fatalf("liked public list: %d body=%s", likedListRec.Code, likedListRec.Body.String())
	}
	if !bytes.Contains(likedListRec.Body.Bytes(), []byte(imageID)) || !bytes.Contains(likedListRec.Body.Bytes(), []byte(`"liked_by_viewer":true`)) {
		t.Fatalf("expected liked public list to contain viewer state body=%s", likedListRec.Body.String())
	}

	favoritedListReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images?page=1&page_size=10&favorited=true&access_token="+session.AccessToken, nil)
	favoritedListRec := httptest.NewRecorder()
	handler.ServeHTTP(favoritedListRec, favoritedListReq)
	if favoritedListRec.Code != http.StatusOK {
		t.Fatalf("favorited public list: %d body=%s", favoritedListRec.Code, favoritedListRec.Body.String())
	}
	if !bytes.Contains(favoritedListRec.Body.Bytes(), []byte(imageID)) || !bytes.Contains(favoritedListRec.Body.Bytes(), []byte(`"favorited_by_viewer":true`)) {
		t.Fatalf("expected favorited public list to contain viewer state body=%s", favoritedListRec.Body.String())
	}

	publicDetailReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images/"+imageID+"?access_token="+session.AccessToken, nil)
	publicDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(publicDetailRec, publicDetailReq)
	if publicDetailRec.Code != http.StatusOK {
		t.Fatalf("public detail: %d body=%s", publicDetailRec.Code, publicDetailRec.Body.String())
	}
	if !bytes.Contains(publicDetailRec.Body.Bytes(), []byte(`"visibility_status":"approved"`)) {
		t.Fatalf("expected approved detail body=%s", publicDetailRec.Body.String())
	}
	if !bytes.Contains(publicDetailRec.Body.Bytes(), []byte(`"liked_by_viewer":true`)) || !bytes.Contains(publicDetailRec.Body.Bytes(), []byte(`"favorited_by_viewer":true`)) {
		t.Fatalf("expected public detail to include viewer interaction state body=%s", publicDetailRec.Body.String())
	}

	publicImageReq := httptest.NewRequest(http.MethodGet, "/api/open/image/v1/gallery/images/"+imageID+"/image", nil)
	publicImageRec := httptest.NewRecorder()
	handler.ServeHTTP(publicImageRec, publicImageReq)
	if publicImageRec.Code != http.StatusOK {
		t.Fatalf("public image: %d body=%s", publicImageRec.Code, publicImageRec.Body.String())
	}
	if got := publicImageRec.Body.Bytes(); !bytes.Equal(got, imageBytes) {
		t.Fatalf("public image bytes mismatch: got %d bytes want %d", len(got), len(imageBytes))
	}
	if contentType := publicImageRec.Header().Get("Content-Type"); contentType != "image/png" {
		t.Fatalf("expected public image/png content type, got %q", contentType)
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

func TestGalleryPublishFallsBackAndImageDeleteRemovesPrivateAsset(t *testing.T) {
	imageBytes := tinyPNG(t)
	var generationServer *httptest.Server
	generationServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/chat/completions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[{"message":{"images":[{"image_url":{"url":"`+generationServer.URL+`/images/delete.png"}}]}}]}`)
		case "/images/delete.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(imageBytes)
		default:
			t.Fatalf("unexpected generation path %s", r.URL.Path)
		}
	}))
	defer generationServer.Close()

	cfg := taskAPIConfig(generationServer.URL)
	cfg.Providers.OpenAI.Enabled = false

	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "gallery-delete@example.com")

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       1,
		ChangePoints: "100.00000",
		Reason:       "seed balance",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}

	taskSvc := imagetaskservice.NewServiceWithStoreAssetsAndBilling(cfg, imagetaskservice.NewMemoryStore(), nil, billingSvc)
	api := handlers.NewAPIWithServices(cfg, authSvc, nil, taskSvc, adminconfigservice.NewService(cfg))
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
	if publishRec.Code != http.StatusAccepted {
		t.Fatalf("publish request should fall back to manual review: %d body=%s", publishRec.Code, publishRec.Body.String())
	}
	if !bytes.Contains(publishRec.Body.Bytes(), []byte(`"visibility_status":"pending_review"`)) {
		t.Fatalf("expected pending review body=%s", publishRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/agent/gallery/v1/images/"+imageID, nil)
	deleteReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("delete gallery image: %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent/gallery/v1/images?page=1&page_size=10", nil)
	listReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list gallery images: %d body=%s", listRec.Code, listRec.Body.String())
	}
	if bytes.Contains(listRec.Body.Bytes(), []byte(imageID)) {
		t.Fatalf("expected deleted image to be absent body=%s", listRec.Body.String())
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
