package router

import (
	"bytes"
	"context"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadminauth "github.com/fatballfish/pic-gallery/internal/domain/adminauth"
	domainadminconfig "github.com/fatballfish/pic-gallery/internal/domain/adminconfig"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	adminauthservice "github.com/fatballfish/pic-gallery/internal/service/adminauth"
	adminconfigservice "github.com/fatballfish/pic-gallery/internal/service/adminconfig"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	stripe "github.com/stripe/stripe-go/v85"
	"github.com/stripe/stripe-go/v85/webhook"
)

func TestCashierMockPaymentCreditsRechargeBucket(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, billingSvc))

	optionsReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/options", nil)
	optionsReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	optionsRec := httptest.NewRecorder()
	handler.ServeHTTP(optionsRec, optionsReq)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("options expected 200, got %d body=%s", optionsRec.Code, optionsRec.Body.String())
	}
	var optionsResp struct {
		Data struct {
			Plans []struct {
				PlanCode string `json:"plan_code"`
			} `json:"plans"`
			VisibleMethods []struct {
				Method string `json:"method"`
			} `json:"visible_methods"`
		} `json:"data"`
	}
	if err := json.NewDecoder(optionsRec.Body).Decode(&optionsResp); err != nil {
		t.Fatalf("decode options: %v", err)
	}
	if len(optionsResp.Data.Plans) == 0 || len(optionsResp.Data.VisibleMethods) == 0 || optionsResp.Data.VisibleMethods[0].Method != "mock" {
		t.Fatalf("expected mock cashier options, got %#v", optionsResp.Data)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create order expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create order: %v", err)
	}
	if createResp.Data.ID == 0 || createResp.Data.Status != "pending" || createResp.Data.Provider != "mock" {
		t.Fatalf("unexpected created cashier order %#v", createResp.Data)
	}
	if createResp.Data.PaymentURL != "" || createResp.Data.PaymentDisplay["type"] != "mock" || createResp.Data.PaymentDisplay["payment_url"] != nil {
		t.Fatalf("expected mock cashier order to use in-page mock action without legacy payment_url, got order=%#v display=%#v", createResp.Data, createResp.Data.PaymentDisplay)
	}

	mockPayReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(createResp.Data.ID)+"/mock-pay", nil)
	mockPayReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	mockPayRec := httptest.NewRecorder()
	handler.ServeHTTP(mockPayRec, mockPayReq)
	if mockPayRec.Code != http.StatusOK {
		t.Fatalf("mock pay expected 200, got %d body=%s", mockPayRec.Code, mockPayRec.Body.String())
	}
	var mockPayResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(mockPayRec.Body).Decode(&mockPayResp); err != nil {
		t.Fatalf("decode mock pay: %v", err)
	}
	if mockPayResp.Data.Status != "completed" || mockPayResp.Data.TradeNo == "" || mockPayResp.Data.PaidAt == nil || mockPayResp.Data.CompletedAt == nil || mockPayResp.Data.LedgerID == 0 {
		t.Fatalf("expected completed mock order, got %#v", mockPayResp.Data)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/orders/"+jsonInt64(createResp.Data.ID), nil)
	detailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("order detail expected 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode order detail: %v", err)
	}
	if detailResp.Data.Status != "completed" || detailResp.Data.PaidAt == nil || detailResp.Data.CompletedAt == nil || detailResp.Data.LedgerID != mockPayResp.Data.LedgerID {
		t.Fatalf("expected completed order detail, got %#v", detailResp.Data)
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("balance expected 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if balanceResp.Data.RechargePoints != "100.00000" || len(balanceResp.Data.Buckets) == 0 || balanceResp.Data.Buckets[0].Bucket != "recharge" {
		t.Fatalf("expected recharge bucket after mock pay, got %#v", balanceResp.Data)
	}

	secondMockPayReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(createResp.Data.ID)+"/mock-pay", nil)
	secondMockPayReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	secondMockPayRec := httptest.NewRecorder()
	handler.ServeHTTP(secondMockPayRec, secondMockPayReq)
	if secondMockPayRec.Code != http.StatusOK {
		t.Fatalf("second mock pay expected 200, got %d body=%s", secondMockPayRec.Code, secondMockPayRec.Body.String())
	}
	var secondMockPayResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(secondMockPayRec.Body).Decode(&secondMockPayResp); err != nil {
		t.Fatalf("decode second mock pay: %v", err)
	}
	if secondMockPayResp.Data.Status != "completed" || secondMockPayResp.Data.LedgerID != mockPayResp.Data.LedgerID {
		t.Fatalf("expected idempotent completed mock order, got %#v", secondMockPayResp.Data)
	}
	afterSecondPayReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	afterSecondPayReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	afterSecondPayRec := httptest.NewRecorder()
	handler.ServeHTTP(afterSecondPayRec, afterSecondPayReq)
	if afterSecondPayRec.Code != http.StatusOK {
		t.Fatalf("balance after second mock pay expected 200, got %d body=%s", afterSecondPayRec.Code, afterSecondPayRec.Body.String())
	}
	var afterSecondPayResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if err := json.NewDecoder(afterSecondPayRec.Body).Decode(&afterSecondPayResp); err != nil {
		t.Fatalf("decode balance after second mock pay: %v", err)
	}
	if afterSecondPayResp.Data.RechargePoints != "100.00000" || afterSecondPayResp.Data.AvailablePoints != "100.00000" {
		t.Fatalf("expected idempotent mock pay to keep balance unchanged, got %#v", afterSecondPayResp.Data)
	}

	customReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"31.25000","visible_method":"mock"}`))
	customReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	customReq.Header.Set("Content-Type", "application/json")
	customRec := httptest.NewRecorder()
	handler.ServeHTTP(customRec, customReq)
	if customRec.Code != http.StatusCreated {
		t.Fatalf("create custom amount order expected 201, got %d body=%s", customRec.Code, customRec.Body.String())
	}
	var customResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(customRec.Body).Decode(&customResp); err != nil {
		t.Fatalf("decode custom order: %v", err)
	}
	if customResp.Data.AmountCNY != "31.25000" || customResp.Data.Points != "100.00000" || customResp.Data.Provider != "mock" {
		t.Fatalf("unexpected custom amount order %#v", customResp.Data)
	}

	customPayReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(customResp.Data.ID)+"/mock-pay", nil)
	customPayReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	customPayRec := httptest.NewRecorder()
	handler.ServeHTTP(customPayRec, customPayReq)
	if customPayRec.Code != http.StatusOK {
		t.Fatalf("custom mock pay expected 200, got %d body=%s", customPayRec.Code, customPayRec.Body.String())
	}

	finalBalanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	finalBalanceReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	finalBalanceRec := httptest.NewRecorder()
	handler.ServeHTTP(finalBalanceRec, finalBalanceReq)
	if finalBalanceRec.Code != http.StatusOK {
		t.Fatalf("final balance expected 200, got %d body=%s", finalBalanceRec.Code, finalBalanceRec.Body.String())
	}
	var finalBalanceResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if err := json.NewDecoder(finalBalanceRec.Body).Decode(&finalBalanceResp); err != nil {
		t.Fatalf("decode final balance: %v", err)
	}
	if finalBalanceResp.Data.RechargePoints != "200.00000" || finalBalanceResp.Data.AvailablePoints != "200.00000" {
		t.Fatalf("expected custom amount recharge to add another 100 points, got %#v", finalBalanceResp.Data)
	}
}

func TestCashierCreateOrderReusesIdempotencyKey(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Cashier.MaxPendingOrdersPerUser = 1
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-idempotent@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, billingSvc))

	create := func() domainbilling.PaymentOrder {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
		req.Header.Set("Authorization", "Bearer "+session.AccessToken)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "cashier-create-once")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create order expected 201, got %d body=%s", rec.Code, rec.Body.String())
		}
		var resp struct {
			Data domainbilling.PaymentOrder `json:"data"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode create order: %v", err)
		}
		return resp.Data
	}

	first := create()
	second := create()
	if second.ID != first.ID || second.OrderNo != first.OrderNo {
		t.Fatalf("expected idempotency replay to reuse order %d/%s, got %#v", first.ID, first.OrderNo, second)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/orders", nil)
	listReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list orders expected 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Data domainbilling.PaymentOrderPage `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode order list: %v", err)
	}
	if listResp.Data.Total != 1 {
		t.Fatalf("expected idempotency replay to keep one order, got total=%d items=%#v", listResp.Data.Total, listResp.Data.Items)
	}
}

func TestCashierPendingOrderLimitUsesAdminConfig(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-limit-admin-config@example.com")
	adminCfgSvc := adminconfigservice.NewService(cfg)
	if _, err := adminCfgSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  "payments",
		Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "payments",
			ConfigKey:      "max_pending_orders_per_user",
			ConfigValue:    map[string]any{"value": 1},
			Scope:          "global",
		}},
	}); err != nil {
		t.Fatalf("UpdateTab payments: %v", err)
	}
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, adminCfgSvc, billingservice.NewService(cfg.Billing)))

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("first create expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	secondReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusConflict {
		t.Fatalf("second create expected admin-configured pending limit 409, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
}

func TestCashierOptionsUsesAdminConfiguredOrderTimeout(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Cashier.OrderTimeoutSeconds = 1800
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-timeout-admin-config@example.com")
	adminCfgSvc := adminconfigservice.NewService(cfg)
	if _, err := adminCfgSvc.UpdateTab(context.Background(), domainadminconfig.UpdateTabRequest{
		TabKey:  "payments",
		Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "payments",
			ConfigKey:      "order_timeout_seconds",
			ConfigValue:    map[string]any{"value": 900},
			Scope:          "global",
		}},
	}); err != nil {
		t.Fatalf("UpdateTab payments: %v", err)
	}
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, adminCfgSvc, billingservice.NewService(cfg.Billing)))

	optionsReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/options", nil)
	optionsReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	optionsRec := httptest.NewRecorder()
	handler.ServeHTTP(optionsRec, optionsReq)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("options expected 200, got %d body=%s", optionsRec.Code, optionsRec.Body.String())
	}
	var optionsResp struct {
		Data struct {
			OrderTimeoutSeconds int `json:"order_timeout_seconds"`
		} `json:"data"`
	}
	if err := json.NewDecoder(optionsRec.Body).Decode(&optionsResp); err != nil {
		t.Fatalf("decode options: %v", err)
	}
	if optionsResp.Data.OrderTimeoutSeconds != 900 {
		t.Fatalf("expected admin configured order timeout 900, got %d", optionsResp.Data.OrderTimeoutSeconds)
	}
}

func TestCashierCancelPendingOrderUsesCanceledStatusAndBlocksMockPay(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-cancel@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, billingSvc))

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create order expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create order: %v", err)
	}
	if createResp.Data.Status != "pending" {
		t.Fatalf("expected pending created order, got %#v", createResp.Data)
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(createResp.Data.ID)+"/cancel", nil)
	cancelReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	cancelRec := httptest.NewRecorder()
	handler.ServeHTTP(cancelRec, cancelReq)
	if cancelRec.Code != http.StatusOK {
		t.Fatalf("cancel order expected 200, got %d body=%s", cancelRec.Code, cancelRec.Body.String())
	}
	var cancelResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(cancelRec.Body).Decode(&cancelResp); err != nil {
		t.Fatalf("decode cancel order: %v", err)
	}
	if cancelResp.Data.Status != "canceled" || cancelResp.Data.ClosedAt == nil {
		t.Fatalf("expected canceled order with closed_at, got %#v", cancelResp.Data)
	}

	mockPayReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(createResp.Data.ID)+"/mock-pay", nil)
	mockPayReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	mockPayRec := httptest.NewRecorder()
	handler.ServeHTTP(mockPayRec, mockPayReq)
	if mockPayRec.Code != http.StatusConflict {
		t.Fatalf("mock pay after cancel expected 409, got %d body=%s", mockPayRec.Code, mockPayRec.Body.String())
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/orders/"+jsonInt64(createResp.Data.ID), nil)
	detailReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("detail after cancel expected 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode detail after cancel: %v", err)
	}
	if detailResp.Data.Status != "canceled" || detailResp.Data.LedgerID != 0 || detailResp.Data.CompletedAt != nil {
		t.Fatalf("expected canceled order to remain unpaid, got %#v", detailResp.Data)
	}
}

func TestCashierWebhookCompletesRechargeOrderIdempotently(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-webhook@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, billingSvc))

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create order expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode create order: %v", err)
	}

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/mock", bytes.NewBufferString(`{"order_no":"`+createResp.Data.OrderNo+`","trade_no":"MOCK-WEBHOOK-001"}`))
	webhookReq.Header.Set("Content-Type", "application/json")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("webhook expected 200, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	var webhookResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(webhookRec.Body).Decode(&webhookResp); err != nil {
		t.Fatalf("decode webhook response: %v", err)
	}
	if webhookResp.Data.Status != "completed" || webhookResp.Data.PaidAt == nil || webhookResp.Data.CompletedAt == nil || webhookResp.Data.LedgerID == 0 {
		t.Fatalf("expected completed webhook order, got %#v", webhookResp.Data)
	}

	secondWebhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/mock", bytes.NewBufferString(`{"order_no":"`+createResp.Data.OrderNo+`","trade_no":"MOCK-WEBHOOK-001"}`))
	secondWebhookReq.Header.Set("Content-Type", "application/json")
	secondWebhookRec := httptest.NewRecorder()
	handler.ServeHTTP(secondWebhookRec, secondWebhookReq)
	if secondWebhookRec.Code != http.StatusOK {
		t.Fatalf("second webhook expected 200, got %d body=%s", secondWebhookRec.Code, secondWebhookRec.Body.String())
	}
	var secondWebhookResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(secondWebhookRec.Body).Decode(&secondWebhookResp); err != nil {
		t.Fatalf("decode second webhook response: %v", err)
	}
	if secondWebhookResp.Data.Status != "completed" || secondWebhookResp.Data.LedgerID != webhookResp.Data.LedgerID {
		t.Fatalf("expected idempotent completed webhook order, got %#v", secondWebhookResp.Data)
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("balance expected 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if balanceResp.Data.RechargePoints != "100.00000" || balanceResp.Data.AvailablePoints != "100.00000" {
		t.Fatalf("expected webhook to credit recharge bucket once, got %#v", balanceResp.Data)
	}
}

func TestCashierJeePayDisplayIsSignedAndPersisted(t *testing.T) {
	handler, userToken, _ := setupJeePayCashierTest(t, "cashier-jeepay-display-user@example.com")
	order := createJeePayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	if order.Provider != "jeepay_alipay" || order.ProviderType != "jeepay_alipay" || order.ProviderInstanceID == 0 || order.PaymentURL == "" {
		t.Fatalf("expected jeepay order provider metadata and payment url, got %#v", order)
	}
	payURL, err := url.Parse(order.PaymentURL)
	if err != nil {
		t.Fatalf("parse jeepay payment url: %v", err)
	}
	query := payURL.Query()
	if !strings.HasSuffix(payURL.Path, "/api/pay/unifiedOrder") {
		t.Fatalf("expected jeepay unifiedOrder URL, got %s", order.PaymentURL)
	}
	if query.Get("mchNo") != "MCH10001" || query.Get("appId") != "APP10001" || query.Get("wayCode") != "ALI_PC" || query.Get("mchOrderNo") != order.OrderNo || query.Get("amount") != "1250" {
		t.Fatalf("unexpected jeepay payment params: %s", order.PaymentURL)
	}
	if query.Get("notifyUrl") == "" || query.Get("returnUrl") == "" || query.Get("sign") == "" || query.Get("signType") != "MD5" {
		t.Fatalf("expected jeepay callbacks and MD5 signature, got %s", order.PaymentURL)
	}
	if order.PaymentDisplay["type"] != "redirect" || order.PaymentDisplay["payment_url"] != order.PaymentURL || order.PaymentDisplay["sign_type"] != "MD5" || order.PaymentDisplay["way_code"] != "ALI_PC" {
		t.Fatalf("expected signed jeepay display to mirror payment url, got %#v", order.PaymentDisplay)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/orders/"+jsonInt64(order.ID), nil)
	detailReq.Header.Set("Authorization", "Bearer "+userToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected order detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode order detail: %v", err)
	}
	if detailResp.Data.PaymentURL != order.PaymentURL || detailResp.Data.PaymentDisplay["payment_url"] != order.PaymentURL {
		t.Fatalf("expected persisted jeepay payment display, create=%#v detail=%#v", order, detailResp.Data)
	}
}

func TestCashierStripeOrderCreatesPaymentIntentAndPersistsNarrowDisplay(t *testing.T) {
	var upstreamAuthorization string
	var upstreamIdempotencyKey string
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/payment_intents" {
			t.Fatalf("unexpected Stripe request %s %s", r.Method, r.URL.Path)
		}
		upstreamAuthorization = r.Header.Get("Authorization")
		upstreamIdempotencyKey = r.Header.Get("Idempotency-Key")
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse Stripe form: %v", err)
		}
		upstreamValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"pi_route_123","object":"payment_intent","amount":1025,"currency":"cny","client_secret":"pi_route_123_secret_client","status":"requires_payment_method","metadata":{"order_no":"route-order"}}`))
	}))
	defer upstream.Close()

	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(upstream.URL), MaxNetworkRetries: stripe.Int64(0)}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

	handler, userToken := setupStripeCashierTest(t, "cashier-stripe-order-user@example.com")
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"10.25","visible_method":"stripe"}`))
	createReq.Header.Set("Authorization", "Bearer "+userToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected Stripe order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode Stripe order: %v", err)
	}
	order := createResp.Data
	if order.ProviderType != "stripe" || order.ClientToken != "pi_route_123" {
		t.Fatalf("expected persisted Stripe provider and PaymentIntent ID, got %#v", order)
	}
	if len(order.PaymentDisplay) != 3 || order.PaymentDisplay["type"] != "stripe_payment_element" || order.PaymentDisplay["client_secret"] != "pi_route_123_secret_client" || order.PaymentDisplay["publishable_key"] != "pk_test_route" {
		t.Fatalf("unexpected Stripe payment display %#v", order.PaymentDisplay)
	}
	if upstreamAuthorization != "Bearer sk_test_route" || upstreamIdempotencyKey != order.OrderNo {
		t.Fatalf("unexpected Stripe authorization or idempotency headers auth=%q idempotency=%q order=%q", upstreamAuthorization, upstreamIdempotencyKey, order.OrderNo)
	}
	if upstreamValues.Get("amount") != "1025" || upstreamValues.Get("currency") != "cny" || upstreamValues.Get("metadata[order_no]") != order.OrderNo {
		t.Fatalf("unexpected Stripe PaymentIntent params %#v", upstreamValues)
	}
	for _, secret := range []string{"sk_test_route", "whsec_route"} {
		if bytes.Contains(createRec.Body.Bytes(), []byte(secret)) {
			t.Fatalf("Stripe order response leaked secret %q", secret)
		}
	}
}

func TestCashierStripeWebhookVerifiesExactBodyAndCreditsOnce(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"pi_webhook_route","object":"payment_intent","amount":1025,"currency":"cny","client_secret":"pi_webhook_route_secret_client","status":"requires_payment_method"}`))
	}))
	defer upstream.Close()
	originalBackend := stripe.GetBackend(stripe.APIBackend)
	stripe.SetBackend(stripe.APIBackend, stripe.GetBackendWithConfig(stripe.APIBackend, &stripe.BackendConfig{URL: stripe.String(upstream.URL), MaxNetworkRetries: stripe.Int64(0)}))
	t.Cleanup(func() { stripe.SetBackend(stripe.APIBackend, originalBackend) })

	handler, userToken := setupStripeCashierTest(t, "cashier-stripe-webhook-user@example.com")
	order := createStripeCustomAmountOrderForWebhookTest(t, handler, userToken, "10.25")
	payload := func(eventID, eventType string, amountFen int64) []byte {
		return []byte(fmt.Sprintf(`{"id":%q,"object":"event","api_version":%q,"type":%q,"data":{"object":{"id":"pi_webhook_route","object":"payment_intent","amount":%d,"currency":"cny","metadata":{"order_no":%q},"status":"succeeded"}}}`, eventID, stripe.APIVersion, eventType, amountFen, order.OrderNo))
	}
	send := func(body []byte, signature string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/stripe", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Stripe-Signature", signature)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}
	sign := func(body []byte) string {
		return webhook.GenerateTestSignedPayload(&webhook.UnsignedPayload{Payload: body, Secret: "whsec_route"}).Header
	}

	tamperedOriginal := payload("evt_tampered", "payment_intent.succeeded", 1025)
	tampered := append([]byte(nil), tamperedOriginal...)
	tampered[len(tampered)-2] = 'x'
	if rec := send(tampered, sign(tamperedOriginal)); rec.Code != http.StatusBadRequest || !bytes.Contains(rec.Body.Bytes(), []byte("PAYMENT_SIGNATURE_INVALID")) {
		t.Fatalf("expected tampered Stripe body to fail signature verification, got %d body=%s", rec.Code, rec.Body.String())
	}

	unrelated := payload("evt_unrelated", "customer.created", 1025)
	if rec := send(unrelated, sign(unrelated)); rec.Code != http.StatusOK {
		t.Fatalf("expected unrelated Stripe event 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	failed := payload("evt_failed", "payment_intent.payment_failed", 1025)
	if rec := send(failed, sign(failed)); rec.Code != http.StatusOK {
		t.Fatalf("expected failed Stripe event acknowledgement, got %d body=%s", rec.Code, rec.Body.String())
	}
	if pending := getCashierOrderForTest(t, handler, userToken, order.ID); pending.Status != "pending" || pending.LedgerID != 0 {
		t.Fatalf("non-success Stripe events mutated order %#v", pending)
	}

	mismatch := payload("evt_mismatch", "payment_intent.succeeded", 1000)
	if rec := send(mismatch, sign(mismatch)); rec.Code != http.StatusConflict || !bytes.Contains(rec.Body.Bytes(), []byte("PAYMENT_AMOUNT_MISMATCH")) {
		t.Fatalf("expected Stripe amount mismatch 409, got %d body=%s", rec.Code, rec.Body.String())
	}

	success := payload("evt_success", "payment_intent.succeeded", 1025)
	if rec := send(success, sign(success)); rec.Code != http.StatusOK {
		t.Fatalf("expected Stripe success webhook 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	completed := getCashierOrderForTest(t, handler, userToken, order.ID)
	if completed.Status != "completed" || completed.TradeNo != "pi_webhook_route" || completed.LedgerID == 0 {
		t.Fatalf("expected completed Stripe order, got %#v", completed)
	}
	if rec := send(success, sign(success)); rec.Code != http.StatusOK {
		t.Fatalf("expected duplicate Stripe webhook 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	duplicate := getCashierOrderForTest(t, handler, userToken, order.ID)
	if duplicate.LedgerID != completed.LedgerID || duplicate.TradeNo != completed.TradeNo {
		t.Fatalf("duplicate Stripe event was not idempotent: first=%#v second=%#v", completed, duplicate)
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+userToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	var balanceResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if balanceRec.Code != http.StatusOK || json.NewDecoder(balanceRec.Body).Decode(&balanceResp) != nil {
		t.Fatalf("load Stripe balance: status=%d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	if balanceResp.Data.RechargePoints != order.Points || balanceResp.Data.AvailablePoints != order.Points {
		t.Fatalf("expected Stripe webhook to credit once, got %#v", balanceResp.Data)
	}
}

func TestCashierJeePayAPIModePostsUnifiedOrderAndPersistsDisplay(t *testing.T) {
	var upstreamPath string
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("expected jeepay unified order POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/pay/unifiedOrder" {
			t.Fatalf("unexpected jeepay api path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse jeepay unified order form: %v", err)
		}
		upstreamValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"SUCCESS","data":{"payOrderId":"JEEPAY-API-PAY-001","payUrl":"https://jeepay.example.com/pay/session","codeUrl":"https://jeepay.example.com/qr/session","payData":"weixin://wxpay/bizpayurl?pr=jeepay-session"}}`))
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
	userSession := loginExistingAuthUser(t, authSvc, "cashier-jeepay-api-user@example.com")
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
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"jeepay_alipay","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := fmt.Sprintf(`{"provider_type":"jeepay_alipay","name":"JeePay 支付宝 API","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"gateway_url":%q,"mch_no":"MCH10001","app_id":"APP10001","key":"merchant-secret","notify_url":"https://merchant.example.com/api/payments/jeepay/notify","return_url":"https://merchant.example.com/checkout/return","payment_mode":"api","way_code":"ALI_PC","client_ip":"127.0.0.1"}}`, upstream.URL)
	providerID := createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"12.50000","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected jeepay api order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	if createResp.Data.ProviderType != "jeepay_alipay" || createResp.Data.ProviderInstanceID != providerID {
		t.Fatalf("expected jeepay api provider metadata, got %#v", createResp.Data)
	}
	if upstreamPath != "/api/pay/unifiedOrder" {
		t.Fatalf("expected jeepay unified order path, got %q", upstreamPath)
	}
	if upstreamValues.Get("mchNo") != "MCH10001" || upstreamValues.Get("appId") != "APP10001" || upstreamValues.Get("mchOrderNo") != createResp.Data.OrderNo || upstreamValues.Get("wayCode") != "ALI_PC" || upstreamValues.Get("amount") != "1250" || upstreamValues.Get("clientIp") != "127.0.0.1" || upstreamValues.Get("sign") == "" || upstreamValues.Get("signType") != "MD5" {
		t.Fatalf("unexpected jeepay unified order params: %#v", upstreamValues)
	}
	if createResp.Data.PaymentURL != "https://jeepay.example.com/pay/session" || createResp.Data.QRCode != "https://jeepay.example.com/qr/session" {
		t.Fatalf("expected jeepay api display fields, got %#v", createResp.Data)
	}
	if createResp.Data.PaymentDisplay["type"] != "qr_code" || createResp.Data.PaymentDisplay["payment_url"] != createResp.Data.PaymentURL || createResp.Data.PaymentDisplay["qr_code"] != createResp.Data.QRCode || createResp.Data.PaymentDisplay["prepay_mode"] != "api" || createResp.Data.PaymentDisplay["channel_trade_no"] != "JEEPAY-API-PAY-001" {
		t.Fatalf("expected jeepay api display to mirror payment fields, got %#v", createResp.Data.PaymentDisplay)
	}
}

func TestCashierJeePayAPIModeSerializesStructuredChannelExtra(t *testing.T) {
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse jeepay unified order form: %v", err)
		}
		upstreamValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"codeUrl":"https://jeepay.example.com/qr/jsapi","payOrderId":"JEEPAY-JSAPI-001"}}`))
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
	userSession := loginExistingAuthUser(t, authSvc, "cashier-jeepay-api-channel-extra-user@example.com")
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
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"wxpay","label":"微信支付","enabled":true,"source_provider_type":"jeepay_wxpay","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := fmt.Sprintf(`{"provider_type":"jeepay_wxpay","name":"JeePay 微信 JSAPI","enabled":true,"supported_methods":["wxpay"],"sort_order":10,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"gateway_url":%q,"mch_no":"MCH10001","app_id":"APP10001","key":"merchant-secret","payment_mode":"api","way_code":"WX_JSAPI","client_ip":"127.0.0.1","channel_extra":{"openid":"wx-openid-001","subAppId":"wx-sub-app"}}}`, upstream.URL)
	createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"12.50000","visible_method":"wxpay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected jeepay jsapi order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if upstreamValues.Get("wayCode") != "WX_JSAPI" || upstreamValues.Get("channelExtra") == "" || upstreamValues.Get("sign") == "" {
		t.Fatalf("expected jeepay jsapi params to include wayCode, channelExtra and sign, got %#v", upstreamValues)
	}
	var channelExtra map[string]string
	if err := json.Unmarshal([]byte(upstreamValues.Get("channelExtra")), &channelExtra); err != nil {
		t.Fatalf("expected channelExtra to be JSON object, got %q: %v", upstreamValues.Get("channelExtra"), err)
	}
	if channelExtra["openid"] != "wx-openid-001" || channelExtra["subAppId"] != "wx-sub-app" {
		t.Fatalf("unexpected channelExtra JSON: %#v", channelExtra)
	}
	if got, want := upstreamValues.Get("sign"), jeepaySignForTest(upstreamValues, "merchant-secret"); got != want {
		t.Fatalf("expected channelExtra to participate in signature, got %s want %s", got, want)
	}
}

func TestCashierJeePayWebhookRejectsInvalidSignature(t *testing.T) {
	handler, userToken, _ := setupJeePayCashierTest(t, "cashier-jeepay-invalid-sign-user@example.com")
	order := createJeePayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	values := jeepayWebhookValuesForTest(order, "MCH10001", "merchant-secret", "1250", "jeepay-trade-invalid-sign")
	values.Set("sign", "invalid-signature")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/jeepay_alipay", strings.NewReader(values.Encode()))
	webhookReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid jeepay signature 400, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if !bytes.Contains(webhookRec.Body.Bytes(), []byte("PAYMENT_SIGNATURE_INVALID")) {
		t.Fatalf("expected PAYMENT_SIGNATURE_INVALID, body=%s", webhookRec.Body.String())
	}
}

func TestCashierJeePayWebhookRejectsAmountMismatch(t *testing.T) {
	handler, userToken, _ := setupJeePayCashierTest(t, "cashier-jeepay-amount-mismatch-user@example.com")
	order := createJeePayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	values := jeepayWebhookValuesForTest(order, "MCH10001", "merchant-secret", "1000", "jeepay-trade-amount-mismatch")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/jeepay_alipay", strings.NewReader(values.Encode()))
	webhookReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusConflict {
		t.Fatalf("expected jeepay amount mismatch 409, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if !bytes.Contains(webhookRec.Body.Bytes(), []byte("PAYMENT_AMOUNT_MISMATCH")) {
		t.Fatalf("expected PAYMENT_AMOUNT_MISMATCH, body=%s", webhookRec.Body.String())
	}
}

func TestCashierJeePayWebhookCompletesRechargeOrderIdempotently(t *testing.T) {
	handler, userToken, _ := setupJeePayCashierTest(t, "cashier-jeepay-success-user@example.com")
	order := createJeePayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	values := jeepayWebhookValuesForTest(order, "MCH10001", "merchant-secret", "1250", "jeepay-trade-success")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/jeepay_alipay", strings.NewReader(values.Encode()))
	webhookReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected jeepay webhook 200, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if strings.TrimSpace(webhookRec.Body.String()) != "success" {
		t.Fatalf("expected raw jeepay success response, got body=%s", webhookRec.Body.String())
	}
	completed := getCashierOrderForTest(t, handler, userToken, order.ID)
	if completed.Status != "completed" || completed.LedgerID == 0 || completed.TradeNo != "jeepay-trade-success" {
		t.Fatalf("expected completed jeepay recharge order, got %#v", completed)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/jeepay_alipay", strings.NewReader(values.Encode()))
	secondReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second jeepay webhook 200, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if strings.TrimSpace(secondRec.Body.String()) != "success" {
		t.Fatalf("expected raw idempotent jeepay success response, got body=%s", secondRec.Body.String())
	}
	secondCompleted := getCashierOrderForTest(t, handler, userToken, order.ID)
	if secondCompleted.Status != "completed" || secondCompleted.LedgerID != completed.LedgerID {
		t.Fatalf("expected idempotent jeepay webhook order, got %#v", secondCompleted)
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+userToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("balance expected 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if balanceResp.Data.RechargePoints != order.Points || balanceResp.Data.AvailablePoints != order.Points {
		t.Fatalf("expected jeepay webhook to credit recharge bucket once, got %#v", balanceResp.Data)
	}
}

func TestCashierOrderSchedulesConfiguredProviderInstance(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, "cashier-provider-user@example.com")
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
	billingSvc := billingservice.NewService(cfg.Billing)
	api := handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil)
	handler := NewWithAPI(api)
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"alipay_direct","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := alipayProviderBodyForTest(t, "支付宝沙箱主账号", "app-123", true, 10)
	providerReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/cashier/provider-instances", bytes.NewBufferString(providerBody))
	providerReq.Header.Set("Authorization", "Bearer "+adminToken)
	providerReq.Header.Set("Content-Type", "application/json")
	providerRec := httptest.NewRecorder()
	handler.ServeHTTP(providerRec, providerReq)
	if providerRec.Code != http.StatusCreated {
		t.Fatalf("expected provider instance create 201, got %d body=%s", providerRec.Code, providerRec.Body.String())
	}
	var providerResp struct {
		Data struct {
			ID int64 `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(providerRec.Body).Decode(&providerResp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected scheduled order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	if createResp.Data.Provider != "alipay_direct" || createResp.Data.VisibleMethod != "alipay" || createResp.Data.ProviderType != "alipay_direct" || createResp.Data.ProviderInstanceID != providerResp.Data.ID {
		t.Fatalf("expected order to include selected provider instance, got %#v", createResp.Data)
	}
	if createResp.Data.PaymentURL == "" || createResp.Data.PaymentDisplay["payment_url"] == "" {
		t.Fatalf("expected alipay order to include payment url display, got %#v", createResp.Data)
	}
	payURL, err := url.Parse(createResp.Data.PaymentURL)
	if err != nil {
		t.Fatalf("parse alipay payment url: %v", err)
	}
	query := payURL.Query()
	if payURL.Scheme != "https" || payURL.Host != "openapi.alipaydev.com" || query.Get("app_id") != "app-123" || query.Get("out_trade_no") != createResp.Data.OrderNo || query.Get("total_amount") != createResp.Data.AmountCNY {
		t.Fatalf("unexpected alipay payment url %s", createResp.Data.PaymentURL)
	}
	if query.Get("notify_url") == "" || query.Get("return_url") == "" || !strings.Contains(query.Get("biz_content"), createResp.Data.OrderNo) {
		t.Fatalf("expected alipay url to carry callback and order payload, got %s", createResp.Data.PaymentURL)
	}
	if query.Get("sign_type") != "RSA2" || query.Get("sign") == "" || createResp.Data.PaymentDisplay["signed"] != true {
		t.Fatalf("expected alipay url to be RSA2 signed, url=%s display=%#v", createResp.Data.PaymentURL, createResp.Data.PaymentDisplay)
	}

	disabledProviderBody := alipayProviderBodyForTest(t, "支付宝沙箱主账号", "app-123", false, 10)
	updateReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/provider-instances/"+jsonInt64(providerResp.Data.ID), bytes.NewBufferString(disabledProviderBody))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected provider disable 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	noProviderReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"alipay"}`))
	noProviderReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	noProviderReq.Header.Set("Content-Type", "application/json")
	noProviderRec := httptest.NewRecorder()
	handler.ServeHTTP(noProviderRec, noProviderReq)
	if noProviderRec.Code != http.StatusConflict {
		t.Fatalf("expected no provider instance 400, got %d body=%s", noProviderRec.Code, noProviderRec.Body.String())
	}
	if !bytes.Contains(noProviderRec.Body.Bytes(), []byte("PAYMENT_PROVIDER_UNAVAILABLE")) {
		t.Fatalf("expected PAYMENT_PROVIDER_UNAVAILABLE error, body=%s", noProviderRec.Body.String())
	}
	if !bytes.Contains(noProviderRec.Body.Bytes(), []byte("payment provider instance is unavailable")) {
		t.Fatalf("expected clear unavailable provider error, body=%s", noProviderRec.Body.String())
	}
}

func TestCashierOrdersListAndClientReturnURL(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, "cashier-return-url-user@example.com")
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-return-url-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierEmail(t, handler, "cashier-return-url-admin@example.com")

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"alipay_direct","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	createProviderInstanceForCashierTest(t, handler, adminToken, alipayProviderBodyForTest(t, "支付宝回跳测试账号", "app-return", true, 10))

	returnURL := "https://app.example.com/#/checkout"
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"alipay","client_return_url":"`+returnURL+`"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected scheduled order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	payURL, err := url.Parse(createResp.Data.PaymentURL)
	if err != nil {
		t.Fatalf("parse alipay payment url: %v", err)
	}
	if got := payURL.Query().Get("return_url"); got != returnURL {
		t.Fatalf("expected client_return_url to be used as channel return_url, got %q in %s", got, createResp.Data.PaymentURL)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/orders?page=1&page_size=10", nil)
	listReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	listRec := httptest.NewRecorder()
	handler.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected cashier order list 200, got %d body=%s", listRec.Code, listRec.Body.String())
	}
	var listResp struct {
		Data struct {
			Items      []domainbilling.PaymentOrder `json:"items"`
			Pagination struct {
				Total int `json:"total"`
			} `json:"pagination"`
		} `json:"data"`
	}
	if err := json.NewDecoder(listRec.Body).Decode(&listResp); err != nil {
		t.Fatalf("decode cashier order list: %v", err)
	}
	if listResp.Data.Pagination.Total != 1 || len(listResp.Data.Items) != 1 || listResp.Data.Items[0].ID != createResp.Data.ID {
		t.Fatalf("expected cashier order list to contain created order, got %#v", listResp.Data)
	}
}

func TestCashierEasyPayPopupDisplayIsSignedAndPersisted(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, "cashier-easypay-user@example.com")
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
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := `{"provider_type":"easypay_alipay","name":"易支付支付宝","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"gateway_url":"https://pay.example.com","pid":"10001","key":"merchant-secret","notify_url":"https://merchant.example.com/api/payments/easypay/notify","return_url":"https://merchant.example.com/checkout/return","payment_mode":"popup"}}`
	providerID := createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"12.50000","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected easypay order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	if createResp.Data.ProviderType != "easypay_alipay" || createResp.Data.ProviderInstanceID != providerID || createResp.Data.PaymentURL == "" {
		t.Fatalf("expected easypay payment display, got %#v", createResp.Data)
	}
	payURL, err := url.Parse(createResp.Data.PaymentURL)
	if err != nil {
		t.Fatalf("parse easypay payment url: %v", err)
	}
	query := payURL.Query()
	if payURL.String() == "mock://checkout/"+createResp.Data.OrderNo || !strings.HasSuffix(payURL.Path, "/submit.php") {
		t.Fatalf("expected easypay submit URL, got %s", createResp.Data.PaymentURL)
	}
	if query.Get("pid") != "10001" || query.Get("type") != "alipay" || query.Get("out_trade_no") != createResp.Data.OrderNo || query.Get("money") != "12.50000" || query.Get("sign") == "" || query.Get("sign_type") != "MD5" {
		t.Fatalf("unexpected easypay query params: %s", createResp.Data.PaymentURL)
	}
	if createResp.Data.PaymentDisplay["sign_type"] != "MD5" || createResp.Data.PaymentDisplay["payment_url"] != createResp.Data.PaymentURL {
		t.Fatalf("expected signed display to mirror payment url, got %#v", createResp.Data.PaymentDisplay)
	}

	detailReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/orders/"+jsonInt64(createResp.Data.ID), nil)
	detailReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	detailRec := httptest.NewRecorder()
	handler.ServeHTTP(detailRec, detailReq)
	if detailRec.Code != http.StatusOK {
		t.Fatalf("expected order detail 200, got %d body=%s", detailRec.Code, detailRec.Body.String())
	}
	var detailResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(detailRec.Body).Decode(&detailResp); err != nil {
		t.Fatalf("decode order detail: %v", err)
	}
	if detailResp.Data.PaymentURL != createResp.Data.PaymentURL || detailResp.Data.PaymentDisplay["payment_url"] != createResp.Data.PaymentURL {
		t.Fatalf("expected persisted payment display, create=%#v detail=%#v", createResp.Data, detailResp.Data)
	}
}

func TestCashierEasyPayAPIModeUsesMAPIAndPersistsDisplay(t *testing.T) {
	var upstreamPath string
	var upstreamValues url.Values
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		if r.Method != http.MethodPost {
			t.Fatalf("expected easypay mapi POST, got %s", r.Method)
		}
		if r.URL.Path != "/mapi.php" {
			t.Fatalf("unexpected easypay api path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse easypay mapi form: %v", err)
		}
		upstreamValues = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":1,"msg":"success","trade_no":"easypay-api-trade-001","payurl":"https://pay.example.com/h5/session","qrcode":"https://pay.example.com/qr/session"}`))
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
	userSession := loginExistingAuthUser(t, authSvc, "cashier-easypay-api-user@example.com")
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
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := fmt.Sprintf(`{"provider_type":"easypay_alipay","name":"易支付支付宝 API","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"gateway_url":%q,"pid":"10001","key":"merchant-secret","notify_url":"https://merchant.example.com/api/payments/easypay/notify","return_url":"https://merchant.example.com/checkout/return","payment_mode":"api","client_ip":"127.0.0.1"}}`, upstream.URL)
	providerID := createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"12.50000","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userSession.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected easypay api order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	if createResp.Data.ProviderType != "easypay_alipay" || createResp.Data.ProviderInstanceID != providerID {
		t.Fatalf("expected easypay api provider metadata, got %#v", createResp.Data)
	}
	if upstreamPath != "/mapi.php" {
		t.Fatalf("expected easypay mapi path, got %q", upstreamPath)
	}
	if upstreamValues.Get("pid") != "10001" || upstreamValues.Get("type") != "alipay" || upstreamValues.Get("out_trade_no") != createResp.Data.OrderNo || upstreamValues.Get("money") != "12.50000" || upstreamValues.Get("clientip") != "127.0.0.1" || upstreamValues.Get("sign") == "" || upstreamValues.Get("sign_type") != "MD5" {
		t.Fatalf("unexpected easypay mapi params: %#v", upstreamValues)
	}
	if createResp.Data.PaymentURL != "https://pay.example.com/h5/session" || createResp.Data.QRCode != "https://pay.example.com/qr/session" {
		t.Fatalf("expected easypay api display fields, got %#v", createResp.Data)
	}
	if createResp.Data.PaymentDisplay["type"] != "qr_code" || createResp.Data.PaymentDisplay["payment_url"] != createResp.Data.PaymentURL || createResp.Data.PaymentDisplay["qr_code"] != createResp.Data.QRCode || createResp.Data.PaymentDisplay["prepay_mode"] != "api" {
		t.Fatalf("expected easypay api display to mirror payment fields, got %#v", createResp.Data.PaymentDisplay)
	}
}

func TestCashierEasyPayWebhookRejectsInvalidSignature(t *testing.T) {
	handler, userToken, _ := setupEasyPayCashierTest(t, "cashier-easypay-invalid-sign-user@example.com")
	order := createEasyPayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	values := easyPayWebhookValuesForTest(order, "10001", "merchant-secret", "12.50000", "easypay-trade-invalid-sign")
	values.Set("sign", "invalid-signature")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/easypay", strings.NewReader(values.Encode()))
	webhookReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid easypay signature 400, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if !bytes.Contains(webhookRec.Body.Bytes(), []byte("PAYMENT_SIGNATURE_INVALID")) {
		t.Fatalf("expected PAYMENT_SIGNATURE_INVALID, body=%s", webhookRec.Body.String())
	}
}

func TestCashierEasyPayWebhookRejectsAmountMismatch(t *testing.T) {
	handler, userToken, _ := setupEasyPayCashierTest(t, "cashier-easypay-amount-mismatch-user@example.com")
	order := createEasyPayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	values := easyPayWebhookValuesForTest(order, "10001", "merchant-secret", "10.00000", "easypay-trade-amount-mismatch")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/easypay", strings.NewReader(values.Encode()))
	webhookReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusConflict {
		t.Fatalf("expected easypay amount mismatch 409, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if !bytes.Contains(webhookRec.Body.Bytes(), []byte("PAYMENT_AMOUNT_MISMATCH")) {
		t.Fatalf("expected PAYMENT_AMOUNT_MISMATCH, body=%s", webhookRec.Body.String())
	}
}

func TestCashierEasyPayWebhookCompletesRechargeOrderIdempotently(t *testing.T) {
	handler, userToken, _ := setupEasyPayCashierTest(t, "cashier-easypay-success-user@example.com")
	order := createEasyPayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	values := easyPayWebhookValuesForTest(order, "10001", "merchant-secret", "12.50000", "easypay-trade-success")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/easypay", strings.NewReader(values.Encode()))
	webhookReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected easypay webhook 200, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if strings.TrimSpace(webhookRec.Body.String()) != "success" {
		t.Fatalf("expected raw easypay success response, got body=%s", webhookRec.Body.String())
	}
	completed := getCashierOrderForTest(t, handler, userToken, order.ID)
	if completed.Status != "completed" || completed.LedgerID == 0 || completed.TradeNo != "easypay-trade-success" {
		t.Fatalf("expected completed easypay recharge order, got %#v", completed)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/easypay", strings.NewReader(values.Encode()))
	secondReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second easypay webhook 200, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if strings.TrimSpace(secondRec.Body.String()) != "success" {
		t.Fatalf("expected raw idempotent easypay success response, got body=%s", secondRec.Body.String())
	}
	secondCompleted := getCashierOrderForTest(t, handler, userToken, order.ID)
	if secondCompleted.Status != "completed" || secondCompleted.LedgerID != completed.LedgerID {
		t.Fatalf("expected idempotent easypay webhook order, got %#v", secondCompleted)
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+userToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("balance expected 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if balanceResp.Data.RechargePoints != order.Points || balanceResp.Data.AvailablePoints != order.Points {
		t.Fatalf("expected easypay webhook to credit recharge bucket once, got %#v", balanceResp.Data)
	}
}

func TestCashierAlipayWebhookRejectsInvalidSignature(t *testing.T) {
	handler, userToken, privateKey := setupAlipayCashierTest(t, "cashier-alipay-invalid-sign-user@example.com")
	order := createAlipayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	values := alipayWebhookValuesForTest(t, order, "app-123", privateKey, "12.50000", "alipay-trade-invalid-sign")
	values.Set("sign", "invalid-signature")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/alipay_direct", strings.NewReader(values.Encode()))
	webhookReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid alipay signature 400, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if !bytes.Contains(webhookRec.Body.Bytes(), []byte("PAYMENT_SIGNATURE_INVALID")) {
		t.Fatalf("expected PAYMENT_SIGNATURE_INVALID, body=%s", webhookRec.Body.String())
	}
}

func TestCashierAlipayWebhookRejectsAmountMismatch(t *testing.T) {
	handler, userToken, privateKey := setupAlipayCashierTest(t, "cashier-alipay-amount-mismatch-user@example.com")
	order := createAlipayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	values := alipayWebhookValuesForTest(t, order, "app-123", privateKey, "10.00000", "alipay-trade-amount-mismatch")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/alipay_direct", strings.NewReader(values.Encode()))
	webhookReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusConflict {
		t.Fatalf("expected alipay amount mismatch 409, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if !bytes.Contains(webhookRec.Body.Bytes(), []byte("PAYMENT_AMOUNT_MISMATCH")) {
		t.Fatalf("expected PAYMENT_AMOUNT_MISMATCH, body=%s", webhookRec.Body.String())
	}
}

func TestCashierAlipayWebhookCompletesRechargeOrderIdempotently(t *testing.T) {
	handler, userToken, privateKey := setupAlipayCashierTest(t, "cashier-alipay-success-user@example.com")
	order := createAlipayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	values := alipayWebhookValuesForTest(t, order, "app-123", privateKey, "12.50000", "alipay-trade-success")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/alipay_direct", strings.NewReader(values.Encode()))
	webhookReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected alipay webhook 200, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if strings.TrimSpace(webhookRec.Body.String()) != "success" {
		t.Fatalf("expected raw alipay success response, got body=%s", webhookRec.Body.String())
	}
	completed := getCashierOrderForTest(t, handler, userToken, order.ID)
	if completed.Status != "completed" || completed.LedgerID == 0 || completed.TradeNo != "alipay-trade-success" {
		t.Fatalf("expected completed alipay recharge order, got %#v", completed)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/alipay_direct", strings.NewReader(values.Encode()))
	secondReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second alipay webhook 200, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if strings.TrimSpace(secondRec.Body.String()) != "success" {
		t.Fatalf("expected raw idempotent alipay success response, got body=%s", secondRec.Body.String())
	}
	secondCompleted := getCashierOrderForTest(t, handler, userToken, order.ID)
	if secondCompleted.Status != "completed" || secondCompleted.LedgerID != completed.LedgerID {
		t.Fatalf("expected idempotent alipay webhook order, got %#v", secondCompleted)
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+userToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("balance expected 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if balanceResp.Data.RechargePoints != order.Points || balanceResp.Data.AvailablePoints != order.Points {
		t.Fatalf("expected alipay webhook to credit recharge bucket once, got %#v", balanceResp.Data)
	}
}

func TestCashierWxPayWebhookRejectsInvalidSignature(t *testing.T) {
	handler, userToken, privateKey, apiV3Key, serial := setupWxPayCashierTest(t, "cashier-wxpay-invalid-sign-user@example.com")
	order := createWxPayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	body, headers := wxPayWebhookRequestForTest(t, order, privateKey, apiV3Key, serial, 1250, "wxpay-trade-invalid-sign")
	headers.Set("Wechatpay-Signature", "invalid-signature")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/wxpay_direct", strings.NewReader(body))
	for key, values := range headers {
		for _, value := range values {
			webhookReq.Header.Add(key, value)
		}
	}
	webhookReq.Header.Set("Content-Type", "application/json")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusBadRequest {
		t.Fatalf("expected invalid wxpay signature 400, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if !bytes.Contains(webhookRec.Body.Bytes(), []byte("PAYMENT_SIGNATURE_INVALID")) {
		t.Fatalf("expected PAYMENT_SIGNATURE_INVALID, body=%s", webhookRec.Body.String())
	}
}

func TestCashierWxPayWebhookRejectsAmountMismatch(t *testing.T) {
	handler, userToken, privateKey, apiV3Key, serial := setupWxPayCashierTest(t, "cashier-wxpay-amount-mismatch-user@example.com")
	order := createWxPayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	body, headers := wxPayWebhookRequestForTest(t, order, privateKey, apiV3Key, serial, 1000, "wxpay-trade-amount-mismatch")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/wxpay_direct", strings.NewReader(body))
	for key, values := range headers {
		for _, value := range values {
			webhookReq.Header.Add(key, value)
		}
	}
	webhookReq.Header.Set("Content-Type", "application/json")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusConflict {
		t.Fatalf("expected wxpay amount mismatch 409, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if !bytes.Contains(webhookRec.Body.Bytes(), []byte("PAYMENT_AMOUNT_MISMATCH")) {
		t.Fatalf("expected PAYMENT_AMOUNT_MISMATCH, body=%s", webhookRec.Body.String())
	}
}

func TestCashierWxPayWebhookCompletesRechargeOrderIdempotently(t *testing.T) {
	handler, userToken, privateKey, apiV3Key, serial := setupWxPayCashierTest(t, "cashier-wxpay-success-user@example.com")
	order := createWxPayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	body, headers := wxPayWebhookRequestForTest(t, order, privateKey, apiV3Key, serial, 1250, "wxpay-trade-success")

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/wxpay_direct", strings.NewReader(body))
	for key, values := range headers {
		for _, value := range values {
			webhookReq.Header.Add(key, value)
		}
	}
	webhookReq.Header.Set("Content-Type", "application/json")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("expected wxpay webhook 200, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if strings.TrimSpace(webhookRec.Body.String()) != `{"code":"SUCCESS","message":"成功"}` {
		t.Fatalf("expected raw wxpay success response, got body=%s", webhookRec.Body.String())
	}
	completed := getCashierOrderForTest(t, handler, userToken, order.ID)
	if completed.Status != "completed" || completed.LedgerID == 0 || completed.TradeNo != "wxpay-trade-success" {
		t.Fatalf("expected completed wxpay recharge order, got %#v", completed)
	}

	secondReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/wxpay_direct", strings.NewReader(body))
	for key, values := range headers {
		for _, value := range values {
			secondReq.Header.Add(key, value)
		}
	}
	secondReq.Header.Set("Content-Type", "application/json")
	secondRec := httptest.NewRecorder()
	handler.ServeHTTP(secondRec, secondReq)
	if secondRec.Code != http.StatusOK {
		t.Fatalf("expected second wxpay webhook 200, got %d body=%s", secondRec.Code, secondRec.Body.String())
	}
	if strings.TrimSpace(secondRec.Body.String()) != `{"code":"SUCCESS","message":"成功"}` {
		t.Fatalf("expected raw idempotent wxpay success response, got body=%s", secondRec.Body.String())
	}
	secondCompleted := getCashierOrderForTest(t, handler, userToken, order.ID)
	if secondCompleted.Status != "completed" || secondCompleted.LedgerID != completed.LedgerID {
		t.Fatalf("expected idempotent wxpay webhook order, got %#v", secondCompleted)
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+userToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("balance expected 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if balanceResp.Data.RechargePoints != order.Points || balanceResp.Data.AvailablePoints != order.Points {
		t.Fatalf("expected wxpay webhook to credit recharge bucket once, got %#v", balanceResp.Data)
	}
}

func TestCashierWxPayDirectOrderUsesNativePrepayCodeURL(t *testing.T) {
	privateKey, _ := testRSAKeyPairPEM(t)
	var upstreamPath string
	var upstreamAuth string
	var upstreamBody struct {
		AppID       string `json:"appid"`
		MchID       string `json:"mchid"`
		Description string `json:"description"`
		OutTradeNo  string `json:"out_trade_no"`
		NotifyURL   string `json:"notify_url"`
		Amount      struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		upstreamAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("expected wxpay native prepay POST, got %s", r.Method)
		}
		if r.URL.Path != "/v3/pay/transactions/native" {
			t.Fatalf("unexpected wxpay native path %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read wxpay native body: %v", err)
		}
		if err := json.Unmarshal(body, &upstreamBody); err != nil {
			t.Fatalf("decode wxpay native body: %v body=%s", err, string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code_url":"weixin://wxpay/bizpayurl?pr=native-prepay"}`))
	}))
	defer upstream.Close()

	handler, userToken := setupWxPayNativeCashierTest(t, "cashier-wxpay-native-user@example.com", privateKey, upstream.URL)
	order := createWxPayCustomAmountOrderForWebhookTest(t, handler, userToken, "12.50000")
	if order.Provider != "wxpay_direct" || order.ProviderType != "wxpay_direct" || order.QRCode != "weixin://wxpay/bizpayurl?pr=native-prepay" {
		t.Fatalf("expected wxpay native code_url to populate qr_code, got %#v", order)
	}
	if order.PaymentDisplay["type"] != "qr_code" || order.PaymentDisplay["qr_code"] != order.QRCode || order.PaymentDisplay["prepay_mode"] != "native" {
		t.Fatalf("expected qr_code payment display from native prepay, got %#v", order.PaymentDisplay)
	}
	if upstreamPath != "/v3/pay/transactions/native" {
		t.Fatalf("expected upstream native path, got %s", upstreamPath)
	}
	if !strings.HasPrefix(upstreamAuth, "WECHATPAY2-SHA256-RSA2048 ") || !strings.Contains(upstreamAuth, `mchid="mch-123"`) || !strings.Contains(upstreamAuth, `serial_no="MERCHANTSERIAL001"`) || !strings.Contains(upstreamAuth, `signature="`) {
		t.Fatalf("expected wxpay v3 authorization header, got %s", upstreamAuth)
	}
	if upstreamBody.AppID != "wx-app-123" || upstreamBody.MchID != "mch-123" || upstreamBody.OutTradeNo != order.OrderNo || upstreamBody.Amount.Total != 1250 || upstreamBody.Amount.Currency != "CNY" {
		t.Fatalf("unexpected wxpay native body %#v for order %#v", upstreamBody, order)
	}
	if upstreamBody.NotifyURL != "http://127.0.0.1:1/api/open/image/v1/payments/webhooks/wxpay_direct" || upstreamBody.Description == "" {
		t.Fatalf("expected notify url and description in wxpay native body, got %#v", upstreamBody)
	}
}

func TestCashierWxPayDirectOrderUsesH5PrepayURL(t *testing.T) {
	privateKey, _ := testRSAKeyPairPEM(t)
	var upstreamPath string
	var upstreamAuth string
	var upstreamBody struct {
		AppID       string `json:"appid"`
		MchID       string `json:"mchid"`
		Description string `json:"description"`
		OutTradeNo  string `json:"out_trade_no"`
		NotifyURL   string `json:"notify_url"`
		Amount      struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
		SceneInfo struct {
			PayerClientIP string `json:"payer_client_ip"`
			H5Info        struct {
				Type string `json:"type"`
			} `json:"h5_info"`
		} `json:"scene_info"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		upstreamAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("expected wxpay h5 prepay POST, got %s", r.Method)
		}
		if r.URL.Path != "/v3/pay/transactions/h5" {
			t.Fatalf("unexpected wxpay h5 path %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read wxpay h5 body: %v", err)
		}
		if err := json.Unmarshal(body, &upstreamBody); err != nil {
			t.Fatalf("decode wxpay h5 body: %v body=%s", err, string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"h5_url":"https://wx.tenpay.example/h5pay?prepay_id=h5-prepay"}`))
	}))
	defer upstream.Close()

	handler, userToken := setupWxPayPrepayCashierTest(t, "cashier-wxpay-h5-user@example.com", privateKey, upstream.URL, "h5")
	order := createWxPayCustomAmountOrderForWebhookTest(t, handler, userToken, "13.50000")
	if order.Provider != "wxpay_direct" || order.ProviderType != "wxpay_direct" || order.PaymentURL != "https://wx.tenpay.example/h5pay?prepay_id=h5-prepay" {
		t.Fatalf("expected wxpay h5_url to populate payment_url, got %#v", order)
	}
	if order.QRCode != "" || order.PaymentDisplay["type"] != "redirect" || order.PaymentDisplay["payment_url"] != order.PaymentURL || order.PaymentDisplay["prepay_mode"] != "h5" {
		t.Fatalf("expected redirect payment display from h5 prepay, got order=%#v display=%#v", order, order.PaymentDisplay)
	}
	if upstreamPath != "/v3/pay/transactions/h5" {
		t.Fatalf("expected upstream h5 path, got %s", upstreamPath)
	}
	if !strings.HasPrefix(upstreamAuth, "WECHATPAY2-SHA256-RSA2048 ") || !strings.Contains(upstreamAuth, `mchid="mch-123"`) || !strings.Contains(upstreamAuth, `serial_no="MERCHANTSERIAL001"`) || !strings.Contains(upstreamAuth, `signature="`) {
		t.Fatalf("expected wxpay v3 authorization header, got %s", upstreamAuth)
	}
	if upstreamBody.AppID != "wx-app-123" || upstreamBody.MchID != "mch-123" || upstreamBody.OutTradeNo != order.OrderNo || upstreamBody.Amount.Total != 1350 || upstreamBody.Amount.Currency != "CNY" {
		t.Fatalf("unexpected wxpay h5 body %#v for order %#v", upstreamBody, order)
	}
	if upstreamBody.NotifyURL != "http://127.0.0.1:1/api/open/image/v1/payments/webhooks/wxpay_direct" || upstreamBody.Description == "" {
		t.Fatalf("expected notify url and description in wxpay h5 body, got %#v", upstreamBody)
	}
	if upstreamBody.SceneInfo.PayerClientIP != "127.0.0.1" || upstreamBody.SceneInfo.H5Info.Type != "Wap" {
		t.Fatalf("expected h5 scene info in wxpay h5 body, got %#v", upstreamBody.SceneInfo)
	}
}

func TestCashierWxPayDirectOrderUsesJSAPIPrepayToken(t *testing.T) {
	privateKey, _ := testRSAKeyPairPEM(t)
	var upstreamPath string
	var upstreamAuth string
	var upstreamBody struct {
		AppID       string `json:"appid"`
		MchID       string `json:"mchid"`
		Description string `json:"description"`
		OutTradeNo  string `json:"out_trade_no"`
		NotifyURL   string `json:"notify_url"`
		Amount      struct {
			Total    int64  `json:"total"`
			Currency string `json:"currency"`
		} `json:"amount"`
		Payer struct {
			OpenID string `json:"openid"`
		} `json:"payer"`
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamPath = r.URL.Path
		upstreamAuth = r.Header.Get("Authorization")
		if r.Method != http.MethodPost {
			t.Fatalf("expected wxpay jsapi prepay POST, got %s", r.Method)
		}
		if r.URL.Path != "/v3/pay/transactions/jsapi" {
			t.Fatalf("unexpected wxpay jsapi path %s", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read wxpay jsapi body: %v", err)
		}
		if err := json.Unmarshal(body, &upstreamBody); err != nil {
			t.Fatalf("decode wxpay jsapi body: %v body=%s", err, string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"prepay_id":"wx-jsapi-prepay"}`))
	}))
	defer upstream.Close()

	handler, userToken := setupWxPayPrepayCashierTest(t, "cashier-wxpay-jsapi-user@example.com", privateKey, upstream.URL, "jsapi", `"openid":"wx-openid-001"`)
	order := createWxPayCustomAmountOrderForWebhookTest(t, handler, userToken, "14.50000")
	if order.Provider != "wxpay_direct" || order.ProviderType != "wxpay_direct" || order.ClientToken == "" {
		t.Fatalf("expected wxpay jsapi prepay to populate client_token, got %#v", order)
	}
	var clientToken map[string]string
	if err := json.Unmarshal([]byte(order.ClientToken), &clientToken); err != nil {
		t.Fatalf("expected jsapi client_token json, got %q err=%v", order.ClientToken, err)
	}
	if clientToken["appId"] != "wx-app-123" || clientToken["package"] != "prepay_id=wx-jsapi-prepay" || clientToken["signType"] != "RSA" || clientToken["nonceStr"] == "" || clientToken["timeStamp"] == "" || clientToken["paySign"] == "" {
		t.Fatalf("unexpected jsapi client token %#v", clientToken)
	}
	if order.PaymentDisplay["type"] != "jsapi" || order.PaymentDisplay["client_token"] != order.ClientToken || order.PaymentDisplay["prepay_mode"] != "jsapi" {
		t.Fatalf("expected jsapi payment display, got order=%#v display=%#v", order, order.PaymentDisplay)
	}
	if upstreamPath != "/v3/pay/transactions/jsapi" {
		t.Fatalf("expected upstream jsapi path, got %s", upstreamPath)
	}
	if !strings.HasPrefix(upstreamAuth, "WECHATPAY2-SHA256-RSA2048 ") || !strings.Contains(upstreamAuth, `mchid="mch-123"`) || !strings.Contains(upstreamAuth, `serial_no="MERCHANTSERIAL001"`) || !strings.Contains(upstreamAuth, `signature="`) {
		t.Fatalf("expected wxpay v3 authorization header, got %s", upstreamAuth)
	}
	if upstreamBody.AppID != "wx-app-123" || upstreamBody.MchID != "mch-123" || upstreamBody.OutTradeNo != order.OrderNo || upstreamBody.Amount.Total != 1450 || upstreamBody.Amount.Currency != "CNY" {
		t.Fatalf("unexpected wxpay jsapi body %#v for order %#v", upstreamBody, order)
	}
	if upstreamBody.Payer.OpenID != "wx-openid-001" {
		t.Fatalf("expected payer openid in wxpay jsapi body, got %#v", upstreamBody.Payer)
	}
	if upstreamBody.NotifyURL != "http://127.0.0.1:1/api/open/image/v1/payments/webhooks/wxpay_direct" || upstreamBody.Description == "" {
		t.Fatalf("expected notify url and description in wxpay jsapi body, got %#v", upstreamBody)
	}
}

func TestCashierRoundRobinSchedulesAcrossProviderInstances(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, "cashier-round-robin-user@example.com")
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
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"alipay_direct","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	firstProviderID := createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, alipayProviderBodyForTest(t, "支付宝账号 A", "app-a", true, 10))
	secondProviderID := createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, alipayProviderBodyForTest(t, "支付宝账号 B", "app-b", true, 20))
	if firstProviderID == secondProviderID {
		t.Fatalf("expected different provider ids, got %d", firstProviderID)
	}

	firstOrder := createCashierOrderForSchedulingTest(t, handler, userSession.AccessToken, "alipay")
	secondOrder := createCashierOrderForSchedulingTest(t, handler, userSession.AccessToken, "alipay")
	if firstOrder.ProviderInstanceID != firstProviderID {
		t.Fatalf("expected first order to use first provider %d, got %#v", firstProviderID, firstOrder)
	}
	if secondOrder.ProviderInstanceID != secondProviderID {
		t.Fatalf("expected second order to round-robin to second provider %d, got %#v", secondProviderID, secondOrder)
	}
}

func TestCashierRejectsTooManyPendingOrders(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-pending-limit@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, billingSvc))

	for index := 0; index < 3; index++ {
		order := createCashierOrderForSchedulingTest(t, handler, session.AccessToken, "mock")
		if order.Status != "pending" {
			t.Fatalf("expected pending seed order %d, got %#v", index+1, order)
		}
	}

	limitedReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	limitedReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	limitedReq.Header.Set("Content-Type", "application/json")
	limitedRec := httptest.NewRecorder()
	handler.ServeHTTP(limitedRec, limitedReq)
	if limitedRec.Code != http.StatusConflict {
		t.Fatalf("expected fourth pending order to be rejected with 409, got %d body=%s", limitedRec.Code, limitedRec.Body.String())
	}
	if !bytes.Contains(limitedRec.Body.Bytes(), []byte("PAYMENT_TOO_MANY_PENDING_ORDERS")) {
		t.Fatalf("expected PAYMENT_TOO_MANY_PENDING_ORDERS error, body=%s", limitedRec.Body.String())
	}
}

func TestCashierMockPaymentIsHiddenAndBlockedInProduction(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.App.Env = "production"
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-prod@example.com")
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, billingSvc))

	optionsReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/options", nil)
	optionsReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	optionsRec := httptest.NewRecorder()
	handler.ServeHTTP(optionsRec, optionsReq)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("options expected 200, got %d body=%s", optionsRec.Code, optionsRec.Body.String())
	}
	var optionsResp struct {
		Data struct {
			VisibleMethods []struct {
				Method string `json:"method"`
			} `json:"visible_methods"`
		} `json:"data"`
	}
	if err := json.NewDecoder(optionsRec.Body).Decode(&optionsResp); err != nil {
		t.Fatalf("decode options: %v", err)
	}
	if len(optionsResp.Data.VisibleMethods) != 0 {
		t.Fatalf("expected production options to hide mock, got %#v", optionsResp.Data.VisibleMethods)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusForbidden {
		t.Fatalf("expected production mock order 403, got %d body=%s", createRec.Code, createRec.Body.String())
	}

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/mock", bytes.NewBufferString(`{"order_no":"PGO-PROD-MOCK","trade_no":"MOCK-PROD"}`))
	webhookReq.Header.Set("Content-Type", "application/json")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusForbidden {
		t.Fatalf("expected production mock webhook 403, got %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
}

func TestBillingPricingCNYPerPointControlsBalanceAndCustomOrders(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-global-rate@example.com")
	adminCfgSvc := adminconfigservice.NewService(cfg)
	if _, err := adminCfgSvc.UpdateTab(t.Context(), domainadminconfig.UpdateTabRequest{
		TabKey:  "billing_pricing",
		Version: 1,
		Items: []domainadminconfig.Item{{
			ConfigCategory: "billing_pricing",
			ConfigKey:      "cny_per_point",
			ConfigValue:    map[string]any{"value": "0.50000"},
			Scope:          "global",
		}},
		UpdatedBy: 1,
	}); err != nil {
		t.Fatalf("UpdateTab billing_pricing: %v", err)
	}
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, adminCfgSvc, billingSvc))

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("balance expected 200, got %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	var balanceResp struct {
		Data domainbilling.BalanceSummary `json:"data"`
	}
	if err := json.NewDecoder(balanceRec.Body).Decode(&balanceResp); err != nil {
		t.Fatalf("decode balance: %v", err)
	}
	if balanceResp.Data.CNYPerPoint != "0.50000" {
		t.Fatalf("expected runtime billing rate in balance, got %#v", balanceResp.Data)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create custom amount order expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode custom order: %v", err)
	}
	if createResp.Data.AmountCNY != "10.00000" || createResp.Data.Points != "20.00000" {
		t.Fatalf("expected runtime billing rate in custom order, got %#v", createResp.Data)
	}
}

func TestCashierCustomAmountUsesAdminConfig(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-admin-config-user@example.com")
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
	billingSvc := billingservice.NewService(cfg.Billing)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingSvc, nil, adminAuth, nil))
	adminToken := loginAdminForCashierConfigTest(t, handler)

	updateReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/custom-amount-config", bytes.NewBufferString(`{"enabled":true,"min_amount_cny":"5.00000","max_amount_cny":"500.00000","cny_per_point":"0.50000"}`))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected custom amount config PUT 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create custom amount order expected 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode custom order: %v", err)
	}
	if createResp.Data.AmountCNY != "10.00000" || createResp.Data.Points != "20.00000" {
		t.Fatalf("expected custom amount order to use admin config, got %#v", createResp.Data)
	}

	tooSmallReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"4.00000","visible_method":"mock"}`))
	tooSmallReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	tooSmallReq.Header.Set("Content-Type", "application/json")
	tooSmallRec := httptest.NewRecorder()
	handler.ServeHTTP(tooSmallRec, tooSmallReq)
	if tooSmallRec.Code != http.StatusBadRequest {
		t.Fatalf("expected below-min custom amount 400, got %d body=%s", tooSmallRec.Code, tooSmallRec.Body.String())
	}
	if !bytes.Contains(tooSmallRec.Body.Bytes(), []byte("PAYMENT_AMOUNT_OUT_OF_RANGE")) {
		t.Fatalf("expected PAYMENT_AMOUNT_OUT_OF_RANGE for below-min custom amount, body=%s", tooSmallRec.Body.String())
	}

	disableReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/custom-amount-config", bytes.NewBufferString(`{"enabled":false,"min_amount_cny":"5.00000","max_amount_cny":"500.00000","cny_per_point":"0.50000"}`))
	disableReq.Header.Set("Authorization", "Bearer "+adminToken)
	disableReq.Header.Set("Content-Type", "application/json")
	disableRec := httptest.NewRecorder()
	handler.ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("expected custom amount config disable 200, got %d body=%s", disableRec.Code, disableRec.Body.String())
	}
	disabledCreateReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"10.00000","visible_method":"mock"}`))
	disabledCreateReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	disabledCreateReq.Header.Set("Content-Type", "application/json")
	disabledCreateRec := httptest.NewRecorder()
	handler.ServeHTTP(disabledCreateRec, disabledCreateReq)
	if disabledCreateRec.Code != http.StatusBadRequest {
		t.Fatalf("expected disabled custom amount 400, got %d body=%s", disabledCreateRec.Code, disabledCreateRec.Body.String())
	}
}

func TestCashierVisibleMethodsUseAdminConfig(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "cashier-visible-methods-user@example.com")
	adminStore := adminauthservice.NewMemoryStore()
	if _, err := adminStore.CreateAdmin(t.Context(), domainadminauth.AdminUser{
		Email:        "cashier-visible-admin@example.com",
		PasswordHash: adminauthservice.HashPasswordForTest("password", "salt"),
		Role:         domainadminauth.RoleAdmin,
		Status:       "active",
	}); err != nil {
		t.Fatalf("CreateAdmin: %v", err)
	}
	adminAuth := adminauthservice.NewService(cfg.Auth, adminStore)
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil))
	adminToken := loginAdminForCashierVisibleMethodsTest(t, handler)

	updateReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"mock","label":"Mock Disabled","enabled":false,"source_provider_type":"mock","scheduler_strategy":"round_robin","display_order":30}]}`))
	updateReq.Header.Set("Authorization", "Bearer "+adminToken)
	updateReq.Header.Set("Content-Type", "application/json")
	updateRec := httptest.NewRecorder()
	handler.ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", updateRec.Code, updateRec.Body.String())
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/api/ops/admin/v1/cashier/visible-methods", nil)
	adminReq.Header.Set("Authorization", "Bearer "+adminToken)
	adminRec := httptest.NewRecorder()
	handler.ServeHTTP(adminRec, adminReq)
	if adminRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods GET 200, got %d body=%s", adminRec.Code, adminRec.Body.String())
	}
	var adminResp struct {
		Data struct {
			Items []struct {
				Method  string `json:"method"`
				Label   string `json:"label"`
				Enabled bool   `json:"enabled"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(adminRec.Body).Decode(&adminResp); err != nil {
		t.Fatalf("decode admin visible methods: %v", err)
	}
	if len(adminResp.Data.Items) != 1 || adminResp.Data.Items[0].Method != "mock" || adminResp.Data.Items[0].Enabled || adminResp.Data.Items[0].Label != "Mock Disabled" {
		t.Fatalf("expected admin to see disabled mock method, got %#v", adminResp.Data.Items)
	}

	optionsReq := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/options", nil)
	optionsReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	optionsRec := httptest.NewRecorder()
	handler.ServeHTTP(optionsRec, optionsReq)
	if optionsRec.Code != http.StatusOK {
		t.Fatalf("options expected 200, got %d body=%s", optionsRec.Code, optionsRec.Body.String())
	}
	var optionsResp struct {
		Data struct {
			VisibleMethods []struct {
				Method string `json:"method"`
			} `json:"visible_methods"`
		} `json:"data"`
	}
	if err := json.NewDecoder(optionsRec.Body).Decode(&optionsResp); err != nil {
		t.Fatalf("decode options: %v", err)
	}
	if len(optionsResp.Data.VisibleMethods) != 0 {
		t.Fatalf("expected disabled method to be hidden from users, got %#v", optionsResp.Data.VisibleMethods)
	}

	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"mock"}`))
	createReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusBadRequest {
		t.Fatalf("expected disabled visible method to reject user order, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if !bytes.Contains(createRec.Body.Bytes(), []byte("PAYMENT_METHOD_UNAVAILABLE")) {
		t.Fatalf("expected PAYMENT_METHOD_UNAVAILABLE error, body=%s", createRec.Body.String())
	}
}

func loginAdminForCashierVisibleMethodsTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"cashier-visible-admin@example.com","password":"password"}`))
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

func loginAdminForCashierSchedulingTest(t *testing.T, handler http.Handler) string {
	t.Helper()
	return loginAdminForCashierEmail(t, handler, "cashier-provider-admin@example.com")
}

func loginAdminForCashierEmail(t *testing.T, handler http.Handler, email string) string {
	t.Helper()
	loginReq := httptest.NewRequest(http.MethodPost, "/api/ops/admin/v1/auth/login", bytes.NewBufferString(`{"email":"`+email+`","password":"password"}`))
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

func createCashierProviderInstanceForSchedulingTest(t *testing.T, handler http.Handler, adminToken string, body string) int64 {
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
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode provider response: %v", err)
	}
	return resp.Data.ID
}

func createCashierOrderForSchedulingTest(t *testing.T, handler http.Handler, userToken string, visibleMethod string) domainbilling.PaymentOrder {
	t.Helper()
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"plan","plan_code":"basic-monthly","visible_method":"`+visibleMethod+`"}`))
	createReq.Header.Set("Authorization", "Bearer "+userToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected scheduled order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode created order: %v", err)
	}
	return createResp.Data
}

func setupEasyPayCashierTest(t *testing.T, userEmail string) (http.Handler, string, string) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, userEmail)
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
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"easypay_alipay","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := `{"provider_type":"easypay_alipay","name":"易支付支付宝","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"gateway_url":"https://pay.example.com","pid":"10001","key":"merchant-secret","notify_url":"https://merchant.example.com/api/payments/easypay/notify","return_url":"https://merchant.example.com/checkout/return","payment_mode":"popup"}}`
	createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)
	return handler, userSession.AccessToken, adminToken
}

func setupStripeCashierTest(t *testing.T, userEmail string) (http.Handler, string) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, userEmail)
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
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"stripe","label":"Stripe","enabled":true,"source_provider_type":"stripe","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected Stripe visible method update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := `{"provider_type":"stripe","name":"Stripe Test","enabled":true,"supported_methods":["stripe"],"sort_order":10,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"publishable_key":"pk_test_route"},"secrets":{"secret_key":"sk_test_route","webhook_secret":"whsec_route"}}`
	createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)
	return handler, userSession.AccessToken
}

func createStripeCustomAmountOrderForWebhookTest(t *testing.T, handler http.Handler, userToken, amountCNY string) domainbilling.PaymentOrder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"`+amountCNY+`","visible_method":"stripe"}`))
	req.Header.Set("Authorization", "Bearer "+userToken)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected Stripe order create 201, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode Stripe order: %v", err)
	}
	return resp.Data
}

func setupJeePayCashierTest(t *testing.T, userEmail string) (http.Handler, string, string) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, userEmail)
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
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"jeepay_alipay","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	providerBody := `{"provider_type":"jeepay_alipay","name":"JeePay 支付宝","enabled":true,"supported_methods":["alipay"],"sort_order":10,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"gateway_url":"https://jeepay.example.com","mch_no":"MCH10001","app_id":"APP10001","key":"merchant-secret","notify_url":"https://merchant.example.com/api/payments/jeepay/notify","return_url":"https://merchant.example.com/checkout/return","way_code":"ALI_PC"}}`
	createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)
	return handler, userSession.AccessToken, adminToken
}

func setupAlipayCashierTest(t *testing.T, userEmail string) (http.Handler, string, string) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, userEmail)
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
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"alipay","label":"支付宝","enabled":true,"source_provider_type":"alipay_direct","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	privateKey, publicKey := testRSAKeyPairPEM(t)
	providerBody := alipayProviderBodyWithKeysForTest("支付宝沙箱回调账号", "app-123", true, 10, privateKey, publicKey)
	createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)
	return handler, userSession.AccessToken, privateKey
}

func setupWxPayCashierTest(t *testing.T, userEmail string) (http.Handler, string, string, string, string) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, userEmail)
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
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"wxpay","label":"微信支付","enabled":true,"source_provider_type":"wxpay_direct","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	privateKey, publicKey := testRSAKeyPairPEM(t)
	apiV3Key := "0123456789abcdef0123456789abcdef"
	serial := "WXTESTSERIAL001"
	providerBody := fmt.Sprintf(`{"provider_type":"wxpay_direct","name":"微信支付沙箱回调账号","enabled":true,"supported_methods":["wxpay"],"sort_order":10,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"app_id":"wx-app-123","mch_id":"mch-123","api_v3_key":%q,"wechat_pay_public_key":%q,"wechat_pay_public_key_id":%q,"qr_code":"weixin://wxpay/bizpayurl?pr=test-code"}}`, apiV3Key, publicKey, serial)
	createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)
	return handler, userSession.AccessToken, privateKey, apiV3Key, serial
}

func setupWxPayNativeCashierTest(t *testing.T, userEmail string, privateKey string, gatewayURL string) (http.Handler, string) {
	return setupWxPayPrepayCashierTest(t, userEmail, privateKey, gatewayURL, "native")
}

func setupWxPayPrepayCashierTest(t *testing.T, userEmail string, privateKey string, gatewayURL string, paymentMode string, extraConfigItems ...string) (http.Handler, string) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	cfg.Cashier.SiteBaseURL = "http://127.0.0.1:1"
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	userSession := loginExistingAuthUser(t, authSvc, userEmail)
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
	handler := NewWithAPI(handlers.NewAPIWithCompletionServices(cfg, authSvc, nil, nil, nil, billingservice.NewService(cfg.Billing), nil, adminAuth, nil))
	adminToken := loginAdminForCashierSchedulingTest(t, handler)

	visibleReq := httptest.NewRequest(http.MethodPut, "/api/ops/admin/v1/cashier/visible-methods", bytes.NewBufferString(`{"items":[{"method":"wxpay","label":"微信支付","enabled":true,"source_provider_type":"wxpay_direct","scheduler_strategy":"round_robin","display_order":10}]}`))
	visibleReq.Header.Set("Authorization", "Bearer "+adminToken)
	visibleReq.Header.Set("Content-Type", "application/json")
	visibleRec := httptest.NewRecorder()
	handler.ServeHTTP(visibleRec, visibleReq)
	if visibleRec.Code != http.StatusOK {
		t.Fatalf("expected visible methods update 200, got %d body=%s", visibleRec.Code, visibleRec.Body.String())
	}

	extraConfig := ""
	for _, item := range extraConfigItems {
		item = strings.TrimSpace(item)
		if item != "" {
			extraConfig += "," + item
		}
	}
	providerBody := fmt.Sprintf(`{"provider_type":"wxpay_direct","name":"微信支付沙箱预下单账号","enabled":true,"supported_methods":["wxpay"],"sort_order":10,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"app_id":"wx-app-123","mch_id":"mch-123","api_v3_key":"0123456789abcdef0123456789abcdef","merchant_private_key":%q,"merchant_certificate_serial":"MERCHANTSERIAL001","wechat_pay_public_key_id":"WXTESTSERIAL001","wechat_pay_public_key":"public","gateway_url":%q,"payment_mode":%q,"client_ip":"127.0.0.1","h5_type":"Wap"%s}}`, privateKey, gatewayURL, paymentMode, extraConfig)
	createCashierProviderInstanceForSchedulingTest(t, handler, adminToken, providerBody)
	return handler, userSession.AccessToken
}

func createEasyPayCustomAmountOrderForWebhookTest(t *testing.T, handler http.Handler, userToken string, amountCNY string) domainbilling.PaymentOrder {
	t.Helper()
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"`+amountCNY+`","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected easypay order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode easypay order: %v", err)
	}
	return createResp.Data
}

func createJeePayCustomAmountOrderForWebhookTest(t *testing.T, handler http.Handler, userToken string, amountCNY string) domainbilling.PaymentOrder {
	t.Helper()
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"`+amountCNY+`","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected jeepay order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode jeepay order: %v", err)
	}
	return createResp.Data
}

func createAlipayCustomAmountOrderForWebhookTest(t *testing.T, handler http.Handler, userToken string, amountCNY string) domainbilling.PaymentOrder {
	t.Helper()
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"`+amountCNY+`","visible_method":"alipay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected alipay order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode alipay order: %v", err)
	}
	return createResp.Data
}

func createWxPayCustomAmountOrderForWebhookTest(t *testing.T, handler http.Handler, userToken string, amountCNY string) domainbilling.PaymentOrder {
	t.Helper()
	createReq := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders", bytes.NewBufferString(`{"purchase_type":"custom_amount","amount_cny":"`+amountCNY+`","visible_method":"wxpay"}`))
	createReq.Header.Set("Authorization", "Bearer "+userToken)
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	handler.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected wxpay order create 201, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	var createResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createRec.Body).Decode(&createResp); err != nil {
		t.Fatalf("decode wxpay order: %v", err)
	}
	return createResp.Data
}

func easyPayWebhookValuesForTest(order domainbilling.PaymentOrder, pid, key, money, tradeNo string) url.Values {
	values := url.Values{}
	values.Set("pid", pid)
	values.Set("type", "alipay")
	values.Set("out_trade_no", order.OrderNo)
	values.Set("trade_no", tradeNo)
	values.Set("trade_status", "TRADE_SUCCESS")
	values.Set("money", money)
	values.Set("name", order.PlanName)
	values.Set("sign", easyPaySignForTest(values, key))
	values.Set("sign_type", "MD5")
	return values
}

func jeepayWebhookValuesForTest(order domainbilling.PaymentOrder, mchNo, key, amountFen, tradeNo string) url.Values {
	values := url.Values{}
	values.Set("mchNo", mchNo)
	values.Set("appId", "APP10001")
	values.Set("mchOrderNo", order.OrderNo)
	values.Set("payOrderId", tradeNo)
	values.Set("amount", amountFen)
	values.Set("state", "2")
	values.Set("wayCode", "ALI_PC")
	values.Set("sign", jeepaySignForTest(values, key))
	values.Set("signType", "MD5")
	return values
}

func jeepaySignForTest(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for name, items := range values {
		if strings.EqualFold(name, "sign") || strings.EqualFold(name, "signType") || len(items) == 0 || strings.TrimSpace(items[0]) == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+values.Get(name))
	}
	sum := md5.Sum([]byte(strings.Join(parts, "&") + "&key=" + key))
	return strings.ToUpper(hex.EncodeToString(sum[:]))
}

func easyPaySignForTest(values url.Values, key string) string {
	keys := make([]string, 0, len(values))
	for name, items := range values {
		if name == "sign" || name == "sign_type" || name == "signType" || len(items) == 0 || strings.TrimSpace(items[0]) == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	var builder strings.Builder
	for index, name := range keys {
		if index > 0 {
			_ = builder.WriteByte('&')
		}
		_, _ = builder.WriteString(name + "=" + values.Get(name))
	}
	_, _ = builder.WriteString(key)
	sum := md5.Sum([]byte(builder.String()))
	return hex.EncodeToString(sum[:])
}

func alipayWebhookValuesForTest(t *testing.T, order domainbilling.PaymentOrder, appID, privateKeyPEM, totalAmount, tradeNo string) url.Values {
	t.Helper()
	values := url.Values{}
	values.Set("app_id", appID)
	values.Set("charset", "utf-8")
	values.Set("gmt_payment", "2026-06-05 12:00:00")
	values.Set("notify_id", "notify-"+tradeNo)
	values.Set("notify_time", "2026-06-05 12:00:01")
	values.Set("notify_type", "trade_status_sync")
	values.Set("out_trade_no", order.OrderNo)
	values.Set("seller_id", "2088100000000000")
	values.Set("subject", order.PlanName)
	values.Set("total_amount", totalAmount)
	values.Set("trade_no", tradeNo)
	values.Set("trade_status", "TRADE_SUCCESS")
	values.Set("version", "1.0")
	values.Set("sign_type", "RSA2")
	values.Set("sign", alipaySignForTest(t, values, privateKeyPEM))
	return values
}

func alipaySignForTest(t *testing.T, values url.Values, privateKeyPEM string) string {
	t.Helper()
	key := parseTestRSAPrivateKey(t, privateKeyPEM)
	signContent := alipaySignContentForTest(values)
	digest := sha256.Sum256([]byte(signContent))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign alipay callback: %v", err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func alipaySignContentForTest(values url.Values) string {
	keys := make([]string, 0, len(values))
	for name, items := range values {
		if name == "sign" || len(items) == 0 || strings.TrimSpace(items[0]) == "" {
			continue
		}
		keys = append(keys, name)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, name := range keys {
		parts = append(parts, name+"="+values.Get(name))
	}
	return strings.Join(parts, "&")
}

func wxPayWebhookRequestForTest(t *testing.T, order domainbilling.PaymentOrder, privateKeyPEM string, apiV3Key string, serial string, amountFen int, transactionID string) (string, http.Header) {
	t.Helper()
	plain := map[string]any{
		"mchid":            "mch-123",
		"appid":            "wx-app-123",
		"out_trade_no":     order.OrderNo,
		"transaction_id":   transactionID,
		"trade_state":      "SUCCESS",
		"success_time":     "2026-06-05T12:00:00+08:00",
		"trade_state_desc": "支付成功",
		"amount": map[string]any{
			"total":       amountFen,
			"currency":    "CNY",
			"payer_total": amountFen,
		},
	}
	plainBytes, err := json.Marshal(plain)
	if err != nil {
		t.Fatalf("marshal wxpay plain resource: %v", err)
	}
	nonce := "wxpaynonce12"
	associatedData := "transaction"
	ciphertext := wxPayEncryptResourceForTest(t, apiV3Key, nonce, associatedData, string(plainBytes))
	bodyBytes, err := json.Marshal(map[string]any{
		"id":            "EV-wxpay-" + transactionID,
		"create_time":   "2026-06-05T12:00:01+08:00",
		"event_type":    "TRANSACTION.SUCCESS",
		"resource_type": "encrypt-resource",
		"summary":       "支付成功",
		"resource": map[string]any{
			"algorithm":       "AEAD_AES_256_GCM",
			"ciphertext":      ciphertext,
			"nonce":           nonce,
			"associated_data": associatedData,
			"original_type":   "transaction",
		},
	})
	if err != nil {
		t.Fatalf("marshal wxpay webhook body: %v", err)
	}
	timestamp := "1780622401"
	nonceHeader := "notify-nonce"
	signature := wxPaySignForTest(t, privateKeyPEM, timestamp, nonceHeader, string(bodyBytes))
	headers := http.Header{}
	headers.Set("Wechatpay-Timestamp", timestamp)
	headers.Set("Wechatpay-Nonce", nonceHeader)
	headers.Set("Wechatpay-Serial", serial)
	headers.Set("Wechatpay-Signature", signature)
	return string(bodyBytes), headers
}

func wxPayEncryptResourceForTest(t *testing.T, apiV3Key string, nonce string, associatedData string, plaintext string) string {
	t.Helper()
	block, err := aes.NewCipher([]byte(apiV3Key))
	if err != nil {
		t.Fatalf("create wxpay aes cipher: %v", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatalf("create wxpay gcm: %v", err)
	}
	sealed := gcm.Seal(nil, []byte(nonce), []byte(plaintext), []byte(associatedData))
	return base64.StdEncoding.EncodeToString(sealed)
}

func getCashierOrderForTest(t *testing.T, handler http.Handler, userToken string, orderID int64) domainbilling.PaymentOrder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/agent/cashier/v1/orders/"+jsonInt64(orderID), nil)
	req.Header.Set("Authorization", "Bearer "+userToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cashier order detail expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode cashier order detail: %v", err)
	}
	return resp.Data
}

func wxPaySignForTest(t *testing.T, privateKeyPEM string, timestamp string, nonce string, body string) string {
	t.Helper()
	key := parseTestRSAPrivateKey(t, privateKeyPEM)
	message := timestamp + "\n" + nonce + "\n" + body + "\n"
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign wxpay callback: %v", err)
	}
	return base64.StdEncoding.EncodeToString(signature)
}

func alipayProviderBodyForTest(t *testing.T, name string, appID string, enabled bool, sortOrder int) string {
	t.Helper()
	privateKey, publicKey := testRSAKeyPairPEM(t)
	return alipayProviderBodyWithKeysForTest(name, appID, enabled, sortOrder, privateKey, publicKey)
}

func alipayProviderBodyWithKeysForTest(name string, appID string, enabled bool, sortOrder int, privateKey string, publicKey string) string {
	return fmt.Sprintf(`{"provider_type":"alipay_direct","name":%q,"enabled":%t,"supported_methods":["alipay"],"sort_order":%d,"scheduler_weight":100,"limits":{"min_amount_cny":"1.00000","max_amount_cny":"500.00000"},"config":{"app_id":%q,"app_private_key":%q,"alipay_public_key":%q}}`, name, enabled, sortOrder, appID, privateKey, publicKey)
}

func testRSAPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	privateKey, _ := testRSAKeyPairPEM(t)
	return privateKey
}

func testRSAKeyPairPEM(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 1024)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	publicDER, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal rsa public key: %v", err)
	}
	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	if len(privatePEM) == 0 || len(publicPEM) == 0 {
		t.Fatal("expected rsa key pair pem")
	}
	return string(privatePEM), string(publicPEM)
}

func parseTestRSAPrivateKey(t *testing.T, raw string) *rsa.PrivateKey {
	t.Helper()
	block, _ := pem.Decode([]byte(raw))
	if block == nil {
		t.Fatal("invalid test private key pem")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse test private key: %v", err)
	}
	return key
}
