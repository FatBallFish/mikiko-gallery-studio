package router

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
)

func TestBillingPlansOrdersWebhookAndSubscriptionFlow(t *testing.T) {
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL:    10 * time.Minute,
		RefreshTokenTTL:   2 * time.Hour,
		Issuer:            "test",
		AccessTokenSecret: "secret",
		RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	session := loginExistingAuthUser(t, authSvc, "billing@example.com")

	billingSvc := billingservice.NewService(cfg.Billing)
	if _, err := billingSvc.AdminAdjust(context.Background(), domainbilling.AdjustRequest{
		UserID:       1,
		ChangePoints: "5.00000",
		Reason:       "seed points",
	}); err != nil {
		t.Fatalf("AdminAdjust: %v", err)
	}

	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, billingSvc)
	handler := NewWithAPI(api)

	plansReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/plans", nil)
	plansReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	plansRec := httptest.NewRecorder()
	handler.ServeHTTP(plansRec, plansReq)
	if plansRec.Code != http.StatusOK {
		t.Fatalf("plans request: %d body=%s", plansRec.Code, plansRec.Body.String())
	}
	if !bytes.Contains(plansRec.Body.Bytes(), []byte(`"basic-monthly"`)) {
		t.Fatalf("expected plans body=%s", plansRec.Body.String())
	}

	createOrderReq := httptest.NewRequest(http.MethodPost, "/api/agent/billing/v1/orders", bytes.NewBufferString(`{"plan_code":"basic-monthly","provider":"mock"}`))
	createOrderReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	createOrderReq.Header.Set("Content-Type", "application/json")
	createOrderRec := httptest.NewRecorder()
	handler.ServeHTTP(createOrderRec, createOrderReq)
	if createOrderRec.Code != http.StatusCreated {
		t.Fatalf("create order: %d body=%s", createOrderRec.Code, createOrderRec.Body.String())
	}
	var orderResp struct {
		Data domainbilling.PaymentOrder `json:"data"`
	}
	if err := json.NewDecoder(createOrderRec.Body).Decode(&orderResp); err != nil {
		t.Fatalf("decode order: %v", err)
	}
	if orderResp.Data.ID == 0 || orderResp.Data.OrderNo == "" {
		t.Fatalf("unexpected order payload %#v", orderResp)
	}
	if orderResp.Data.VisibleMethod != "mock" || orderResp.Data.PaymentDisplay["type"] != "mock" {
		t.Fatalf("expected legacy billing order to be created through cashier, got %#v", orderResp.Data)
	}
	if orderResp.Data.PaymentURL != "" || orderResp.Data.PaymentDisplay["payment_url"] != nil {
		t.Fatalf("expected legacy billing mock order to avoid legacy mock:// payment_url, got order=%#v display=%#v", orderResp.Data, orderResp.Data.PaymentDisplay)
	}

	getOrderReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/orders/"+jsonInt64(orderResp.Data.ID), nil)
	getOrderReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	getOrderRec := httptest.NewRecorder()
	handler.ServeHTTP(getOrderRec, getOrderReq)
	if getOrderRec.Code != http.StatusOK {
		t.Fatalf("get order: %d body=%s", getOrderRec.Code, getOrderRec.Body.String())
	}

	webhookReq := httptest.NewRequest(http.MethodPost, "/api/open/image/v1/payments/webhooks/mock", bytes.NewBufferString(`{"order_no":"`+orderResp.Data.OrderNo+`","trade_no":"MOCK-001"}`))
	webhookReq.Header.Set("Content-Type", "application/json")
	webhookRec := httptest.NewRecorder()
	handler.ServeHTTP(webhookRec, webhookReq)
	if webhookRec.Code != http.StatusOK {
		t.Fatalf("webhook: %d body=%s", webhookRec.Code, webhookRec.Body.String())
	}
	if !bytes.Contains(webhookRec.Body.Bytes(), []byte(`"status":"completed"`)) {
		t.Fatalf("expected completed recharge webhook body=%s", webhookRec.Body.String())
	}

	balanceReq := httptest.NewRequest(http.MethodGet, "/api/agent/billing/v1/balance", nil)
	balanceReq.Header.Set("Authorization", "Bearer "+session.AccessToken)
	balanceRec := httptest.NewRecorder()
	handler.ServeHTTP(balanceRec, balanceReq)
	if balanceRec.Code != http.StatusOK {
		t.Fatalf("balance: %d body=%s", balanceRec.Code, balanceRec.Body.String())
	}
	if !bytes.Contains(balanceRec.Body.Bytes(), []byte(`"subscription_points":"100.00000"`)) || !bytes.Contains(balanceRec.Body.Bytes(), []byte(`"gift_points":"5.00000"`)) {
		t.Fatalf("expected purchased and gift package points body=%s", balanceRec.Body.String())
	}
}

func jsonInt64(value int64) string {
	return strconv.FormatInt(value, 10)
}
