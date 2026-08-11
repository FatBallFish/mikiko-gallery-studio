package router

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	authservice "github.com/fatballfish/pic-gallery/internal/service/auth"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	cashierservice "github.com/fatballfish/pic-gallery/internal/service/cashier"
)

func TestCashierOrderSyncReconcilesPaidProvider(t *testing.T) {
	queryCalls := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/query" {
			http.NotFound(w, r)
			return
		}
		queryCalls++
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1, "trade_status": "TRADE_SUCCESS", "out_trade_no": r.PostForm.Get("out_trade_no"), "pid": "1001",
			"trade_no": "EP-INIT-001", "money": "19.90000",
		})
	}))
	defer upstream.Close()

	handler, billingSvc, session, order := setupSafeCashierCancelTest(t, upstream.URL, true, true)
	req := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(order.ID)+"/sync", nil)
	req.Header.Set("Authorization", "Bearer "+session.AccessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("sync expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `"raw"`) {
		t.Fatalf("user sync response exposed provider raw payload: %s", rec.Body.String())
	}
	var response struct {
		Data struct {
			Order domainbilling.PaymentOrder            `json:"order"`
			Sync  cashierservice.QueryOrderStatusResult `json:"sync"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode sync response: %v", err)
	}
	if response.Data.Order.Status != "completed" || response.Data.Order.CompletedAt == nil {
		t.Fatalf("unexpected reconciled order %#v", response.Data.Order)
	}
	if !response.Data.Sync.Paid || !response.Data.Sync.Completed || response.Data.Sync.Raw != nil || queryCalls != 1 {
		t.Fatalf("unexpected sanitized sync result %#v queryCalls=%d", response.Data.Sync, queryCalls)
	}
	stored, err := billingSvc.GetOrder(t.Context(), order.UserID, order.ID)
	if err != nil || stored.Status != "completed" || stored.TradeNo != "EP-INIT-001" || stored.LedgerID == 0 {
		t.Fatalf("sync did not persist completion: order=%#v err=%v", stored, err)
	}
}

func TestCashierOrderSyncEnforcesOwnershipAndProviderEvidence(t *testing.T) {
	t.Run("pending response is sanitized", func(t *testing.T) {
		upstream := pendingEasyPayQueryServer(t, nil)
		defer upstream.Close()
		handler, _, owner, _, order := setupCashierSyncTest(t, upstream.URL, 81, []domaincashier.ProviderInstance{easyPaySyncInstance(81, upstream.URL)})
		response := syncCashierOrderForTest(t, handler, owner.AccessToken, order.ID, http.StatusOK)
		if response.Order.Status != "pending" || response.Sync.QueryStatus != "pending" || response.Sync.Paid || response.Sync.Raw != nil {
			t.Fatalf("unexpected pending sync response %#v", response)
		}
	})

	t.Run("cross user is hidden", func(t *testing.T) {
		upstream := pendingEasyPayQueryServer(t, nil)
		defer upstream.Close()
		handler, _, _, other, order := setupCashierSyncTest(t, upstream.URL, 81, []domaincashier.ProviderInstance{easyPaySyncInstance(81, upstream.URL)})
		syncCashierOrderForTest(t, handler, other.AccessToken, order.ID, http.StatusNotFound)
	})

	t.Run("missing provider instance fails closed", func(t *testing.T) {
		handler, billingSvc, owner, _, order := setupCashierSyncTest(t, "http://127.0.0.1:1", 99, []domaincashier.ProviderInstance{easyPaySyncInstance(81, "http://127.0.0.1:1")})
		syncCashierOrderForTest(t, handler, owner.AccessToken, order.ID, http.StatusConflict)
		stored, err := billingSvc.GetOrder(t.Context(), order.UserID, order.ID)
		if err != nil || stored.Status != "pending" || stored.LedgerID != 0 {
			t.Fatalf("missing provider changed order: order=%#v err=%v", stored, err)
		}
	})

	t.Run("paid amount mismatch fails closed", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"code": 1, "trade_status": "TRADE_SUCCESS", "out_trade_no": r.PostForm.Get("out_trade_no"), "pid": "1001",
				"trade_no": "EP-SYNC-MISMATCH", "money": "20.00000",
			})
		}))
		defer upstream.Close()
		handler, billingSvc, owner, _, order := setupCashierSyncTest(t, upstream.URL, 81, []domaincashier.ProviderInstance{easyPaySyncInstance(81, upstream.URL)})
		syncCashierOrderForTest(t, handler, owner.AccessToken, order.ID, http.StatusConflict)
		stored, err := billingSvc.GetOrder(t.Context(), order.UserID, order.ID)
		if err != nil || stored.Status != "pending" || stored.LedgerID != 0 {
			t.Fatalf("amount mismatch changed order: order=%#v err=%v", stored, err)
		}
	})
}

func TestCashierOrderSyncCoalescesConcurrentProviderQueries(t *testing.T) {
	var queryCalls atomic.Int32
	upstream := pendingEasyPayQueryServer(t, func() {
		queryCalls.Add(1)
		time.Sleep(80 * time.Millisecond)
	})
	defer upstream.Close()
	handler, _, owner, _, order := setupCashierSyncTest(t, upstream.URL, 81, []domaincashier.ProviderInstance{easyPaySyncInstance(81, upstream.URL)})

	const requests = 8
	var wg sync.WaitGroup
	errs := make(chan string, requests)
	for range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(order.ID)+"/sync", nil)
			req.Header.Set("Authorization", "Bearer "+owner.AccessToken)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				errs <- fmt.Sprintf("status=%d body=%s", rec.Code, rec.Body.String())
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if got := queryCalls.Load(); got != 1 {
		t.Fatalf("concurrent sync made %d provider queries, want 1", got)
	}
}

func TestCashierOrderSyncThrottlesSequentialProviderQueries(t *testing.T) {
	var queryCalls atomic.Int32
	upstream := pendingEasyPayQueryServer(t, func() {
		queryCalls.Add(1)
	})
	defer upstream.Close()
	handler, _, owner, _, order := setupCashierSyncTest(t, upstream.URL, 81, []domaincashier.ProviderInstance{easyPaySyncInstance(81, upstream.URL)})

	first := syncCashierOrderForTest(t, handler, owner.AccessToken, order.ID, http.StatusOK)
	second := syncCashierOrderForTest(t, handler, owner.AccessToken, order.ID, http.StatusOK)
	if first.Sync.QueryStatus != "pending" || second.Sync.QueryStatus != "pending" {
		t.Fatalf("unexpected throttled responses first=%#v second=%#v", first, second)
	}
	if got := queryCalls.Load(); got != 1 {
		t.Fatalf("sequential sync made %d provider queries within throttle window, want 1", got)
	}
}

func TestCashierOrderSyncKeepsSharedQueryAliveWhenOneWaiterCancels(t *testing.T) {
	var queryCalls atomic.Int32
	queryStarted := make(chan struct{})
	releaseQuery := make(chan struct{})
	upstream := pendingEasyPayQueryServer(t, func() {
		if queryCalls.Add(1) == 1 {
			close(queryStarted)
		}
		<-releaseQuery
	})
	defer upstream.Close()
	handler, _, owner, _, order := setupCashierSyncTest(t, upstream.URL, 81, []domaincashier.ProviderInstance{easyPaySyncInstance(81, upstream.URL)})

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstDone := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(order.ID)+"/sync", nil).WithContext(firstCtx)
		req.Header.Set("Authorization", "Bearer "+owner.AccessToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		firstDone <- rec.Code
	}()
	<-queryStarted

	secondDone := make(chan int, 1)
	go func() {
		req := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(order.ID)+"/sync", nil)
		req.Header.Set("Authorization", "Bearer "+owner.AccessToken)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		secondDone <- rec.Code
	}()
	cancelFirst()
	close(releaseQuery)

	if got := <-firstDone; got != http.StatusRequestTimeout {
		t.Fatalf("canceled waiter status=%d, want %d", got, http.StatusRequestTimeout)
	}
	if got := <-secondDone; got != http.StatusOK {
		t.Fatalf("remaining waiter status=%d, want %d", got, http.StatusOK)
	}
	if got := queryCalls.Load(); got != 1 {
		t.Fatalf("shared query calls=%d, want 1", got)
	}
}

func setupCashierSyncTest(t *testing.T, upstreamURL string, providerInstanceID int64, instances []domaincashier.ProviderInstance) (http.Handler, *billingservice.Service, domainauthSession, domainauthSession, domainbilling.PaymentOrder) {
	t.Helper()
	cfg := taskAPIConfig("http://127.0.0.1:1")
	authSvc := authservice.NewService(config.AuthConfig{
		AccessTokenTTL: 10 * time.Minute, RefreshTokenTTL: 2 * time.Hour, Issuer: "test",
		AccessTokenSecret: "secret", RefreshCookieName: "pg_refresh",
	}, map[string]string{"basic": "1.00000"})
	owner := loginExistingAuthUser(t, authSvc, "cashier-sync-owner-"+fmt.Sprint(time.Now().UnixNano())+"@example.com")
	other := loginExistingAuthUser(t, authSvc, "cashier-sync-other-"+fmt.Sprint(time.Now().UnixNano())+"@example.com")
	claims, err := authSvc.ParseAccessToken(owner.AccessToken)
	if err != nil {
		t.Fatalf("ParseAccessToken: %v", err)
	}
	billingSvc := billingservice.NewService(cfg.Billing)
	order, err := billingSvc.CreateOrder(context.Background(), domainbilling.CreateOrderRequest{
		UserID: claims.UserID, OrderNo: "PGO-USER-SYNC-" + fmt.Sprint(time.Now().UnixNano()), PlanCode: "basic-monthly",
		Provider: "easypay_alipay", PurchaseType: "plan", VisibleMethod: "alipay", ProviderType: "easypay_alipay", ProviderInstanceID: providerInstanceID,
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	api := handlers.NewAPIWithRuntimeServices(cfg, authSvc, nil, nil, nil, billingSvc)
	api.SetCashierProviderInstanceStore(&cashierProviderInstanceStoreStub{instances: instances})
	return NewWithAPI(api), billingSvc, owner, other, order
}

func easyPaySyncInstance(id int64, upstreamURL string) domaincashier.ProviderInstance {
	return domaincashier.ProviderInstance{
		ID: id, ProviderType: "easypay_alipay", Enabled: true, SupportedMethods: []string{"alipay"},
		Config: map[string]any{"gateway_url": upstreamURL, "query_url": upstreamURL, "pid": "1001", "key": "secret"},
	}
}

func pendingEasyPayQueryServer(t *testing.T, beforeResponse func()) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if beforeResponse != nil {
			beforeResponse()
		}
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 1, "trade_status": "WAITING", "out_trade_no": r.PostForm.Get("out_trade_no"), "pid": "1001",
		})
	}))
}

func syncCashierOrderForTest(t *testing.T, handler http.Handler, accessToken string, orderID int64, wantStatus int) struct {
	Order domainbilling.PaymentOrder
	Sync  cashierservice.QueryOrderStatusResult
} {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/agent/cashier/v1/orders/"+jsonInt64(orderID)+"/sync", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != wantStatus {
		t.Fatalf("sync expected %d, got %d body=%s", wantStatus, rec.Code, rec.Body.String())
	}
	result := struct {
		Order domainbilling.PaymentOrder
		Sync  cashierservice.QueryOrderStatusResult
	}{}
	if wantStatus == http.StatusOK {
		var response struct {
			Data struct {
				Order domainbilling.PaymentOrder            `json:"order"`
				Sync  cashierservice.QueryOrderStatusResult `json:"sync"`
			} `json:"data"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("decode sync response: %v", err)
		}
		result.Order, result.Sync = response.Data.Order, response.Data.Sync
	}
	return result
}
