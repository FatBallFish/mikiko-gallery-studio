package router

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	stripe "github.com/stripe/stripe-go/v85"
)

func TestAdminCashierReadEndpoints(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierTest(t, handler)

	endpoints := []struct {
		name string
		path string
	}{
		{name: "overview", path: "/api/ops/admin/v1/cashier/overview"},
		{name: "plans", path: "/api/ops/admin/v1/cashier/plans?page=1&page_size=10&plan_type=points_package"},
		{name: "visible methods", path: "/api/ops/admin/v1/cashier/visible-methods"},
		{name: "provider instances", path: "/api/ops/admin/v1/cashier/provider-instances?page=1&page_size=10"},
		{name: "orders", path: "/api/ops/admin/v1/cashier/orders?page=1&page_size=10"},
		{name: "webhook events", path: "/api/ops/admin/v1/cashier/webhook-events?page=1&page_size=10"},
	}
	for _, endpoint := range endpoints {
		t.Run(endpoint.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, endpoint.path, nil)
			req.Header.Set("Authorization", "Bearer "+adminToken)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected %s 200, got %d body=%s", endpoint.name, rec.Code, rec.Body.String())
			}
		})
	}

	overviewReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/overview", nil)
	overviewReq.Header.Set("Authorization", "Bearer "+adminToken)
	overviewRec := httptest.NewRecorder()
	handler.ServeHTTP(overviewRec, overviewReq)
	var overviewResp struct {
		Data struct {
			MockEnabled bool     `json:"mock_enabled"`
			Methods     []string `json:"enabled_methods"`
		} `json:"data"`
	}
	if err := json.NewDecoder(overviewRec.Body).Decode(&overviewResp); err != nil {
		t.Fatalf("decode overview: %v", err)
	}
	if !overviewResp.Data.MockEnabled || len(overviewResp.Data.Methods) != 1 || overviewResp.Data.Methods[0] != "mock" {
		t.Fatalf("unexpected overview %#v", overviewResp.Data)
	}

	createOverviewProvider := func(name string, enabled bool, sortOrder int) {
		body := fmt.Sprintf(`{"provider_type":"alipay_direct","name":%q,"enabled":%t,"supported_methods":["alipay"],"sort_order":%d,"scheduler_weight":100,"config":{"app_id":%q,"payment_url":"https://pay.example.com/session"}}`, name, enabled, sortOrder, name)
		createProviderInstanceForCashierTest(t, handler, adminToken, body)
	}
	createOverviewProvider("overview-alipay-a", true, 10)
	createOverviewProvider("overview-alipay-b", true, 20)
	createOverviewProvider("overview-alipay-disabled", false, 30)
	providerOverviewReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/overview", nil)
	providerOverviewReq.Header.Set("Authorization", "Bearer "+adminToken)
	providerOverviewRec := httptest.NewRecorder()
	handler.ServeHTTP(providerOverviewRec, providerOverviewReq)
	if providerOverviewRec.Code != http.StatusOK {
		t.Fatalf("expected provider overview 200, got %d body=%s", providerOverviewRec.Code, providerOverviewRec.Body.String())
	}
	var providerOverviewResp struct {
		Data struct {
			EnabledProviderInstances int `json:"enabled_provider_instances"`
		} `json:"data"`
	}
	if err := json.NewDecoder(providerOverviewRec.Body).Decode(&providerOverviewResp); err != nil {
		t.Fatalf("decode provider overview: %v", err)
	}
	if providerOverviewResp.Data.EnabledProviderInstances != 3 {
		t.Fatalf("expected overview to count enabled provider instances, got %#v", providerOverviewResp.Data)
	}

	userSession := loginExistingAuthUser(t, authSvc, "cashier-user@example.com")
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	secondCreateReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	secondCreateReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	secondCreateReq.Header.Set("Content-Type", "application/json")
	secondCreateRec := httptest.NewRecorder()
	handler.ServeHTTP(secondCreateRec, secondCreateReq)
	if secondCreateRec.Code != http.StatusCreated {
		t.Fatalf("expected second user cashier order create 201, got %d body=%s", secondCreateRec.Code, secondCreateRec.Body.String())
	}

	ordersReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/orders?page=1&page_size=10", nil)
	ordersReq.Header.Set("Authorization", "Bearer "+adminToken)
	ordersRec := httptest.NewRecorder()
	handler.ServeHTTP(ordersRec, ordersReq)
	if ordersRec.Code != http.StatusOK {
		t.Fatalf("expected admin cashier orders 200, got %d body=%s", ordersRec.Code, ordersRec.Body.String())
	}
	var ordersResp struct {
		Data struct {
			Items []struct {
				OrderNo string `json:"order_no"`
				Status  string `json:"status"`
			} `json:"items"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.NewDecoder(ordersRec.Body).Decode(&ordersResp); err != nil {
		t.Fatalf("decode admin orders: %v", err)
	}
	if ordersResp.Data.Pagination.Total != 2 || len(ordersResp.Data.Items) != 2 || ordersResp.Data.Items[0].Status != "pending" || ordersResp.Data.Items[0].OrderNo == "" {
		t.Fatalf("expected admin cashier orders to include user order, got %#v", ordersResp.Data)
	}

	var createResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(strings.NewReader(createRec.Body.String())).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	mockPayReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(createResp.Data.ID)+"/mock-pay", nil)
	mockPayReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	mockPayRec := httptest.NewRecorder()
	handler.ServeHTTP(mockPayRec, mockPayReq)
	if mockPayRec.Code != http.StatusOK {
		t.Fatalf("expected mock pay 200, got %d body=%s", mockPayRec.Code, mockPayRec.Body.String())
	}

	paidOverviewReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/overview", nil)
	paidOverviewReq.Header.Set("Authorization", "Bearer "+adminToken)
	paidOverviewRec := httptest.NewRecorder()
	handler.ServeHTTP(paidOverviewRec, paidOverviewReq)
	if paidOverviewRec.Code != http.StatusOK {
		t.Fatalf("expected paid overview 200, got %d body=%s", paidOverviewRec.Code, paidOverviewRec.Body.String())
	}
	var paidOverviewResp struct {
		Data struct {
			TodayOrderCount     int    `json:"today_order_count"`
			TodayCompletedCount int    `json:"today_completed_count"`
			TodayAmountCNY      string `json:"today_amount_cny"`
			SuccessRate         string `json:"success_rate"`
			PendingCount        int    `json:"pending_count"`
			FailedWebhookCount  int    `json:"failed_webhook_count"`
		} `json:"data"`
	}
	if err := json.NewDecoder(paidOverviewRec.Body).Decode(&paidOverviewResp); err != nil {
		t.Fatalf("decode paid overview: %v", err)
	}
	if paidOverviewResp.Data.TodayOrderCount != 2 || paidOverviewResp.Data.TodayCompletedCount != 1 || paidOverviewResp.Data.PendingCount != 1 || paidOverviewResp.Data.TodayAmountCNY != "19.90000" || paidOverviewResp.Data.SuccessRate != "50.00%" || paidOverviewResp.Data.FailedWebhookCount != 0 {
		t.Fatalf("unexpected paid overview %#v", paidOverviewResp.Data)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/webhook-events?page=1&page_size=10", nil)
	eventsReq.Header.Set("Authorization", "Bearer "+adminToken)
	eventsRec := httptest.NewRecorder()
	handler.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected admin cashier events 200, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	var eventsResp struct {
		Data struct {
			Items []struct {
				OrderNo      string `json:"order_no"`
				ProviderType string `json:"provider_type"`
				Status       string `json:"status"`
			} `json:"items"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.NewDecoder(eventsRec.Body).Decode(&eventsResp); err != nil {
		t.Fatalf("decode admin events: %v", err)
	}
	if eventsResp.Data.Pagination.Total != 1 || len(eventsResp.Data.Items) != 1 || eventsResp.Data.Items[0].Status != "processed" || eventsResp.Data.Items[0].ProviderType != "mock" || eventsResp.Data.Items[0].OrderNo == "" {
		t.Fatalf("expected admin cashier events to include processed mock event, got %#v", eventsResp.Data)
	}

	orderDetailReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID), nil)
	orderDetailReq.Header.Set("Authorization", "Bearer "+adminToken)
	orderDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(orderDetailRec, orderDetailReq)
	if orderDetailRec.Code != http.StatusOK {
		t.Fatalf("expected admin cashier order detail 200, got %d body=%s", orderDetailRec.Code, orderDetailRec.Body.String())
	}
	var orderDetailResp struct {
		Data struct {
			ID      int64  `json:"id"`
			OrderNo string `json:"order_no"`
			Status  string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(orderDetailRec.Body).Decode(&orderDetailResp); err != nil {
		t.Fatalf("decode order detail: %v", err)
	}
	if orderDetailResp.Data.ID != createResp.Data.ID || orderDetailResp.Data.OrderNo == "" || orderDetailResp.Data.Status != "completed" {
		t.Fatalf("unexpected order detail %#v", orderDetailResp.Data)
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/webhook-events/1/retry", nil)
	retryReq.Header.Set("Authorization", "Bearer "+adminToken)
	retryRec := httptest.NewRecorder()
	handler.ServeHTTP(retryRec, retryReq)
	if retryRec.Code != http.StatusOK {
		t.Fatalf("expected admin cashier webhook retry 200, got %d body=%s", retryRec.Code, retryRec.Body.String())
	}
	var retryResp struct {
		Data struct {
			ID           int64  `json:"id"`
			OrderNo      string `json:"order_no"`
			ProviderType string `json:"provider_type"`
			Status       string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(retryRec.Body).Decode(&retryResp); err != nil {
		t.Fatalf("decode retry response: %v", err)
	}
	if retryResp.Data.ID != 1 || retryResp.Data.Status != "processed" || retryResp.Data.ProviderType != "mock" || retryResp.Data.OrderNo == "" {
		t.Fatalf("unexpected retry response %#v", retryResp.Data)
	}
}

func TestAdminCashierOrdersSupportsOperationalFilters(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil))
	adminToken := loginAdminForCashierTest(t, handler)
	if err := authSvc.SendEmailCode("cashier-filter-a@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode userA: %v", err)
	}
	userA, userASession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-filter-a@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode userA: %v", err)
	}
	if err := authSvc.SendEmailCode("cashier-filter-b@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode userB: %v", err)
	}
	_, userBSession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-filter-b@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode userB: %v", err)
	}

	createOrder := func(token string, body string) domainbilling.PaymentOrder {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("expected cashier order create 201, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Data domainbilling.PaymentOrder `json:"data"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode created order: %v", err)
		}
		return resp.Data
	}

	completed := createOrder(userASession.AccessToken, `{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`)
	mockPayReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(completed.ID)+"/mock-pay", nil)
	mockPayReq.Header.Set("Authorization", "Bearer "+userASession.AccessToken)
	mockPayRec := httptest.NewRecorder()
	handler.ServeHTTP(mockPayRec, mockPayReq)
	if mockPayRec.Code != http.StatusOK {
		t.Fatalf("expected mock pay 200, got %d body=%s", mockPayRec.Code, mockPayRec.Body.String())
	}
	pending := createOrder(userBSession.AccessToken, `{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}`)

	assertFilter := func(query string, wantOrderNo string) {
		req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/orders?page=1&page_size=10&"+query, nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected admin cashier orders filter 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Data struct {
				Items      []domainbilling.PaymentOrder `json:"items"`
				Pagination struct {
					Total int `json:"total"`
				} `json:"pagination"`
			} `json:"data"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode filtered orders: %v", err)
		}
		if resp.Data.Pagination.Total != 1 || len(resp.Data.Items) != 1 || resp.Data.Items[0].OrderNo != wantOrderNo {
			t.Fatalf("expected only order %s for query %s, got %#v", wantOrderNo, query, resp.Data)
		}
	}

	assertFilter("status=completed", completed.OrderNo)
	assertFilter("order_no="+url.QueryEscape(pending.OrderNo[:len(pending.OrderNo)-2]), pending.OrderNo)
	assertFilter("purchase_type=custom_amount&visible_method=mock", pending.OrderNo)
	assertFilter("user_id="+jsonInt64(userA.ID), completed.OrderNo)
}

func TestAdminCashierOrderCompleteManuallyCreditsRechargeBalance(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-manual-complete-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, userSession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-manual-complete-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierTest(t, handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID      int64  `json:"id"`
			UserID  int64  `json:"user_id"`
			OrderNo string `json:"order_no"`
			Status  string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	if createResp.Data.Status != "pending" || createResp.Data.UserID != user.ID {
		t.Fatalf("expected pending order for user, got %#v", createResp.Data)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/complete", bytes.NewBufferString(`{"provider":"manual_alipay","trade_no":"MANUAL-TRADE-001","reason":"confirmed in provider console"}`))
	completeReq.Header.Set("Authorization", "Bearer "+adminToken)
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()
	handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("expected admin manual complete 200, got %d body=%s", completeRec.Code, completeRec.Body.String())
	}
	var completeResp struct {
		Data struct {
			ID        int64  `json:"id"`
			Status    string `json:"status"`
			Provider  string `json:"provider"`
			TradeNo   string `json:"trade_no"`
			LedgerID  int64  `json:"ledger_id"`
			Completed string `json:"completed_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(completeRec.Body).Decode(&completeResp); err != nil {
		t.Fatalf("decode completed order: %v", err)
	}
	if completeResp.Data.ID != createResp.Data.ID || completeResp.Data.Status != "completed" || completeResp.Data.Provider != "manual_alipay" || completeResp.Data.TradeNo != "MANUAL-TRADE-001" || completeResp.Data.LedgerID == 0 || completeResp.Data.Completed == "" {
		t.Fatalf("unexpected completed order %#v", completeResp.Data)
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.RechargePoints != "100.00000" || balance.AvailablePoints != "100.00000" {
		t.Fatalf("expected manual complete to credit recharge balance, got %#v", balance)
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/complete", bytes.NewBufferString(`{"provider":"manual_alipay","trade_no":"MANUAL-TRADE-001","reason":"confirmed in provider console"}`))
	replayReq.Header.Set("Authorization", "Bearer "+adminToken)
	replayReq.Header.Set("Content-Type", "application/json")
	replayRec := httptest.NewRecorder()
	handler.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("expected idempotent admin manual complete 200, got %d body=%s", replayRec.Code, replayRec.Body.String())
	}
	balanceAfterReplay, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after replay: %v", err)
	}
	if balanceAfterReplay.RechargePoints != "100.00000" || balanceAfterReplay.AvailablePoints != "100.00000" {
		t.Fatalf("expected replay not to double credit, got %#v", balanceAfterReplay)
	}
}

func TestAdminCashierOrderCloseCancelsPendingOrderAndBlocksCompletion(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-close-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-close-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierTest(t, handler)

	orderID, _ := createCustomCashierOrderForTest(t, handler, session.AccessToken, "mock", "12.50000")
	closeReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/close", bytes.NewBufferString(`{"reason":"duplicate pending order"}`))
	closeReq.Header.Set("Authorization", "Bearer "+adminToken)
	closeReq.Header.Set("Content-Type", "application/json")
	closeRec := httptest.NewRecorder()
	handler.ServeHTTP(closeRec, closeReq)
	if closeRec.Code != http.StatusOK {
		t.Fatalf("expected admin close 200, got %d body=%s", closeRec.Code, closeRec.Body.String())
	}
	var closeResp struct {
		Data struct {
			ID       int64  `json:"id"`
			Status   string `json:"status"`
			ClosedAt string `json:"closed_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(closeRec.Body).Decode(&closeResp); err != nil {
		t.Fatalf("decode close response: %v", err)
	}
	if closeResp.Data.ID != orderID || closeResp.Data.Status != "canceled" || closeResp.Data.ClosedAt == "" {
		t.Fatalf("unexpected closed order response %#v", closeResp.Data)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/complete", bytes.NewBufferString(`{"trade_no":"MANUAL-TRADE-CLOSED-001","reason":"should not complete closed order"}`))
	completeReq.Header.Set("Authorization", "Bearer "+adminToken)
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()
	handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusConflict {
		t.Fatalf("expected closed order completion 409, got %d body=%s", completeRec.Code, completeRec.Body.String())
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "0.00000" || balance.RechargePoints != "0.00000" {
		t.Fatalf("expected closing pending order not to credit balance, got %#v", balance)
	}
}

func TestAdminCashierOrderRefundDeductsUnusedRechargeBalance(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-refund-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, userSession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-refund-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierTest(t, handler)

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}

	completeReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/complete", bytes.NewBufferString(`{"provider":"manual_alipay","trade_no":"MANUAL-TRADE-REFUND-001","reason":"confirmed before refund test"}`))
	completeReq.Header.Set("Authorization", "Bearer "+adminToken)
	completeReq.Header.Set("Content-Type", "application/json")
	completeRec := httptest.NewRecorder()
	handler.ServeHTTP(completeRec, completeReq)
	if completeRec.Code != http.StatusOK {
		t.Fatalf("expected admin manual complete 200, got %d body=%s", completeRec.Code, completeRec.Body.String())
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.RechargePoints != "100.00000" {
		t.Fatalf("expected completed order to credit recharge balance, got %#v", balance)
	}

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-TRADE-001","reason":"customer requested refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusOK {
		t.Fatalf("expected admin refund 200, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	var refundResp struct {
		Data struct {
			ID            int64  `json:"id"`
			Status        string `json:"status"`
			RefundTradeNo string `json:"refund_trade_no"`
			RefundedAt    string `json:"refunded_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(refundRec.Body).Decode(&refundResp); err != nil {
		t.Fatalf("decode refund response: %v", err)
	}
	if refundResp.Data.ID != createResp.Data.ID || refundResp.Data.Status != "refunded" || refundResp.Data.RefundTradeNo != "REFUND-TRADE-001" || refundResp.Data.RefundedAt == "" {
		t.Fatalf("unexpected refund response %#v", refundResp.Data)
	}
	balanceAfterRefund, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after refund: %v", err)
	}
	if balanceAfterRefund.RechargePoints != "0.00000" || balanceAfterRefund.AvailablePoints != "0.00000" {
		t.Fatalf("expected refund to deduct unused recharge balance, got %#v", balanceAfterRefund)
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-TRADE-001","reason":"customer requested refund"}`))
	replayReq.Header.Set("Authorization", "Bearer "+adminToken)
	replayReq.Header.Set("Content-Type", "application/json")
	replayRec := httptest.NewRecorder()
	handler.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("expected idempotent admin refund 200, got %d body=%s", replayRec.Code, replayRec.Body.String())
	}
	balanceAfterReplay, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after refund replay: %v", err)
	}
	if balanceAfterReplay.RechargePoints != "0.00000" || balanceAfterReplay.AvailablePoints != "0.00000" {
		t.Fatalf("expected refund replay not to double deduct, got %#v", balanceAfterReplay)
	}
}

func TestAdminCashierOrderChargebackDeductsBalanceAndIsIdempotent(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-chargeback-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-chargeback-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil, nil, nil, nil, nil))
	adminToken := loginAdminForCashierTest(t, handler)

	orderID, _ := createCustomCashierOrderForTest(t, handler, session.AccessToken, "mock", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "CHARGEBACK-TRADE-001")

	chargebackReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/chargeback", bytes.NewBufferString(`{"charge_points":"5.00000","reason":"provider chargeback confirmed"}`))
	chargebackReq.Header.Set("Authorization", "Bearer "+adminToken)
	chargebackReq.Header.Set("Content-Type", "application/json")
	chargebackReq.Header.Set("Idempotency-Key", "cashier-chargeback-once")
	chargebackRec := httptest.NewRecorder()
	handler.ServeHTTP(chargebackRec, chargebackReq)
	if chargebackRec.Code != http.StatusOK {
		t.Fatalf("expected cashier chargeback 200, got %d body=%s", chargebackRec.Code, chargebackRec.Body.String())
	}
	var chargebackResp struct {
		Data struct {
			Order struct {
				ID                       int64  `json:"id"`
				OrderNo                  string `json:"order_no"`
				ChargebackPoints         string `json:"chargeback_points"`
				ChargebackReason         string `json:"chargeback_reason"`
				ChargebackIdempotencyKey string `json:"chargeback_idempotency_key"`
				ChargebackAt             string `json:"chargeback_at"`
			} `json:"order"`
			Balance struct {
				AvailablePoints string `json:"available_points"`
				RechargePoints  string `json:"recharge_points"`
			} `json:"balance"`
		} `json:"data"`
	}
	if err := json.NewDecoder(chargebackRec.Body).Decode(&chargebackResp); err != nil {
		t.Fatalf("decode chargeback response: %v body=%s", err, chargebackRec.Body.String())
	}
	if chargebackResp.Data.Order.ID != orderID || chargebackResp.Data.Order.OrderNo == "" || chargebackResp.Data.Balance.AvailablePoints != "35.00000" || chargebackResp.Data.Balance.RechargePoints != "35.00000" {
		t.Fatalf("unexpected chargeback response %#v", chargebackResp.Data)
	}
	if chargebackResp.Data.Order.ChargebackPoints != "5.00000" || chargebackResp.Data.Order.ChargebackReason != "provider chargeback confirmed" || chargebackResp.Data.Order.ChargebackIdempotencyKey != "cashier-chargeback-once" || chargebackResp.Data.Order.ChargebackAt == "" {
		t.Fatalf("expected chargeback response to include dispute summary, got %#v", chargebackResp.Data.Order)
	}
	detailReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID), nil)
	detailReq.Header.Set("Authorization", "Bearer "+adminToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected cashier order detail 200 after chargeback, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data struct {
			ChargebackPoints string `json:"chargeback_points"`
			ChargebackReason string `json:"chargeback_reason"`
			ChargebackAt     string `json:"chargeback_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode order detail response: %v body=%s", err, detailRec.Body.String())
	}
	if detailResp.Data.ChargebackPoints != "5.00000" || detailResp.Data.ChargebackReason != "provider chargeback confirmed" || detailResp.Data.ChargebackAt == "" {
		t.Fatalf("expected order detail to persist chargeback summary, got %#v", detailResp.Data)
	}
	balanceAfterChargeback, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after chargeback: %v", err)
	}
	if balanceAfterChargeback.AvailablePoints != "35.00000" || balanceAfterChargeback.RechargePoints != "35.00000" {
		t.Fatalf("expected chargeback to deduct points once, got %#v", balanceAfterChargeback)
	}

	replayReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/chargeback", bytes.NewBufferString(`{"charge_points":"5.00000","reason":"provider chargeback confirmed"}`))
	replayReq.Header.Set("Authorization", "Bearer "+adminToken)
	replayReq.Header.Set("Content-Type", "application/json")
	replayReq.Header.Set("Idempotency-Key", "cashier-chargeback-once")
	replayRec := httptest.NewRecorder()
	handler.ServeHTTP(replayRec, replayReq)
	if replayRec.Code != http.StatusOK {
		t.Fatalf("expected idempotent cashier chargeback replay 200, got %d body=%s", replayRec.Code, replayRec.Body.String())
	}
	balanceAfterReplay, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after chargeback replay: %v", err)
	}
	if balanceAfterReplay.AvailablePoints != "35.00000" || balanceAfterReplay.RechargePoints != "35.00000" {
		t.Fatalf("expected chargeback replay not to double deduct, got %#v", balanceAfterReplay)
	}

	missingKeyReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/chargeback", bytes.NewBufferString(`{"charge_points":"1.00000","reason":"missing key"}`))
	missingKeyReq.Header.Set("Authorization", "Bearer "+adminToken)
	missingKeyReq.Header.Set("Content-Type", "application/json")
	missingKeyRec := httptest.NewRecorder()
	handler.ServeHTTP(missingKeyRec, missingKeyReq)
	if missingKeyRec.Code != http.StatusBadRequest {
		t.Fatalf("expected missing idempotency key 400, got %d body=%s", missingKeyRec.Code, missingKeyRec.Body.String())
	}
}

func TestAdminCashierOrderRefundCallsEasyPayProviderBeforeLocalDeduct(t *testing.T) {
	var upstreamPath string
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("expected easypay refund POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse easypay refund form: %v", err)
		}
		upstreamValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"success","refund_no":"EASYPAY-REFUND-001"}`))
	}))
	defer upstream.Close()

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-refund-easypay-user@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, fmt.Sprintf(`{"provider_type":"easypay_alipay","name":"易支付退款账号","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"pid":"10001","key":"merchant-secret","payment_mode":"popup"}}`, upstream.URL))
	orderID, orderNo := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "alipay", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "EASYPAY-TRADE-001")

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-EASYPAY-001","reason":"customer requested refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusOK {
		t.Fatalf("expected admin easypay refund 200, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if upstreamPath != "/api.php" || upstreamValues.Get("act") != "refund" || upstreamValues.Get("pid") != "10001" || upstreamValues.Get("key") != "merchant-secret" || upstreamValues.Get("money") != "12.50000" || upstreamValues.Get("out_trade_no") != orderNo {
		t.Fatalf("unexpected easypay refund request path=%q values=%#v order_no=%s", upstreamPath, upstreamValues, orderNo)
	}
	assertAdminCashierRefundedForTest(t, refundRec.Body.String(), orderID, "REFUND-EASYPAY-001")
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after easypay refund: %v", err)
	}
	if balance.RechargePoints != "0.00000" || balance.AvailablePoints != "0.00000" {
		t.Fatalf("expected easypay refund to deduct local recharge balance, got %#v", balance)
	}
}

func TestAdminCashierOrderRefundSupportsPartialEasyPayRefund(t *testing.T) {
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected easypay refund POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse easypay refund form: %v", err)
		}
		upstreamValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"success","refund_no":"EASYPAY-REFUND-PARTIAL-001"}`))
	}))
	defer upstream.Close()

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-refund-easypay-partial-user@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, fmt.Sprintf(`{"provider_type":"easypay_alipay","name":"易支付部分退款账号","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"pid":"10001","key":"merchant-secret","payment_mode":"popup"}}`, upstream.URL))
	orderID, orderNo := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "alipay", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "EASYPAY-TRADE-PARTIAL-001")

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-EASYPAY-PARTIAL-001","refund_amount_cny":"5.00000","reason":"partial refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusOK {
		t.Fatalf("expected admin easypay partial refund 200, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if upstreamValues.Get("act") != "refund" || upstreamValues.Get("money") != "5.00000" || upstreamValues.Get("out_trade_no") != orderNo {
		t.Fatalf("unexpected easypay partial refund request values=%#v order_no=%s", upstreamValues, orderNo)
	}
	var refundResp struct {
		Data struct {
			ID                int64  `json:"id"`
			Status            string `json:"status"`
			RefundTradeNo     string `json:"refund_trade_no"`
			RefundedAmountCNY string `json:"refunded_amount_cny"`
			RefundedPoints    string `json:"refunded_points"`
			RefundedAt        string `json:"refunded_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(refundRec.Body).Decode(&refundResp); err != nil {
		t.Fatalf("decode partial refund response: %v body=%s", err, refundRec.Body.String())
	}
	if refundResp.Data.ID != orderID || refundResp.Data.Status != "partially_refunded" || refundResp.Data.RefundTradeNo != "REFUND-EASYPAY-PARTIAL-001" || refundResp.Data.RefundedAmountCNY != "5.00000" || refundResp.Data.RefundedPoints != "16.00000" || refundResp.Data.RefundedAt != "" {
		t.Fatalf("unexpected partial refund response %#v body=%s", refundResp.Data, refundRec.Body.String())
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after easypay partial refund: %v", err)
	}
	if balance.RechargePoints != "24.00000" || balance.AvailablePoints != "24.00000" {
		t.Fatalf("expected partial refund to deduct only refunded recharge balance, got %#v", balance)
	}
}

func TestAdminCashierOrderRefundDoesNotDeductWhenEasyPayProviderFails(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"refund failed"}`))
	}))
	defer upstream.Close()

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-refund-easypay-fail-user@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, fmt.Sprintf(`{"provider_type":"easypay_alipay","name":"易支付退款失败账号","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"pid":"10001","key":"merchant-secret","payment_mode":"popup"}}`, upstream.URL))
	orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "alipay", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "EASYPAY-TRADE-FAIL-001")

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-EASYPAY-FAIL-001","reason":"customer requested refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusBadGateway {
		t.Fatalf("expected failed easypay refund 502, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if !bytes.Contains(refundRec.Body.Bytes(), []byte(`PAYMENT_PROVIDER_UNAVAILABLE`)) {
		t.Fatalf("expected PAYMENT_PROVIDER_UNAVAILABLE body=%s", refundRec.Body.String())
	}
	detailReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID), nil)
	detailReq.Header.Set("Authorization", "Bearer "+adminToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK || !bytes.Contains(detailRec.Body.Bytes(), []byte(`"status":"completed"`)) {
		t.Fatalf("expected failed provider refund to leave order completed, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after failed easypay refund: %v", err)
	}
	if balance.RechargePoints != "40.00000" || balance.AvailablePoints != "40.00000" {
		t.Fatalf("expected failed easypay refund not to deduct balance, got %#v", balance)
	}
}

func TestAdminCashierOrderRefundPrecheckSkipsProviderWhenRechargeBalanceConsumed(t *testing.T) {
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"success","refund_no":"EASYPAY-REFUND-SHOULD-NOT-HAPPEN"}`))
	}))
	defer upstream.Close()

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-refund-precheck-user@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, fmt.Sprintf(`{"provider_type":"easypay_alipay","name":"易支付退款预检账号","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"pid":"10001","key":"merchant-secret","payment_mode":"popup"}}`, upstream.URL))
	orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "alipay", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "EASYPAY-TRADE-PRECHECK-001")

	if _, err := billingSvc.ReserveTask(t.Context(), domainbilling.ReserveRequest{UserID: user.ID, TaskID: "precheck-consume-task", EstimatedPoints: "5.00000", Reason: "consume recharge before refund"}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	if _, err := billingSvc.FinalizeTask(t.Context(), domainbilling.FinalizeRequest{UserID: user.ID, TaskID: "precheck-consume-task", EstimatedPoints: "5.00000", ActualPoints: "5.00000", Reason: "consume recharge before refund"}); err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-PRECHECK-001","reason":"customer requested refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusConflict {
		t.Fatalf("expected consumed balance refund precheck 409, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if upstreamCalls != 0 {
		t.Fatalf("expected refund precheck to skip provider call, got %d upstream calls", upstreamCalls)
	}
	if !bytes.Contains(refundRec.Body.Bytes(), []byte(`payment order recharge balance is insufficient for refund`)) {
		t.Fatalf("expected insufficient recharge refund error, body=%s", refundRec.Body.String())
	}
}

func TestAdminCashierOrderRefundFreezesRechargeGrantBeforeProviderCall(t *testing.T) {
	var reserveErr error
	var userID int64
	var upstreamCalls int
	var billingSvc *billingservice.Service
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		_, reserveErr = billingSvc.ReserveTask(r.Context(), domainbilling.ReserveRequest{
			UserID:          userID,
			TaskID:          "refund-freeze-concurrent-task",
			EstimatedPoints: "5.00000",
			Reason:          "concurrent consume during provider refund",
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"success","refund_no":"EASYPAY-REFUND-FREEZE-001"}`))
	}))
	defer upstream.Close()

	handler, adminToken, user, userSession, svc := newAdminCashierRefundProviderTest(t, "cashier-refund-freeze-user@example.com")
	userID = user.ID
	billingSvc = svc
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, fmt.Sprintf(`{"provider_type":"easypay_alipay","name":"易支付退款冻结账号","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"pid":"10001","key":"merchant-secret","payment_mode":"popup"}}`, upstream.URL))
	orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "alipay", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "EASYPAY-TRADE-FREEZE-001")

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-FREEZE-001","reason":"customer requested refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusOK {
		t.Fatalf("expected refund with frozen grant 200, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected one upstream refund call, got %d", upstreamCalls)
	}
	if reserveErr == nil {
		t.Fatalf("expected concurrent reserve during refund provider call to fail while recharge grant is frozen")
	}
	if !strings.Contains(reserveErr.Error(), "insufficient points") {
		t.Fatalf("expected concurrent reserve to fail with insufficient points, got %v", reserveErr)
	}
}

func TestAdminCashierOrderRefundRecordsCompensationWhenLocalFinalizeFailsAfterProviderSuccess(t *testing.T) {
	var userID int64
	var billingSvc *billingservice.Service
	var orderID int64
	var upstreamCalls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls++
		if _, err := billingSvc.ReleaseRefundPaymentOrder(r.Context(), domainbilling.RefundPaymentOrderRequest{
			UserID:        userID,
			OrderID:       orderID,
			RefundTradeNo: "REFUND-COMPENSATE-001",
			Reason:        "simulate local finalize failure after provider accepted refund",
		}); err != nil {
			t.Fatalf("ReleaseRefundPaymentOrder in upstream hook: %v", err)
		}
		if _, err := billingSvc.ReserveTask(r.Context(), domainbilling.ReserveRequest{
			UserID:          userID,
			TaskID:          "refund-compensate-consume-task",
			EstimatedPoints: "5.00000",
			Reason:          "consume recharge after provider refund accepted",
		}); err != nil {
			t.Fatalf("ReserveTask in upstream hook: %v", err)
		}
		if _, err := billingSvc.FinalizeTask(r.Context(), domainbilling.FinalizeRequest{
			UserID:          userID,
			TaskID:          "refund-compensate-consume-task",
			EstimatedPoints: "5.00000",
			ActualPoints:    "5.00000",
			Reason:          "consume recharge after provider refund accepted",
		}); err != nil {
			t.Fatalf("FinalizeTask in upstream hook: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"success","refund_no":"EASYPAY-REFUND-COMPENSATE-001"}`))
	}))
	defer upstream.Close()

	handler, adminToken, user, userSession, svc := newAdminCashierRefundProviderTest(t, "cashier-refund-compensate-user@example.com")
	userID = user.ID
	billingSvc = svc
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, fmt.Sprintf(`{"provider_type":"easypay_alipay","name":"易支付退款补偿账号","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"pid":"10001","key":"merchant-secret","payment_mode":"popup"}}`, upstream.URL))
	orderID, _ = createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "alipay", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "EASYPAY-TRADE-COMPENSATE-001")

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-COMPENSATE-001","reason":"customer requested refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusConflict {
		t.Fatalf("expected local finalize failure 409 after provider success, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if upstreamCalls != 1 {
		t.Fatalf("expected one upstream refund call, got %d", upstreamCalls)
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/webhook-events?page=1&page_size=10", nil)
	eventsReq.Header.Set("Authorization", "Bearer "+adminToken)
	eventsRec := httptest.NewRecorder()
	handler.ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected events list 200, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	var eventsResp struct {
		Data struct {
			Items []struct {
				ID              int64  `json:"id"`
				OrderID         int64  `json:"order_id"`
				Status          string `json:"status"`
				EventType       string `json:"event_type"`
				FailureReason   string `json:"failure_reason"`
				SignatureStatus string `json:"signature_status"`
				ResultSummary   string `json:"result_summary"`
				PayloadPreview  string `json:"payload_preview"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(eventsRec.Body).Decode(&eventsResp); err != nil {
		t.Fatalf("decode events: %v", err)
	}
	if len(eventsResp.Data.Items) == 0 || eventsResp.Data.Items[0].OrderID != orderID || eventsResp.Data.Items[0].Status != "failed" || eventsResp.Data.Items[0].EventType != "refund.local_finalize_failed" || !strings.Contains(eventsResp.Data.Items[0].FailureReason, "payment order recharge balance is insufficient for refund") {
		t.Fatalf("expected failed refund compensation event first, got %#v", eventsResp.Data.Items)
	}
	if eventsResp.Data.Items[0].SignatureStatus != "failed" || !strings.Contains(eventsResp.Data.Items[0].ResultSummary, "处理失败") || !strings.Contains(eventsResp.Data.Items[0].PayloadPreview, "refund_trade_no") {
		t.Fatalf("expected webhook troubleshooting fields, got %#v", eventsResp.Data.Items[0])
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/webhook-events/"+jsonInt64(eventsResp.Data.Items[0].ID)+"/retry", nil)
	retryReq.Header.Set("Authorization", "Bearer "+adminToken)
	retryRec := httptest.NewRecorder()
	handler.ServeHTTP(retryRec, retryReq)
	if retryRec.Code != http.StatusConflict {
		t.Fatalf("expected retry to still surface local refund conflict while balance is consumed, got %d body=%s", retryRec.Code, retryRec.Body.String())
	}
}

func TestAdminDashboardEscalatesRefundCompensationFailures(t *testing.T) {
	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-refund-dashboard-alert-user@example.com")
	orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "mock", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "MOCK-TRADE-REFUND-ALERT-001")
	if _, err := billingSvc.RecordRefundFinalizeFailure(t.Context(), billingservice.RefundFinalizeFailureRequest{
		RefundPaymentOrderRequest: domainbilling.RefundPaymentOrderRequest{
			UserID:          user.ID,
			OrderID:         orderID,
			RefundTradeNo:   "REFUND-DASHBOARD-ALERT-001",
			Reason:          "local finalize failed",
			OperatorAdminID: 1,
		},
		FailureReason: "payment order recharge balance is insufficient for refund",
	}); err != nil {
		t.Fatalf("RecordRefundFinalizeFailure: %v", err)
	}

	dashboardReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/metrics/dashboard", nil)
	dashboardReq.Header.Set("Authorization", "Bearer "+adminToken)
	dashboardRec := httptest.NewRecorder()
	handler.ServeHTTP(dashboardRec, dashboardReq)
	if dashboardRec.Code != http.StatusOK {
		t.Fatalf("expected dashboard 200, got %d body=%s", dashboardRec.Code, dashboardRec.Body.String())
	}
	var dashboardResp struct {
		Data struct {
			Operations struct {
				FailedWebhookCount               int     `json:"failed_webhook_count"`
				RefundCompensationFailedCount    int     `json:"refund_compensation_failed_count"`
				RefundCompensationOldestFailedAt *string `json:"refund_compensation_oldest_failed_at"`
			} `json:"operations"`
			Metrics []struct {
				Key   string `json:"key"`
				Tone  string `json:"tone"`
				Trend string `json:"trend"`
			} `json:"metrics"`
		} `json:"data"`
	}
	if err := json.NewDecoder(dashboardRec.Body).Decode(&dashboardResp); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if dashboardResp.Data.Operations.FailedWebhookCount != 1 || dashboardResp.Data.Operations.RefundCompensationFailedCount != 1 || dashboardResp.Data.Operations.RefundCompensationOldestFailedAt == nil {
		t.Fatalf("expected refund compensation failure summary in dashboard, got %#v body=%s", dashboardResp.Data.Operations, dashboardRec.Body.String())
	}
	metricFound := false
	for _, metric := range dashboardResp.Data.Metrics {
		if metric.Key == "refund_compensation_failures" {
			metricFound = true
			if metric.Tone != "danger" || !strings.Contains(metric.Trend, "需处理") {
				t.Fatalf("expected danger refund compensation metric, got %#v", metric)
			}
		}
	}
	if !metricFound {
		t.Fatalf("expected refund_compensation_failures metric body=%s", dashboardRec.Body.String())
	}
}

func TestAdminReadinessEscalatesRefundCompensationFailures(t *testing.T) {
	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-refund-readiness-alert-user@example.com")
	orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "mock", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "MOCK-TRADE-REFUND-READINESS-001")
	if _, err := billingSvc.RecordRefundFinalizeFailure(t.Context(), billingservice.RefundFinalizeFailureRequest{
		RefundPaymentOrderRequest: domainbilling.RefundPaymentOrderRequest{
			UserID:          user.ID,
			OrderID:         orderID,
			RefundTradeNo:   "REFUND-READINESS-ALERT-001",
			Reason:          "local finalize failed",
			OperatorAdminID: 1,
		},
		FailureReason: "payment order recharge balance is insufficient for refund",
	}); err != nil {
		t.Fatalf("RecordRefundFinalizeFailure: %v", err)
	}

	readinessReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/readiness", nil)
	readinessReq.Header.Set("Authorization", "Bearer "+adminToken)
	readinessRec := httptest.NewRecorder()
	handler.ServeHTTP(readinessRec, readinessReq)
	if readinessRec.Code != http.StatusOK {
		t.Fatalf("expected readiness 200, got %d body=%s", readinessRec.Code, readinessRec.Body.String())
	}
	var readinessResp struct {
		Data struct {
			Checks []struct {
				Key         string `json:"key"`
				Status      string `json:"status"`
				Detail      string `json:"detail"`
				ActionRoute string `json:"action_route"`
				Blocking    bool   `json:"blocking"`
			} `json:"checks"`
		} `json:"data"`
	}
	if err := json.NewDecoder(readinessRec.Body).Decode(&readinessResp); err != nil {
		t.Fatalf("decode readiness: %v", err)
	}
	for _, check := range readinessResp.Data.Checks {
		if check.Key == "refund_compensation" {
			if check.Status != "fail" || !check.Blocking || check.ActionRoute != "cashier" || !strings.Contains(check.Detail, "1 个") {
				t.Fatalf("expected blocking refund compensation readiness check, got %#v", check)
			}
			return
		}
	}
	t.Fatalf("expected refund_compensation readiness check body=%s", readinessRec.Body.String())
}

func TestAdminCashierOrderRefundCallsAlipayDirectProvider(t *testing.T) {
	privateKey, _ := testRSAKeyPairPEM(t)
	var upstreamPath string
	var upstreamQuery url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("expected alipay refund GET, got %s", r.Method)
		}
		upstreamQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alipay_trade_refund_response":{"code":"10000","msg":"Success","fund_change":"Y","trade_no":"ALIPAY-TRADE-001","out_trade_no":"ignored-by-test"}}`))
	}))
	defer upstream.Close()

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-refund-alipay-user@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"alipay_direct","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, fmt.Sprintf(`{"provider_type":"alipay_direct","name":"支付宝官方退款","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"app_id":"app-123","app_private_key":%q}}`, upstream.URL+"/gateway.do", privateKey))
	orderID, orderNo := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "alipay", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "ALIPAY-TRADE-001")

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-ALIPAY-001","reason":"customer requested refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusOK {
		t.Fatalf("expected admin alipay refund 200, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if upstreamPath != "/gateway.do" || upstreamQuery.Get("app_id") != "app-123" || upstreamQuery.Get("method") != "alipay.trade.refund" || upstreamQuery.Get("sign_type") != "RSA2" || upstreamQuery.Get("sign") == "" {
		t.Fatalf("unexpected alipay refund request path=%q query=%#v", upstreamPath, upstreamQuery)
	}
	bizContent := upstreamQuery.Get("biz_content")
	if !strings.Contains(bizContent, orderNo) || !strings.Contains(bizContent, `"refund_amount":"12.50000"`) || !strings.Contains(bizContent, `"out_request_no":"REFUND-ALIPAY-001"`) {
		t.Fatalf("unexpected alipay refund biz_content=%s order_no=%s", bizContent, orderNo)
	}
	assertAdminCashierRefundedForTest(t, refundRec.Body.String(), orderID, "REFUND-ALIPAY-001")
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after alipay refund: %v", err)
	}
	if balance.RechargePoints != "0.00000" || balance.AvailablePoints != "0.00000" {
		t.Fatalf("expected alipay refund to deduct local recharge balance, got %#v", balance)
	}
}

func TestAdminCashierStripeQueryAndPartialRefund(t *testing.T) {
	var queryPath string
	var refundValues url.Values
	var refundIdempotencyKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payment_intents":
			_, _ = w.Write([]byte(`{"id":"pi_admin_stripe","object":"payment_intent","amount":1025,"currency":"cny","client_secret":"pi_admin_stripe_secret_client","status":"requires_payment_method"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/payment_intents/pi_admin_stripe":
			queryPath = r.URL.Path
			_, _ = w.Write([]byte(`{"id":"pi_admin_stripe","object":"payment_intent","amount":1025,"currency":"cny","status":"succeeded"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
			refundIdempotencyKey = r.Header.Get("Idempotency-Key")
			if err := r.ParseForm(); err != nil {
				t.Fatalf("parse Stripe refund form: %v", err)
			}
			refundValues = r.PostForm
			_, _ = w.Write([]byte(`{"id":"re_admin_stripe","object":"refund","amount":525,"currency":"cny","payment_intent":"pi_admin_stripe","status":"succeeded"}`))
		default:
			t.Fatalf("unexpected Stripe request %s %s", r.Method, r.URL.Path)
		}
	}))
	defer upstream.Close()
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(upstream.URL), MaxNetworkRetries: stripe.Int64(0)}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-stripe-admin-user@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"stripe","label":"Stripe","enabled":true,"source_provider_type":"stripe","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, `{"provider_type":"stripe","name":"Stripe Test","enabled":true,"supported_methods":["stripe"],"sort_order":10,"scheduler_weight":100,"config":{"publishable_key":"pk_test_admin"},"secrets":{"secret_key":"sk_test_admin","webhook_secret":"whsec_admin"}}`)
	orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "stripe", "10.25")

	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("expected Stripe admin sync 200, got %d body=%s", syncRec.Code, syncRec.Body.String())
	}
	if queryPath != "/v1/payment_intents/pi_admin_stripe" {
		t.Fatalf("expected pending order sync to use client token, got query path %q", queryPath)
	}
	completed := getCashierOrderForTest(t, handler, userSession.AccessToken, orderID)
	if completed.Status != "completed" || completed.TradeNo != "pi_admin_stripe" {
		t.Fatalf("expected Stripe sync to complete order, got %#v", completed)
	}

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-STRIPE-001","refund_amount_cny":"5.25","reason":"partial refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusOK {
		t.Fatalf("expected Stripe partial refund 200, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if refundIdempotencyKey != "REFUND-STRIPE-001" || refundValues.Get("amount") != "525" || refundValues.Get("payment_intent") != "pi_admin_stripe" || refundValues.Get("metadata[refund_trade_no]") != "REFUND-STRIPE-001" {
		t.Fatalf("unexpected Stripe refund request idempotency=%q values=%#v", refundIdempotencyKey, refundValues)
	}
	refunded := getCashierOrderForTest(t, handler, userSession.AccessToken, orderID)
	if refunded.Status != "partially_refunded" || refunded.RefundTradeNo != "REFUND-STRIPE-001" || refunded.RefundedAmountCNY != "5.25000" {
		t.Fatalf("unexpected local Stripe refund state %#v", refunded)
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("load balance after Stripe refund: %v", err)
	}
	if balance.RechargePoints != "16.00000" || balance.AvailablePoints != "16.00000" {
		t.Fatalf("expected partial Stripe refund to retain exact remaining balance, got %#v", balance)
	}
}

func TestAdminCashierStripeRefundWaitsForProviderSuccess(t *testing.T) {
	tests := []struct {
		name              string
		initialStatus     string
		queryStatus       string
		wantFirstHTTPCode int
		wantFirstCode     string
		wantRetrySuccess  bool
	}{
		{name: "pending refund is queried before local settlement", initialStatus: "pending", queryStatus: "succeeded", wantFirstHTTPCode: http.StatusConflict, wantFirstCode: "PAYMENT_REFUND_PENDING", wantRetrySuccess: true},
		{name: "failed refund does not settle locally", initialStatus: "failed", wantFirstHTTPCode: http.StatusConflict, wantFirstCode: "PAYMENT_REFUND_FAILED"},
		{name: "canceled refund does not settle locally", initialStatus: "canceled", wantFirstHTTPCode: http.StatusConflict, wantFirstCode: "PAYMENT_REFUND_FAILED"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var refundCreates int
			var refundQueries int
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/v1/payment_intents":
					_, _ = w.Write([]byte(`{"id":"pi_refund_state","object":"payment_intent","amount":1000,"currency":"cny","client_secret":"pi_refund_state_secret_client","status":"requires_payment_method"}`))
				case r.Method == http.MethodGet && r.URL.Path == "/v1/payment_intents/pi_refund_state":
					_, _ = w.Write([]byte(`{"id":"pi_refund_state","object":"payment_intent","amount":1000,"currency":"cny","status":"succeeded"}`))
				case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
					refundCreates++
					_, _ = fmt.Fprintf(w, `{"id":"re_refund_state","object":"refund","amount":500,"currency":"cny","payment_intent":"pi_refund_state","status":%q}`, tt.initialStatus)
				case r.Method == http.MethodGet && r.URL.Path == "/v1/refunds/re_refund_state":
					refundQueries++
					_, _ = fmt.Fprintf(w, `{"id":"re_refund_state","object":"refund","amount":500,"currency":"cny","payment_intent":"pi_refund_state","status":%q}`, tt.queryStatus)
				default:
					t.Fatalf("unexpected Stripe request %s %s", r.Method, r.URL.Path)
				}
			}))
			defer upstream.Close()
			originalBackend := stripe.GetBackend(stripe.APIBackend)
			stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(upstream.URL), MaxNetworkRetries: stripe.Int64(0)}))
			t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

			handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-stripe-refund-state-"+tt.initialStatus+"@example.com")
			putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"stripe","label":"Stripe","enabled":true,"source_provider_type":"stripe","scheduler_strategy":"round_robin","display_order":10}]`)
			createProviderInstanceForCashierTest(t, handler, adminToken, `{"provider_type":"stripe","name":"Stripe Test","enabled":true,"supported_methods":["stripe"],"sort_order":10,"scheduler_weight":100,"config":{"publishable_key":"pk_test_admin"},"secrets":{"secret_key":"sk_test_admin","webhook_secret":"whsec_admin"}}`)
			orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "stripe", "10.00")
			syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/sync", nil)
			syncReq.Header.Set("Authorization", "Bearer "+adminToken)
			syncRec := httptest.NewRecorder()
			handler.ServeHTTP(syncRec, syncReq)
			if syncRec.Code != http.StatusOK {
				t.Fatalf("complete Stripe order: status=%d body=%s", syncRec.Code, syncRec.Body.String())
			}

			refundBody := `{"refund_trade_no":"REFUND-STRIPE-STATE-001","refund_amount_cny":"5.00","reason":"state check"}`
			refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(refundBody))
			refundReq.Header.Set("Authorization", "Bearer "+adminToken)
			refundReq.Header.Set("Content-Type", "application/json")
			refundRec := httptest.NewRecorder()
			handler.ServeHTTP(refundRec, refundReq)
			if refundRec.Code != tt.wantFirstHTTPCode || !bytes.Contains(refundRec.Body.Bytes(), []byte(`"code":"`+tt.wantFirstCode+`"`)) {
				t.Fatalf("unexpected first refund response status=%d body=%s", refundRec.Code, refundRec.Body.String())
			}
			pending := getCashierOrderForTest(t, handler, userSession.AccessToken, orderID)
			if pending.Status != "completed" || pending.RefundedAmountCNY != "" || pending.RefundedPoints != "" {
				t.Fatalf("provider %s must not settle local refund, got %#v", tt.initialStatus, pending)
			}

			if tt.wantRetrySuccess {
				retryReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(refundBody))
				retryReq.Header.Set("Authorization", "Bearer "+adminToken)
				retryReq.Header.Set("Content-Type", "application/json")
				retryRec := httptest.NewRecorder()
				handler.ServeHTTP(retryRec, retryReq)
				if retryRec.Code != http.StatusOK {
					t.Fatalf("expected queried Stripe refund to settle, status=%d body=%s", retryRec.Code, retryRec.Body.String())
				}
				settled := getCashierOrderForTest(t, handler, userSession.AccessToken, orderID)
				if settled.Status != "partially_refunded" || settled.RefundedAmountCNY != "5.00000" {
					t.Fatalf("unexpected queried Stripe refund state %#v", settled)
				}
			}
			if refundCreates != 1 || refundQueries != boolInt(tt.wantRetrySuccess) {
				t.Fatalf("unexpected Stripe refund calls creates=%d queries=%d", refundCreates, refundQueries)
			}
			balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
			if err != nil {
				t.Fatalf("GetBalance after Stripe refund state check: %v", err)
			}
			if tt.wantRetrySuccess {
				if balance.AvailablePoints != "16.00000" {
					t.Fatalf("expected settled partial refund balance, got %#v", balance)
				}
			} else if balance.AvailablePoints != "32.00000" {
				t.Fatalf("expected failed provider refund to release local balance, got %#v", balance)
			}
		})
	}
}

func TestAdminCashierStripeRefundKeepsFreezeWhenProviderOutcomeIsUncertain(t *testing.T) {
	var refundCreates int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payment_intents":
			_, _ = w.Write([]byte(`{"id":"pi_refund_uncertain","object":"payment_intent","amount":1000,"currency":"cny","client_secret":"pi_refund_uncertain_secret_client","status":"requires_payment_method"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/payment_intents/pi_refund_uncertain":
			_, _ = w.Write([]byte(`{"id":"pi_refund_uncertain","object":"payment_intent","amount":1000,"currency":"cny","status":"succeeded"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
			refundCreates++
			if refundCreates == 1 {
				_, _ = w.Write([]byte(`{"id":"re_refund_uncertain","object":"refund","amount":500`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"re_refund_uncertain","object":"refund","amount":500,"currency":"cny","payment_intent":"pi_refund_uncertain","status":"succeeded"}`))
		default:
			t.Errorf("unexpected Stripe request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(upstream.URL), MaxNetworkRetries: stripe.Int64(0)}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-stripe-refund-uncertain@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"stripe","label":"Stripe","enabled":true,"source_provider_type":"stripe","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, `{"provider_type":"stripe","name":"Stripe Test","enabled":true,"supported_methods":["stripe"],"sort_order":10,"scheduler_weight":100,"config":{"publishable_key":"pk_test_admin"},"secrets":{"secret_key":"sk_test_admin","webhook_secret":"whsec_admin"}}`)
	orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "stripe", "10.00")
	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("complete Stripe order: status=%d body=%s", syncRec.Code, syncRec.Body.String())
	}

	refundBody := `{"refund_trade_no":"REFUND-STRIPE-UNCERTAIN-001","refund_amount_cny":"5.00","reason":"uncertain result"}`
	firstReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(refundBody))
	firstReq.Header.Set("Authorization", "Bearer "+adminToken)
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusBadGateway || !bytes.Contains(firstRec.Body.Bytes(), []byte(`"code":"PAYMENT_PROVIDER_UNAVAILABLE"`)) {
		t.Fatalf("unexpected uncertain refund response status=%d body=%s", firstRec.Code, firstRec.Body.String())
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after uncertain Stripe refund: %v", err)
	}
	if balance.AvailablePoints != "16.00000" || balance.FrozenPoints != "16.00000" {
		t.Fatalf("uncertain Stripe outcome must keep refund points frozen, got %#v", balance)
	}

	retryReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(refundBody))
	retryReq.Header.Set("Authorization", "Bearer "+adminToken)
	retryReq.Header.Set("Content-Type", "application/json")
	retryRec := httptest.NewRecorder()
	handler.ServeHTTP(retryRec, retryReq)
	if retryRec.Code != http.StatusOK {
		t.Fatalf("retry uncertain Stripe refund: status=%d body=%s", retryRec.Code, retryRec.Body.String())
	}
	if refundCreates != 2 {
		t.Fatalf("expected one idempotent retry after uncertain response, got %d Stripe creates", refundCreates)
	}
	settled := getCashierOrderForTest(t, handler, userSession.AccessToken, orderID)
	if settled.Status != "partially_refunded" || settled.RefundedAmountCNY != "5.00000" {
		t.Fatalf("unexpected settled Stripe refund %#v", settled)
	}
}

func TestAdminCashierStripePendingRefundRejectsChangedAmountBeforeProviderQuery(t *testing.T) {
	var refundCreates int
	var refundQueries int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payment_intents":
			_, _ = w.Write([]byte(`{"id":"pi_refund_bound","object":"payment_intent","amount":1000,"currency":"cny","client_secret":"pi_refund_bound_secret_client","status":"requires_payment_method"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/payment_intents/pi_refund_bound":
			_, _ = w.Write([]byte(`{"id":"pi_refund_bound","object":"payment_intent","amount":1000,"currency":"cny","status":"succeeded"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
			refundCreates++
			_, _ = w.Write([]byte(`{"id":"re_refund_bound","object":"refund","amount":500,"currency":"cny","payment_intent":"pi_refund_bound","status":"pending"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/refunds/re_refund_bound":
			refundQueries++
			_, _ = w.Write([]byte(`{"id":"re_refund_bound","object":"refund","amount":500,"currency":"cny","payment_intent":"pi_refund_bound","status":"pending"}`))
		default:
			t.Errorf("unexpected Stripe request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(upstream.URL), MaxNetworkRetries: stripe.Int64(0)}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-stripe-refund-bound@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"stripe","label":"Stripe","enabled":true,"source_provider_type":"stripe","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, `{"provider_type":"stripe","name":"Stripe Test","enabled":true,"supported_methods":["stripe"],"sort_order":10,"scheduler_weight":100,"config":{"publishable_key":"pk_test_admin"},"secrets":{"secret_key":"sk_test_admin","webhook_secret":"whsec_admin"}}`)
	orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "stripe", "10.00")
	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("complete Stripe order: status=%d body=%s", syncRec.Code, syncRec.Body.String())
	}

	firstBody := `{"refund_trade_no":"REFUND-STRIPE-BOUND-001","refund_amount_cny":"5.00","reason":"pending amount"}`
	firstReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(firstBody))
	firstReq.Header.Set("Authorization", "Bearer "+adminToken)
	firstReq.Header.Set("Content-Type", "application/json")
	firstRec := httptest.NewRecorder()
	handler.ServeHTTP(firstRec, firstReq)
	if firstRec.Code != http.StatusConflict || !bytes.Contains(firstRec.Body.Bytes(), []byte(`"code":"PAYMENT_REFUND_PENDING"`)) {
		t.Fatalf("unexpected pending refund response status=%d body=%s", firstRec.Code, firstRec.Body.String())
	}

	changedBody := `{"refund_trade_no":"REFUND-STRIPE-BOUND-001","refund_amount_cny":"4.00","reason":"changed amount"}`
	changedReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(changedBody))
	changedReq.Header.Set("Authorization", "Bearer "+adminToken)
	changedReq.Header.Set("Content-Type", "application/json")
	changedRec := httptest.NewRecorder()
	handler.ServeHTTP(changedRec, changedReq)
	if changedRec.Code != http.StatusConflict || !bytes.Contains(changedRec.Body.Bytes(), []byte(`"code":"PAYMENT_AMOUNT_MISMATCH"`)) {
		t.Fatalf("changed pending refund amount should be rejected, status=%d body=%s", changedRec.Code, changedRec.Body.String())
	}
	if refundCreates != 1 || refundQueries != 0 {
		t.Fatalf("changed amount must fail before another Stripe call, creates=%d queries=%d", refundCreates, refundQueries)
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after changed refund amount: %v", err)
	}
	if balance.AvailablePoints != "16.00000" || balance.FrozenPoints != "16.00000" {
		t.Fatalf("changed amount must not release the original freeze, got %#v", balance)
	}
}

func TestAdminCashierStripeRefundReleasesFreezeWhenProviderInstanceIsMissing(t *testing.T) {
	var refundCreates int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/payment_intents":
			_, _ = w.Write([]byte(`{"id":"pi_refund_missing_instance","object":"payment_intent","amount":1000,"currency":"cny","client_secret":"pi_refund_missing_instance_secret_client","status":"requires_payment_method"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/payment_intents/pi_refund_missing_instance":
			_, _ = w.Write([]byte(`{"id":"pi_refund_missing_instance","object":"payment_intent","amount":1000,"currency":"cny","status":"succeeded"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/v1/refunds":
			refundCreates++
			_, _ = w.Write([]byte(`{"id":"re_unexpected","object":"refund","amount":500,"currency":"cny","payment_intent":"pi_refund_missing_instance","status":"succeeded"}`))
		default:
			t.Errorf("unexpected Stripe request %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer upstream.Close()
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(upstream.URL), MaxNetworkRetries: stripe.Int64(0)}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-stripe-refund-missing-instance@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"stripe","label":"Stripe","enabled":true,"source_provider_type":"stripe","scheduler_strategy":"round_robin","display_order":10}]`)
	providerID := createProviderInstanceForCashierTest(t, handler, adminToken, `{"provider_type":"stripe","name":"Stripe Test","enabled":true,"supported_methods":["stripe"],"sort_order":10,"scheduler_weight":100,"config":{"publishable_key":"pk_test_admin"},"secrets":{"secret_key":"sk_test_admin","webhook_secret":"whsec_admin"}}`)
	orderID, _ := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "stripe", "10.00")
	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("complete Stripe order: status=%d body=%s", syncRec.Code, syncRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/cashier/provider-instances/"+jsonInt64(providerID), nil)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("delete Stripe provider instance: status=%d body=%s", deleteRec.Code, deleteRec.Body.String())
	}

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-STRIPE-MISSING-INSTANCE-001","refund_amount_cny":"5.00"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusConflict || !bytes.Contains(refundRec.Body.Bytes(), []byte(`"code":"PAYMENT_PROVIDER_UNAVAILABLE"`)) {
		t.Fatalf("unexpected missing provider refund response status=%d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if refundCreates != 0 {
		t.Fatalf("missing provider instance must fail before Stripe refund call, got %d calls", refundCreates)
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after missing provider refund: %v", err)
	}
	if balance.AvailablePoints != "32.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("certain pre-call failure must release refund freeze, got %#v", balance)
	}
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func TestAdminCashierOrderRefundCallsWxPayDirectProvider(t *testing.T) {
	privateKey, _ := testRSAKeyPairPEM(t)
	var upstreamPath string
	var upstreamAuth string
	var upstreamPayload map[string]any
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		upstreamAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("expected wxpay refund POST, got %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read wxpay refund body: %v", err)
		}
		if err := json.Unmarshal(body, &upstreamPayload); err != nil {
			t.Fatalf("decode wxpay refund body: %v body=%s", err, string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"refund_id":"WXPAY-REFUND-001","status":"SUCCESS","out_refund_no":"REFUND-WXPAY-001"}`))
	}))
	defer upstream.Close()

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-refund-wxpay-user@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"wxpay","label":"微信支付","enabled":true,"source_provider_type":"wxpay_direct","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, fmt.Sprintf(`{"provider_type":"wxpay_direct","name":"微信支付官方退款","enabled":true,"supported_methods":["wxpay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"app_id":"wx-app-123","mch_id":"mch-123","merchant_private_key":%q,"merchant_certificate_serial":"MERCHANTSERIAL001","qr_code":"weixin://wxpay/bizpayurl?pr=refund-test"}}`, upstream.URL, privateKey))
	orderID, orderNo := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "wxpay", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "WXPAY-TRADE-001")

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-WXPAY-001","reason":"customer requested refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusOK {
		t.Fatalf("expected admin wxpay refund 200, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if upstreamPath != "/v3/refund/domestic/refunds" || !strings.HasPrefix(upstreamAuth, "WECHATPAY2-SHA256-RSA2048 ") || !strings.Contains(upstreamAuth, `mchid="mch-123"`) || !strings.Contains(upstreamAuth, `serial_no="MERCHANTSERIAL001"`) {
		t.Fatalf("unexpected wxpay refund request path=%q auth=%q", upstreamPath, upstreamAuth)
	}
	if upstreamPayload["out_trade_no"] != orderNo || upstreamPayload["out_refund_no"] != "REFUND-WXPAY-001" {
		t.Fatalf("unexpected wxpay refund payload %#v order_no=%s", upstreamPayload, orderNo)
	}
	amount, ok := upstreamPayload["amount"].(map[string]any)
	if !ok || fmt.Sprint(amount["refund"]) != "1250" || fmt.Sprint(amount["total"]) != "1250" || amount["currency"] != "CNY" {
		t.Fatalf("unexpected wxpay refund amount payload %#v", upstreamPayload)
	}
	assertAdminCashierRefundedForTest(t, refundRec.Body.String(), orderID, "REFUND-WXPAY-001")
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after wxpay refund: %v", err)
	}
	if balance.RechargePoints != "0.00000" || balance.AvailablePoints != "0.00000" {
		t.Fatalf("expected wxpay refund to deduct local recharge balance, got %#v", balance)
	}
}

func TestAdminCashierOrderRefundCallsJeePayProvider(t *testing.T) {
	var upstreamPath string
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("expected jeepay refund POST, got %s", r.Method)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read jeepay refund body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse jeepay refund form: %v body=%s", err, string(body))
		}
		upstreamValues = values
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"refundOrderId":"JEEPAY-REFUND-001","mchRefundNo":"REFUND-JEEPAY-001","refundAmount":1250,"state":2,"channelOrderNo":"CHANNEL-REFUND-001"}}`))
	}))
	defer upstream.Close()

	handler, adminToken, user, userSession, billingSvc := newAdminCashierRefundProviderTest(t, "cashier-refund-jeepay-user@example.com")
	putVisibleMethodsForCashierTest(t, handler, adminToken, `[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"jeepay_alipay","scheduler_strategy":"round_robin","display_order":10}]`)
	createProviderInstanceForCashierTest(t, handler, adminToken, fmt.Sprintf(`{"provider_type":"jeepay_alipay","name":"JeePay 支付宝退款","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"mch_no":"MCH10001","app_id":"APP10001","key":"merchant-secret","way_code":"ALI_PC","client_ip":"127.0.0.1"}}`, upstream.URL))
	orderID, orderNo := createCustomCashierOrderForTest(t, handler, userSession.AccessToken, "alipay", "12.50000")
	completeAdminCashierOrderForTest(t, handler, adminToken, orderID, "JEEPAY-PAY-001")

	refundReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/refund", bytes.NewBufferString(`{"refund_trade_no":"REFUND-JEEPAY-001","reason":"customer requested refund"}`))
	refundReq.Header.Set("Authorization", "Bearer "+adminToken)
	refundReq.Header.Set("Content-Type", "application/json")
	refundRec := httptest.NewRecorder()
	handler.ServeHTTP(refundRec, refundReq)
	if refundRec.Code != http.StatusOK {
		t.Fatalf("expected admin jeepay refund 200, got %d body=%s", refundRec.Code, refundRec.Body.String())
	}
	if upstreamPath != "/api/refund/refundOrder" || upstreamValues.Get("mchNo") != "MCH10001" || upstreamValues.Get("appId") != "APP10001" || upstreamValues.Get("mchOrderNo") != orderNo || upstreamValues.Get("payOrderId") != "JEEPAY-PAY-001" || upstreamValues.Get("mchRefundNo") != "REFUND-JEEPAY-001" || upstreamValues.Get("refundAmount") != "1250" || upstreamValues.Get("currency") != "cny" || upstreamValues.Get("refundReason") != "customer requested refund" || upstreamValues.Get("clientIp") != "127.0.0.1" || upstreamValues.Get("version") != "1.0" || upstreamValues.Get("signType") != "MD5" || upstreamValues.Get("sign") == "" {
		t.Fatalf("unexpected jeepay refund request path=%q values=%#v order_no=%s", upstreamPath, upstreamValues, orderNo)
	}
	assertAdminCashierRefundedForTest(t, refundRec.Body.String(), orderID, "REFUND-JEEPAY-001")
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after jeepay refund: %v", err)
	}
	if balance.RechargePoints != "0.00000" || balance.AvailablePoints != "0.00000" {
		t.Fatalf("expected jeepay refund to deduct local recharge balance, got %#v", balance)
	}
}

func TestAdminCashierOrderSyncCompletesPaidProviderOrder(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-sync-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, userSession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-sync-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierTest(t, handler)

	providerBody := `{"provider_type":"mock","name":"Mock 查单已支付","enabled":true,"supported_methods":["mock"],"sort_order":1,"scheduler_weight":100,"config":{"mock":true,"query_status":"paid","query_trade_no":"SYNC-TRADE-001","query_amount_cny":"19.90000"}}`
	providerReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(providerBody))
	providerReq.Header.Set("Authorization", "Bearer "+adminToken)
	providerReq.Header.Set("Content-Type", "application/json")
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider instance create 201, got %d body=%s", providerRec.Code, providerRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	if createResp.Data.Status != "pending" {
		t.Fatalf("expected pending order before sync, got %#v", createResp.Data)
	}

	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("expected admin order sync 200, got %d body=%s", syncRec.Code, syncRec.Body.String())
	}
	var syncResp struct {
		Data struct {
			Order struct {
				ID       int64  `json:"id"`
				Status   string `json:"status"`
				TradeNo  string `json:"trade_no"`
				LedgerID int64  `json:"ledger_id"`
			} `json:"order"`
			Sync struct {
				QueryStatus string `json:"query_status"`
				Paid        bool   `json:"paid"`
				Completed   bool   `json:"completed"`
				TradeNo     string `json:"trade_no"`
				AmountCNY   string `json:"amount_cny"`
			} `json:"sync"`
		} `json:"data"`
	}
	if err := json.NewDecoder(syncRec.Body).Decode(&syncResp); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if syncResp.Data.Order.ID != createResp.Data.ID || syncResp.Data.Order.Status != "completed" || syncResp.Data.Order.TradeNo != "SYNC-TRADE-001" || syncResp.Data.Order.LedgerID == 0 {
		t.Fatalf("unexpected synced order %#v", syncResp.Data.Order)
	}
	if syncResp.Data.Sync.QueryStatus != "paid" || !syncResp.Data.Sync.Paid || !syncResp.Data.Sync.Completed || syncResp.Data.Sync.TradeNo != "SYNC-TRADE-001" || syncResp.Data.Sync.AmountCNY != "19.90000" {
		t.Fatalf("unexpected sync result %#v", syncResp.Data.Sync)
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after sync: %v", err)
	}
	if balance.RechargePoints != "100.00000" || balance.AvailablePoints != "100.00000" {
		t.Fatalf("expected sync to credit recharge balance, got %#v", balance)
	}
}

func TestAdminCashierOrderSyncRejectsPaidAmountMismatch(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-sync-mismatch-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, userSession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-sync-mismatch-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierTest(t, handler)

	providerBody := `{"provider_type":"mock","name":"Mock 查单金额不一致","enabled":true,"supported_methods":["mock"],"sort_order":1,"scheduler_weight":100,"config":{"mock":true,"query_status":"paid","query_trade_no":"SYNC-TRADE-MISMATCH","query_amount_cny":"18.00000"}}`
	providerReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(providerBody))
	providerReq.Header.Set("Authorization", "Bearer "+adminToken)
	providerReq.Header.Set("Content-Type", "application/json")
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider instance create 201, got %d body=%s", providerRec.Code, providerRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}

	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusConflict {
		t.Fatalf("expected amount mismatch 409, got %d body=%s", syncRec.Code, syncRec.Body.String())
	}
	if !bytes.Contains(syncRec.Body.Bytes(), []byte(`PAYMENT_AMOUNT_MISMATCH`)) {
		t.Fatalf("expected PAYMENT_AMOUNT_MISMATCH body=%s", syncRec.Body.String())
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after mismatch sync: %v", err)
	}
	if balance.RechargePoints != "0.00000" || balance.AvailablePoints != "0.00000" {
		t.Fatalf("expected mismatch sync not to credit balance, got %#v", balance)
	}
}

func TestAdminCashierOrderSyncClassifiesRiskControlStatus(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-sync-risk-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	_, session, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-sync-risk-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	handlerAPI := handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil, nil, nil, nil, nil)
	handler := NewWithAPI(handlerAPI)
	adminToken := loginAdminForCashierTest(t, handler)

	providerBody := `{"provider_type":"mock","name":"Mock 风控查单","enabled":true,"supported_methods":["mock"],"sort_order":1,"scheduler_weight":100,"config":{"mock":true,"query_status":"risk_control","query_trade_no":"SYNC-RISK-001","query_amount_cny":"19.90000"}}`
	providerReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(providerBody))
	providerReq.Header.Set("Authorization", "Bearer "+adminToken)
	providerReq.Header.Set("Content-Type", "application/json")
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider create 201, got %d body=%s", providerRec.Code, providerRec.Body.String())
	}

	orderID, _ := createCustomCashierOrderForTest(t, handler, session.AccessToken, "mock", "19.90000")
	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("expected sync 200, got %d body=%s", syncRec.Code, syncRec.Body.String())
	}
	var syncResp struct {
		Data struct {
			Sync struct {
				QueryStatus  string `json:"query_status"`
				RiskCategory string `json:"risk_category"`
				ActionHint   string `json:"action_hint"`
			} `json:"sync"`
		} `json:"data"`
	}
	if err := json.NewDecoder(syncRec.Body).Decode(&syncResp); err != nil {
		t.Fatalf("decode sync response: %v body=%s", err, syncRec.Body.String())
	}
	if syncResp.Data.Sync.QueryStatus != "failed" || syncResp.Data.Sync.RiskCategory != "risk_control" || !strings.Contains(syncResp.Data.Sync.ActionHint, "更换支付渠道") {
		t.Fatalf("expected risk-control sync classification, got %#v", syncResp.Data.Sync)
	}
}

func TestAdminCashierOrderSyncQueriesAlipayDirectProvider(t *testing.T) {
	privateKey, _ := testRSAKeyPairPEM(t)
	var upstreamPath string
	var upstreamQuery url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodGet {
			t.Fatalf("expected alipay query GET, got %s", r.Method)
		}
		upstreamQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"alipay_trade_query_response":{"code":"10000","msg":"Success","trade_status":"TRADE_SUCCESS","trade_no":"ALIPAY-QUERY-001","total_amount":"12.50000"}}`))
	}))
	defer upstream.Close()

	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-sync-alipay-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, userSession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-sync-alipay-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"alipay_direct","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := fmt.Sprintf(`{"provider_type":"alipay_direct","name":"支付宝官方查单","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"app_id":"app-123","app_private_key":%q}}`, upstream.URL+"/gateway.do", privateKey)
	providerReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(providerBody))
	providerReq.Header.Set("Authorization", "Bearer "+adminToken)
	providerReq.Header.Set("Content-Type", "application/json")
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider instance create 201, got %d body=%s", providerRec.Code, providerRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"12.50000","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID      int64  `json:"id"`
			OrderNo string `json:"order_no"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}

	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("expected admin order sync 200, got %d body=%s", syncRec.Code, syncRec.Body.String())
	}
	var syncResp struct {
		Data struct {
			Order struct {
				Status  string `json:"status"`
				TradeNo string `json:"trade_no"`
			} `json:"order"`
			Sync struct {
				QueryStatus string         `json:"query_status"`
				Paid        bool           `json:"paid"`
				Completed   bool           `json:"completed"`
				TradeNo     string         `json:"trade_no"`
				AmountCNY   string         `json:"amount_cny"`
				Raw         map[string]any `json:"raw"`
			} `json:"sync"`
		} `json:"data"`
	}
	if err := json.NewDecoder(syncRec.Body).Decode(&syncResp); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if upstreamPath != "/gateway.do" || upstreamQuery.Get("app_id") != "app-123" || upstreamQuery.Get("method") != "alipay.trade.query" || upstreamQuery.Get("sign_type") != "RSA2" || upstreamQuery.Get("sign") == "" || !strings.Contains(upstreamQuery.Get("biz_content"), createResp.Data.OrderNo) {
		t.Fatalf("unexpected alipay query request path=%q query=%#v order=%#v", upstreamPath, upstreamQuery, createResp.Data)
	}
	if syncResp.Data.Order.Status != "completed" || syncResp.Data.Order.TradeNo != "ALIPAY-QUERY-001" {
		t.Fatalf("unexpected alipay synced order %#v", syncResp.Data.Order)
	}
	if syncResp.Data.Sync.QueryStatus != "paid" || !syncResp.Data.Sync.Paid || !syncResp.Data.Sync.Completed || syncResp.Data.Sync.TradeNo != "ALIPAY-QUERY-001" || syncResp.Data.Sync.AmountCNY != "12.50000" || syncResp.Data.Sync.Raw["source"] != "alipay_query_api" {
		t.Fatalf("unexpected alipay sync result %#v", syncResp.Data.Sync)
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after alipay sync: %v", err)
	}
	if balance.RechargePoints != "40.00000" || balance.AvailablePoints != "40.00000" {
		t.Fatalf("expected alipay sync to credit recharge balance, got %#v", balance)
	}
}

func TestAdminCashierOrderSyncQueriesWxPayDirectProvider(t *testing.T) {
	privateKey, _ := testRSAKeyPairPEM(t)
	var upstreamPath string
	var upstreamAuth string
	var upstreamQuery url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		upstreamQuery = r.URL.Query()
		upstreamAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodGet {
			t.Fatalf("expected wxpay query GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"appid":"wx-app-123","mchid":"mch-123","out_trade_no":"ignored-by-test","transaction_id":"WXPAY-QUERY-001","trade_state":"SUCCESS","amount":{"total":1250,"currency":"CNY"}}`))
	}))
	defer upstream.Close()

	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-sync-wxpay-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, userSession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-sync-wxpay-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"wxpay","label":"微信支付","enabled":true,"source_provider_type":"wxpay_direct","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := fmt.Sprintf(`{"provider_type":"wxpay_direct","name":"微信支付官方查单","enabled":true,"supported_methods":["wxpay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"app_id":"wx-app-123","mch_id":"mch-123","merchant_private_key":%q,"merchant_certificate_serial":"MERCHANTSERIAL001","qr_code":"weixin://wxpay/bizpayurl?pr=query-test"}}`, upstream.URL, privateKey)
	providerReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(providerBody))
	providerReq.Header.Set("Authorization", "Bearer "+adminToken)
	providerReq.Header.Set("Content-Type", "application/json")
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider instance create 201, got %d body=%s", providerRec.Code, providerRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"12.50000","visible_method":"wxpay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID      int64  `json:"id"`
			OrderNo string `json:"order_no"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}

	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("expected admin order sync 200, got %d body=%s", syncRec.Code, syncRec.Body.String())
	}
	var syncResp struct {
		Data struct {
			Order struct {
				Status  string `json:"status"`
				TradeNo string `json:"trade_no"`
			} `json:"order"`
			Sync struct {
				QueryStatus string         `json:"query_status"`
				Paid        bool           `json:"paid"`
				Completed   bool           `json:"completed"`
				TradeNo     string         `json:"trade_no"`
				AmountCNY   string         `json:"amount_cny"`
				Raw         map[string]any `json:"raw"`
			} `json:"sync"`
		} `json:"data"`
	}
	if err := json.NewDecoder(syncRec.Body).Decode(&syncResp); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	expectedPath := "/v3/pay/transactions/out-trade-no/" + createResp.Data.OrderNo
	if upstreamPath != expectedPath || upstreamQuery.Get("mchid") != "mch-123" || !strings.HasPrefix(upstreamAuth, "WECHATPAY2-SHA256-RSA2048 ") || !strings.Contains(upstreamAuth, `mchid="mch-123"`) || !strings.Contains(upstreamAuth, `serial_no="MERCHANTSERIAL001"`) || !strings.Contains(upstreamAuth, `signature="`) {
		t.Fatalf("unexpected wxpay query request path=%q query=%#v auth=%q order=%#v", upstreamPath, upstreamQuery, upstreamAuth, createResp.Data)
	}
	if syncResp.Data.Order.Status != "completed" || syncResp.Data.Order.TradeNo != "WXPAY-QUERY-001" {
		t.Fatalf("unexpected wxpay synced order %#v", syncResp.Data.Order)
	}
	if syncResp.Data.Sync.QueryStatus != "paid" || !syncResp.Data.Sync.Paid || !syncResp.Data.Sync.Completed || syncResp.Data.Sync.TradeNo != "WXPAY-QUERY-001" || syncResp.Data.Sync.AmountCNY != "12.50000" || syncResp.Data.Sync.Raw["source"] != "wxpay_query_api" {
		t.Fatalf("unexpected wxpay sync result %#v", syncResp.Data.Sync)
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after wxpay sync: %v", err)
	}
	if balance.RechargePoints != "40.00000" || balance.AvailablePoints != "40.00000" {
		t.Fatalf("expected wxpay sync to credit recharge balance, got %#v", balance)
	}
}

func TestAdminCashierOrderSyncQueriesEasyPayProvider(t *testing.T) {
	var upstreamPath string
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("expected easypay query POST, got %s", r.Method)
		}
		if r.URL.Path != "/api.php" {
			t.Fatalf("expected easypay query path /api.php, got %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse easypay query form: %v", err)
		}
		upstreamValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"success","status":1,"money":"12.50000","trade_no":"EASYPAY-QUERY-001"}`))
	}))
	defer upstream.Close()

	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-sync-easypay-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, userSession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-sync-easypay-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := fmt.Sprintf(`{"provider_type":"easypay_alipay","name":"易支付支付宝查单","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"pid":"10001","key":"merchant-secret","payment_mode":"popup"}}`, upstream.URL)
	providerReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(providerBody))
	providerReq.Header.Set("Authorization", "Bearer "+adminToken)
	providerReq.Header.Set("Content-Type", "application/json")
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider instance create 201, got %d body=%s", providerRec.Code, providerRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"12.50000","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID      int64  `json:"id"`
			OrderNo string `json:"order_no"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}

	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("expected admin order sync 200, got %d body=%s", syncRec.Code, syncRec.Body.String())
	}
	var syncResp struct {
		Data struct {
			Order struct {
				Status  string `json:"status"`
				TradeNo string `json:"trade_no"`
			} `json:"order"`
			Sync struct {
				QueryStatus string         `json:"query_status"`
				Paid        bool           `json:"paid"`
				Completed   bool           `json:"completed"`
				TradeNo     string         `json:"trade_no"`
				AmountCNY   string         `json:"amount_cny"`
				Raw         map[string]any `json:"raw"`
			} `json:"sync"`
		} `json:"data"`
	}
	if err := json.NewDecoder(syncRec.Body).Decode(&syncResp); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if upstreamPath != "/api.php" || upstreamValues.Get("act") != "order" || upstreamValues.Get("pid") != "10001" || upstreamValues.Get("key") != "merchant-secret" || upstreamValues.Get("out_trade_no") != createResp.Data.OrderNo {
		t.Fatalf("unexpected easypay query request path=%q values=%#v order=%#v", upstreamPath, upstreamValues, createResp.Data)
	}
	if syncResp.Data.Order.Status != "completed" || syncResp.Data.Order.TradeNo != "EASYPAY-QUERY-001" {
		t.Fatalf("unexpected easypay synced order %#v", syncResp.Data.Order)
	}
	if syncResp.Data.Sync.QueryStatus != "paid" || !syncResp.Data.Sync.Paid || !syncResp.Data.Sync.Completed || syncResp.Data.Sync.TradeNo != "EASYPAY-QUERY-001" || syncResp.Data.Sync.AmountCNY != "12.50000" || syncResp.Data.Sync.Raw["source"] != "easypay_query_api" {
		t.Fatalf("unexpected easypay sync result %#v", syncResp.Data.Sync)
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after easypay sync: %v", err)
	}
	if balance.RechargePoints != "40.00000" || balance.AvailablePoints != "40.00000" {
		t.Fatalf("expected easypay sync to credit recharge balance, got %#v", balance)
	}
}

func TestAdminCashierOrderSyncQueriesJeePayProvider(t *testing.T) {
	var upstreamPath string
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("expected jeepay query POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/pay/query" {
			t.Fatalf("expected jeepay query path /api/pay/query, got %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read jeepay query body: %v", err)
		}
		values, err := url.ParseQuery(string(body))
		if err != nil {
			t.Fatalf("parse jeepay query form: %v", err)
		}
		upstreamValues = values
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"success","data":{"state":2,"amount":1250,"payOrderId":"JEEPAY-QUERY-001"}}`))
	}))
	defer upstream.Close()

	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	if err := authSvc.SendEmailCode("cashier-sync-jeepay-user@example.com", "login"); err != nil {
		t.Fatalf("SendEmailCode: %v", err)
	}
	user, userSession, err := loginAuthUserWithPasswordSetup(t, authSvc, "cashier-sync-jeepay-user@example.com", "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"jeepay_alipay","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := fmt.Sprintf(`{"provider_type":"jeepay_alipay","name":"JeePay 支付宝查单","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"config":{"gateway_url":%q,"mch_no":"MCH10001","app_id":"APP10001","key":"merchant-secret","way_code":"ALI_PC"}}`, upstream.URL)
	providerReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(providerBody))
	providerReq.Header.Set("Authorization", "Bearer "+adminToken)
	providerReq.Header.Set("Content-Type", "application/json")
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider instance create 201, got %d body=%s", providerRec.Code, providerRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"12.50000","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID      int64  `json:"id"`
			OrderNo string `json:"order_no"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}

	syncReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(createResp.Data.ID)+"/sync", nil)
	syncReq.Header.Set("Authorization", "Bearer "+adminToken)
	syncRec := httptest.NewRecorder()
	handler.ServeHTTP(syncRec, syncReq)
	if syncRec.Code != http.StatusOK {
		t.Fatalf("expected admin order sync 200, got %d body=%s", syncRec.Code, syncRec.Body.String())
	}
	var syncResp struct {
		Data struct {
			Order struct {
				Status  string `json:"status"`
				TradeNo string `json:"trade_no"`
			} `json:"order"`
			Sync struct {
				QueryStatus string         `json:"query_status"`
				Paid        bool           `json:"paid"`
				Completed   bool           `json:"completed"`
				TradeNo     string         `json:"trade_no"`
				AmountCNY   string         `json:"amount_cny"`
				Raw         map[string]any `json:"raw"`
			} `json:"sync"`
		} `json:"data"`
	}
	if err := json.NewDecoder(syncRec.Body).Decode(&syncResp); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if upstreamPath != "/api/pay/query" || upstreamValues.Get("mchNo") != "MCH10001" || upstreamValues.Get("appId") != "APP10001" || upstreamValues.Get("mchOrderNo") != createResp.Data.OrderNo || upstreamValues.Get("signType") != "MD5" || upstreamValues.Get("sign") == "" {
		t.Fatalf("unexpected jeepay query request path=%q values=%#v order=%#v", upstreamPath, upstreamValues, createResp.Data)
	}
	if syncResp.Data.Order.Status != "completed" || syncResp.Data.Order.TradeNo != "JEEPAY-QUERY-001" {
		t.Fatalf("unexpected jeepay synced order %#v", syncResp.Data.Order)
	}
	if syncResp.Data.Sync.QueryStatus != "paid" || !syncResp.Data.Sync.Paid || !syncResp.Data.Sync.Completed || syncResp.Data.Sync.TradeNo != "JEEPAY-QUERY-001" || syncResp.Data.Sync.AmountCNY != "12.50000" || syncResp.Data.Sync.Raw["source"] != "jeepay_query_api" {
		t.Fatalf("unexpected jeepay sync result %#v", syncResp.Data.Sync)
	}
	balance, err := billingSvc.GetBalance(t.Context(), user.ID, "1.00000")
	if err != nil {
		t.Fatalf("GetBalance after jeepay sync: %v", err)
	}
	if balance.RechargePoints != "40.00000" || balance.AvailablePoints != "40.00000" {
		t.Fatalf("expected jeepay sync to credit recharge balance, got %#v", balance)
	}
}

func TestAdminCashierVisibleMethodsAllowsTrailingSlash(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierTest(t, handler)

	req := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods/", bytes.NewBufferString(`{"items":[{"method":"mock","label":"Mock 测试","enabled":true,"source_provider_type":"mock","scheduler_strategy":"round_robin","display_order":10}]}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected trailing slash visible methods PUT 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			Items []struct {
				Method string `json:"method"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data.Items) != 1 || resp.Data.Items[0].Method != "mock" {
		t.Fatalf("expected visible methods payload from cashier handler, got %#v body=%s", resp.Data, rec.Body.String())
	}
}

func TestAdminCashierCustomAmountConfig(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Billing.CNYPerPoint = "0.25000"
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-config-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierConfigTest(t, handler)

	getConfig := func() struct {
		Enabled      bool   `json:"enabled"`
		MinAmountCNY string `json:"min_amount_cny"`
		MaxAmountCNY string `json:"max_amount_cny"`
		CNYPerPoint  string `json:"cny_per_point"`
	} {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/custom-amount-config", nil)
		req.Header.Set("Authorization", "Bearer "+adminToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected custom amount config GET 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Data struct {
				Enabled      bool   `json:"enabled"`
				MinAmountCNY string `json:"min_amount_cny"`
				MaxAmountCNY string `json:"max_amount_cny"`
				CNYPerPoint  string `json:"cny_per_point"`
			} `json:"data"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode config: %v", err)
		}
		return resp.Data
	}

	initial := getConfig()
	if !initial.Enabled || initial.MinAmountCNY != "1.00000" || initial.MaxAmountCNY != "999.00000" || initial.CNYPerPoint != "0.25000" {
		t.Fatalf("unexpected initial custom amount config %#v", initial)
	}

	updateReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/custom-amount-config", bytes.NewBufferString(`{"enabled":false,"min_amount_cny":"5.00000","max_amount_cny":"500.00000","cny_per_point":"0.50000"}`))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected custom amount config PUT 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	updated := getConfig()
	if updated.Enabled || updated.MinAmountCNY != "5.00000" || updated.MaxAmountCNY != "500.00000" || updated.CNYPerPoint != "0.50000" {
		t.Fatalf("unexpected updated custom amount config %#v", updated)
	}
}

func TestAdminCashierPlanCreateAndUpdate(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-plan-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierPlanTest(t, handler)

	createBody := `{"plan_code":"points-250","plan_name":"250 积分包","plan_type":"points_package","purchase_enabled":true,"price_cny":"39.90000","points":"250.00000","bonus_points":"25.00000","currency":"CNY","sort_order":3,"description":"适合集中体验"}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/plans", bytes.NewBufferString(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected cashier plan create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID              int64  `json:"id"`
			PlanCode        string `json:"plan_code"`
			PlanName        string `json:"plan_name"`
			PlanType        string `json:"plan_type"`
			PurchaseEnabled bool   `json:"purchase_enabled"`
			PriceCNY        string `json:"price_cny"`
			Points          string `json:"points"`
			BonusPoints     string `json:"bonus_points"`
			SortOrder       int    `json:"sort_order"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if createResp.Data.ID == 0 || createResp.Data.PlanCode != "points-250" || createResp.Data.PlanType != "points_package" || !createResp.Data.PurchaseEnabled || createResp.Data.SortOrder != 3 {
		t.Fatalf("unexpected created plan %#v", createResp.Data)
	}

	updateBody := `{"plan_name":"250 积分包 Pro","plan_type":"subscription","purchase_enabled":false,"price_cny":"59.90000","points":"260.00000","bonus_points":"0.00000","currency":"CNY","sort_order":9,"status":"active","description":"订阅占位，暂不开放购买"}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/plans/"+jsonInt64(createResp.Data.ID), bytes.NewBufferString(updateBody))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected cashier plan update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	var updateResp struct {
		Data struct {
			PlanCode        string `json:"plan_code"`
			PlanName        string `json:"plan_name"`
			PlanType        string `json:"plan_type"`
			PurchaseEnabled bool   `json:"purchase_enabled"`
			PriceCNY        string `json:"price_cny"`
			Points          string `json:"points"`
			BonusPoints     string `json:"bonus_points"`
			SortOrder       int    `json:"sort_order"`
			Description     string `json:"description"`
		} `json:"data"`
	}
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode update response: %v", err)
	}
	if updateResp.Data.PlanCode != "points-250" || updateResp.Data.PlanName != "250 积分包 Pro" || updateResp.Data.PlanType != "subscription" || updateResp.Data.PurchaseEnabled || updateResp.Data.PriceCNY != "59.90000" || updateResp.Data.SortOrder != 9 {
		t.Fatalf("unexpected updated plan %#v", updateResp.Data)
	}

	adminListReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/plans?page=1&page_size=20", nil)
	adminListReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminListRec := httptest.NewRecorder()
	handler.ServeHTTP(adminListRec, adminListReq)
	if adminListRec.Code != http.StatusOK {
		t.Fatalf("expected cashier plan list 200, got %d body=%s", adminListRec.Code, adminListRec.Body.String())
	}
	if !bytes.Contains(adminListRec.Body.Bytes(), []byte(`"points-250"`)) || !bytes.Contains(adminListRec.Body.Bytes(), []byte(`"subscription"`)) {
		t.Fatalf("expected admin plan list to include hidden subscription placeholder, body=%s", adminListRec.Body.String())
	}

	optionsReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/options", nil)
	optionsReq.Header.Set("Authorization", "Bearer "+loginExistingAuthUser(t, authSvc, "cashier-plan-user@example.com").AccessToken)
	optionsRec := httptest.NewRecorder()
	handler.ServeHTTP(optionsRec, optionsReq)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("expected cashier options 200, got %d body=%s", optionsRec.Code, optionsRec.Body.String())
	}
	if bytes.Contains(optionsRec.Body.Bytes(), []byte(`"points-250"`)) {
		t.Fatalf("expected disabled subscription placeholder to be hidden from user options, body=%s", optionsRec.Body.String())
	}

	legacyPlansReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/plans", nil)
	legacyPlansReq.Header.Set("Authorization", "Bearer "+loginExistingAuthUser(t, authSvc, "cashier-plan-legacy-user@example.com").AccessToken)
	legacyPlansRec := httptest.NewRecorder()
	handler.ServeHTTP(legacyPlansRec, legacyPlansReq)
	if legacyPlansRec.Code != http.StatusOK {
		t.Fatalf("expected legacy billing plans 200, got %d body=%s", legacyPlansRec.Code, legacyPlansRec.Body.String())
	}
	if bytes.Contains(legacyPlansRec.Body.Bytes(), []byte(`"points-250"`)) {
		t.Fatalf("expected legacy billing plans to hide subscription placeholder, body=%s", legacyPlansRec.Body.String())
	}

	hiddenOrderReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"points-250","visible_method":"mock"}`))
	hiddenOrderReq.Header.Set("Authorization", "Bearer "+loginExistingAuthUser(t, authSvc, "cashier-plan-direct-user@example.com").AccessToken)
	hiddenOrderReq.Header.Set("Content-Type", "application/json")
	hiddenOrderRec := httptest.NewRecorder()
	handler.ServeHTTP(hiddenOrderRec, hiddenOrderReq)
	if hiddenOrderRec.Code == http.StatusCreated {
		t.Fatalf("expected hidden subscription placeholder order create to fail, body=%s", hiddenOrderRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/cashier/plans/"+jsonInt64(createResp.Data.ID), nil)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected cashier plan delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	var deleteResp struct {
		Data struct {
			PlanCode        string `json:"plan_code"`
			PlanType        string `json:"plan_type"`
			Status          string `json:"status"`
			PurchaseEnabled bool   `json:"purchase_enabled"`
		} `json:"data"`
	}
	if err := json.NewDecoder(deleteRec.Body).Decode(&deleteResp); err != nil {
		t.Fatalf("decode delete response: %v", err)
	}
	if deleteResp.Data.PlanCode != "points-250" || deleteResp.Data.PlanType != "subscription" || deleteResp.Data.Status != "archived" || deleteResp.Data.PurchaseEnabled {
		t.Fatalf("expected deleted plan to be archived and not purchasable, got %#v", deleteResp.Data)
	}
}

func TestAdminCashierProviderInstanceCreateAndUpdate(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-provider-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierProviderTest(t, handler)

	invalidJeePayBody := `{"provider_type":"jeepay_alipay","name":"invalid JeePay","enabled":true,"supported_methods":["alipay"],"config":{"gateway_url":"https://pay.example.com","mch_no":"submitted-merchant","way_code":"ALI_PC"},"secrets":{"key":"submitted-secret"}}`
	invalidJeePayReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(invalidJeePayBody))
	invalidJeePayReq.Header.Set("Authorization", "Bearer "+adminToken)
	invalidJeePayReq.Header.Set("Content-Type", "application/json")
	invalidJeePayRec := httptest.NewRecorder()
	handler.ServeHTTP(invalidJeePayRec, invalidJeePayReq)
	if invalidJeePayRec.Code != http.StatusBadRequest || !bytes.Contains(invalidJeePayRec.Body.Bytes(), []byte(`"code":"PAYMENT_PROVIDER_CONFIG_INVALID"`)) || !bytes.Contains(invalidJeePayRec.Body.Bytes(), []byte("app_id")) {
		t.Fatalf("expected typed JeePay provider validation error, got %d body=%s", invalidJeePayRec.Code, invalidJeePayRec.Body.String())
	}
	if bytes.Contains(invalidJeePayRec.Body.Bytes(), []byte("submitted-merchant")) || bytes.Contains(invalidJeePayRec.Body.Bytes(), []byte("submitted-secret")) {
		t.Fatalf("provider validation error leaked submitted values: %s", invalidJeePayRec.Body.String())
	}

	createBody := `{"provider_type":"alipay_direct","name":"支付宝沙箱主账号","enabled":true,"supported_methods":["alipay"],"sort_order":20,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000","daily_amount_limit_cny":"5000.00000"},"config":{"app_id":"app-123","app_private_key":"super-secret-private-key","alipay_public_key":"public-key","gateway_url":"https://openapi-sandbox.dl.alipaydev.com/gateway.do"}}`
	createReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(createBody))
	createReq.Header.Set("Authorization", "Bearer "+adminToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected provider instance create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if bytes.Contains(createRec.Body.Bytes(), []byte("super-secret-private-key")) {
		t.Fatalf("expected create response to redact provider secrets, body=%s", createRec.Body.String())
	}
	var createResp struct {
		Data struct {
			ID                int64          `json:"id"`
			ProviderType      string         `json:"provider_type"`
			Name              string         `json:"name"`
			Enabled           bool           `json:"enabled"`
			SupportedMethods  []string       `json:"supported_methods"`
			SortOrder         int            `json:"sort_order"`
			SchedulerWeight   int            `json:"scheduler_weight"`
			ConfigStatus      string         `json:"config_status"`
			CredentialsStatus map[string]any `json:"credentials_status"`
			Config            map[string]any `json:"config"`
		} `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create provider response: %v", err)
	}
	if createResp.Data.ID == 0 || createResp.Data.ProviderType != "alipay_direct" || createResp.Data.Name != "支付宝沙箱主账号" || !createResp.Data.Enabled || createResp.Data.SortOrder != 20 || createResp.Data.SchedulerWeight != 100 || createResp.Data.ConfigStatus != "configured" {
		t.Fatalf("unexpected created provider instance %#v", createResp.Data)
	}
	if len(createResp.Data.SupportedMethods) != 1 || createResp.Data.SupportedMethods[0] != "alipay" {
		t.Fatalf("unexpected supported methods %#v", createResp.Data.SupportedMethods)
	}
	if createResp.Data.Config["app_private_key"] != nil {
		t.Fatalf("expected config to omit secret keys, got %#v", createResp.Data.Config)
	}
	if hasSecret, _ := createResp.Data.CredentialsStatus["has_secret"].(bool); !hasSecret {
		t.Fatalf("expected credentials_status.has_secret=true, got %#v", createResp.Data.CredentialsStatus)
	}
	if fingerprint, _ := createResp.Data.CredentialsStatus["fingerprint"].(string); !strings.HasPrefix(fingerprint, "sha256:") {
		t.Fatalf("expected credentials fingerprint, got %#v", createResp.Data.CredentialsStatus)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/provider-instances?page=1&page_size=10", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected provider instance list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	if !bytes.Contains(listRec.Body.Bytes(), []byte(`"alipay_direct"`)) || bytes.Contains(listRec.Body.Bytes(), []byte("super-secret-private-key")) {
		t.Fatalf("expected provider list to include instance without secret, body=%s", listRec.Body.String())
	}

	updateBody := `{"provider_type":"alipay_direct","name":"支付宝沙箱备用账号","enabled":false,"supported_methods":["alipay"],"sort_order":30,"scheduler_weight":50,"limits":{"min_amount_cny":"2.00000","max_amount_cny":"800.00000"},"config":{"app_id":"app-456","app_private_key":"rotated-secret-private-key"}}`
	updateReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/provider-instances/"+jsonInt64(createResp.Data.ID), bytes.NewBufferString(updateBody))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected provider instance update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}
	if bytes.Contains(updateRec.Body.Bytes(), []byte("rotated-secret-private-key")) {
		t.Fatalf("expected update response to redact provider secrets, body=%s", updateRec.Body.String())
	}
	var updateResp struct {
		Data struct {
			ID                int64          `json:"id"`
			Name              string         `json:"name"`
			Enabled           bool           `json:"enabled"`
			SortOrder         int            `json:"sort_order"`
			SchedulerWeight   int            `json:"scheduler_weight"`
			Config            map[string]any `json:"config"`
			CredentialsStatus map[string]any `json:"credentials_status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(updateRec.Body).Decode(&updateResp); err != nil {
		t.Fatalf("decode update provider response: %v", err)
	}
	if updateResp.Data.ID != createResp.Data.ID || updateResp.Data.Name != "支付宝沙箱备用账号" || updateResp.Data.Enabled || updateResp.Data.SortOrder != 30 || updateResp.Data.SchedulerWeight != 50 {
		t.Fatalf("unexpected updated provider instance %#v", updateResp.Data)
	}
	if updateResp.Data.Config["app_private_key"] != nil {
		t.Fatalf("expected updated config to omit secret keys, got %#v", updateResp.Data.Config)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/provider-instances/"+jsonInt64(createResp.Data.ID), nil)
	detailReq.Header.Set("Authorization", "Bearer "+adminToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected provider instance detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	if !bytes.Contains(detailRec.Body.Bytes(), []byte(`"支付宝沙箱备用账号"`)) || bytes.Contains(detailRec.Body.Bytes(), []byte("rotated-secret-private-key")) {
		t.Fatalf("expected provider detail to include update without secret, body=%s", detailRec.Body.String())
	}

	wxpayBody := `{"provider_type":"wxpay_direct","name":"微信支付账号","enabled":true,"supported_methods":["wxpay"],"sort_order":40,"scheduler_weight":80,"config":{"app_id":"wx-app","mch_id":"mch-123","api_v3_key":"wx-api-v3-secret","merchant_private_key":"wx-private-key","merchant_certificate_serial":"serial-123","wechat_pay_public_key":"wx-public-key","wechat_pay_public_key_id":"wx-public-key-id"}}`
	wxpayReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(wxpayBody))
	wxpayReq.Header.Set("Authorization", "Bearer "+adminToken)
	wxpayReq.Header.Set("Content-Type", "application/json")
	wxpayRec := httptest.NewRecorder()
	handler.ServeHTTP(wxpayRec, wxpayReq)
	if wxpayRec.Code != http.StatusCreated {
		t.Fatalf("expected wxpay provider instance create 201, got %d body=%s", wxpayRec.Code, wxpayRec.Body.String())
	}
	if bytes.Contains(wxpayRec.Body.Bytes(), []byte("wx-api-v3-secret")) || bytes.Contains(wxpayRec.Body.Bytes(), []byte("wx-private-key")) {
		t.Fatalf("expected wxpay provider response to redact api_v3_key and private key, body=%s", wxpayRec.Body.String())
	}
	var wxpayResp struct {
		Data struct {
			Config            map[string]any `json:"config"`
			CredentialsStatus map[string]any `json:"credentials_status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(wxpayRec.Body).Decode(&wxpayResp); err != nil {
		t.Fatalf("decode wxpay provider response: %v", err)
	}
	if wxpayResp.Data.Config["api_v3_key"] != nil || wxpayResp.Data.Config["merchant_private_key"] != nil {
		t.Fatalf("expected wxpay secret config keys to be omitted, got %#v", wxpayResp.Data.Config)
	}
	if hasSecret, _ := wxpayResp.Data.CredentialsStatus["has_secret"].(bool); !hasSecret {
		t.Fatalf("expected wxpay credentials_status.has_secret=true, got %#v", wxpayResp.Data.CredentialsStatus)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/ops/admin/v1/cashier/provider-instances/"+jsonInt64(createResp.Data.ID), nil)
	deleteReq.Header.Set("Authorization", "Bearer "+adminToken)
	deleteRec := httptest.NewRecorder()
	handler.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected provider instance delete 200, got %d body=%s", deleteRec.Code, deleteRec.Body.String())
	}
	var deleteResp struct {
		Data struct {
			ID           int64  `json:"id"`
			ProviderType string `json:"provider_type"`
			Name         string `json:"name"`
		} `json:"data"`
	}
	if err := json.NewDecoder(deleteRec.Body).Decode(&deleteResp); err != nil {
		t.Fatalf("decode delete provider response: %v", err)
	}
	if deleteResp.Data.ID != createResp.Data.ID || deleteResp.Data.ProviderType != "alipay_direct" || deleteResp.Data.Name != "支付宝沙箱备用账号" {
		t.Fatalf("unexpected deleted provider instance %#v", deleteResp.Data)
	}
	deletedDetailReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/provider-instances/"+jsonInt64(createResp.Data.ID), nil)
	deletedDetailReq.Header.Set("Authorization", "Bearer "+adminToken)
	deletedDetailRec := httptest.NewRecorder()
	handler.ServeHTTP(deletedDetailRec, deletedDetailReq)
	if deletedDetailRec.Code != http.StatusNotFound {
		t.Fatalf("expected deleted provider instance detail 404, got %d body=%s", deletedDetailRec.Code, deletedDetailRec.Body.String())
	}
}

func newAdminCashierRefundProviderTest(t *testing.T, email string) (http.Handler, string, domainauth.User, domainauth.Session, *billingservice.Service) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
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
	user, userSession, err := loginAuthUserWithPasswordSetup(t, authSvc, email, "123456")
	if err != nil {
		t.Fatalf("LoginWithEmailCode: %v", err)
	}
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithModelAdminService(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil, nil, nil, nil, nil))
	adminToken := loginAdminForCashierTest(t, handler)
	return handler, adminToken, user, userSession, billingSvc
}

func putVisibleMethodsForCashierTest(t *testing.T, handler http.Handler, adminToken string, itemsJSON string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":`+itemsJSON+`}`))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func createProviderInstanceForCashierTest(t *testing.T, handler http.Handler, adminToken string, body string) int64 {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected provider instance create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil || resp.Data.ID <= 0 {
		t.Fatalf("decode provider instance create response: err=%v body=%s", err, rec.Body.String())
	}
	return resp.Data.ID
}

func createCustomCashierOrderForTest(t *testing.T, handler http.Handler, userToken string, visibleMethod string, amountCNY string) (int64, string) {
	t.Helper()
	body := fmt.Sprintf(`{"purchase_type":"custom_amount","amount_cny":%q,"visible_method":%q}`, amountCNY, visibleMethod)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected user cashier order create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data struct {
			ID      int64  `json:"id"`
			OrderNo string `json:"order_no"`
			Status  string `json:"status"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	if resp.Data.ID == 0 || resp.Data.OrderNo == "" || resp.Data.Status != "pending" {
		t.Fatalf("unexpected created order %#v", resp.Data)
	}
	return resp.Data.ID, resp.Data.OrderNo
}

func completeAdminCashierOrderForTest(t *testing.T, handler http.Handler, adminToken string, orderID int64, tradeNo string) {
	t.Helper()
	body := fmt.Sprintf(`{"trade_no":%q,"reason":"confirmed before refund test"}`, tradeNo)
	req := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/orders/"+jsonInt64(orderID)+"/complete", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+adminToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected admin complete 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func assertAdminCashierRefundedForTest(t *testing.T, body string, orderID int64, refundTradeNo string) {
	t.Helper()
	var resp struct {
		Data struct {
			ID            int64  `json:"id"`
			Status        string `json:"status"`
			RefundTradeNo string `json:"refund_trade_no"`
			RefundedAt    string `json:"refunded_at"`
		} `json:"data"`
	}
	if err := json.NewDecoder(strings.NewReader(body)).Decode(&resp); err != nil {
		t.Fatalf("decode refund response: %v body=%s", err, body)
	}
	if resp.Data.ID != orderID || resp.Data.Status != "refunded" || resp.Data.RefundTradeNo != refundTradeNo || resp.Data.RefundedAt == "" {
		t.Fatalf("unexpected refund response %#v body=%s", resp.Data, body)
	}
}

func loginAdminForCashierTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"cashier-admin@example.com","password":"password"}`))
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
		t.Fatalf("decode login response: %v", err)
	}
	return loginResp.Data.AccessToken
}

func loginAdminForCashierProviderTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"cashier-provider-admin@example.com","password":"password"}`))
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
		t.Fatalf("decode login response: %v", err)
	}
	return loginResp.Data.AccessToken
}

func loginAdminForCashierConfigTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"cashier-config-admin@example.com","password":"password"}`))
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
		t.Fatalf("decode login response: %v", err)
	}
	return loginResp.Data.AccessToken
}

func loginAdminForCashierPlanTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"cashier-plan-admin@example.com","password":"password"}`))
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
		t.Fatalf("decode login response: %v", err)
	}
	return loginResp.Data.AccessToken
}
