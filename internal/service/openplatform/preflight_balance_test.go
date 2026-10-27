package openplatform_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainauth "github.com/fatballfish/pic-gallery/internal/domain/auth"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/http/handlers"
	"github.com/fatballfish/pic-gallery/internal/mcpserver"
	"github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/internal/service/openplatform"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type preflightUsers struct{}

func (preflightUsers) GetUserByID(id int64) (domainauth.User, bool) {
	return domainauth.User{ID: 7, Status: "active", GroupMultiplier: "1.00000"}, id == 7
}

type preflightBalanceStore struct {
	billing.Store
	state billing.BalanceState
	reads int
	err   error
}

func (s *preflightBalanceStore) GetBalance(context.Context, int64) (billing.BalanceState, error) {
	s.reads++
	// A second read must not produce a success based on a different snapshot.
	if s.reads > 1 {
		return billing.BalanceState{}, errors.New("duplicate balance read")
	}
	return s.state, s.err
}

type preflightImageBilling struct{ *billing.Service }

func (b preflightImageBilling) ResolveAndEstimateRouteTask(context.Context, domainbilling.EstimateRequest, config.GenerationLimitsConfig) (modelhub.ResolvedRequest, domainbilling.EstimateResult, error) {
	return modelhub.ResolvedRequest{}, domainbilling.EstimateResult{EstimatedPoints: "5.00000", CapabilityVersion: "fixture-v1", UserGroupMultiplier: "1.00000", RequestedOutputImageCount: 1}, nil
}

type preflightKeyQuota struct {
	err   error
	calls int
}

func (q *preflightKeyQuota) CheckTaskAllowed(_ context.Context, keyID, userID int64, points string, _ time.Time) (domainbilling.APIKeyQuota, error) {
	q.calls++
	if keyID != 42 || userID != 7 || points != "5.00000" {
		return domainbilling.APIKeyQuota{}, errors.New("wrong quota inputs")
	}
	return domainbilling.APIKeyQuota{}, q.err
}

func preflightFixture(state billing.BalanceState, quota *preflightKeyQuota) (*openplatform.Service, *preflightBalanceStore, openplatform.Principal) {
	store := &preflightBalanceStore{Store: billing.NewMemoryStore(5), state: state}
	bills := billing.NewServiceWithStore(config.BillingConfig{CNYPerPoint: "0.31250", PointsScale: 5}, store)
	cfg := config.Config{}
	images := imagetask.NewServiceWithProvidersStoreAssetsAndBilling(cfg, nil, imagetask.NewMemoryStore(), nil, preflightImageBilling{bills})
	svc := openplatform.New(openplatform.Dependencies{Auth: preflightUsers{}, Images: images, Billing: bills, APIKeys: quota, QuoteKey: []byte("01234567890123456789012345678901")})
	return svc, store, openplatform.Principal{UserID: 7, APIKeyID: "42", Scopes: []string{"images:write"}}
}

var preflightInput = json.RawMessage(`{"project_id":"project","route_model_code":"fixture","prompt":"cat"}`)

func assertPreflightBalance(t *testing.T, raw []byte, state billing.BalanceState) {
	t.Helper()
	var got struct {
		domainbilling.EstimateResult
		QuoteToken string    `json:"quote_token"`
		ExpiresAt  time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if !got.Sufficient || got.InsufficientPoints != "0.00000" || got.EstimatedPoints != "5.00000" || got.QuoteToken == "" || got.ExpiresAt.IsZero() {
		t.Fatalf("inconsistent successful preflight: %s", raw)
	}
	if got.Balance == nil || got.Balance.AvailablePoints != state.AvailablePoints || got.Balance.FrozenPoints != state.FrozenPoints || !reflect.DeepEqual(got.Balance.Buckets, state.Buckets) || got.Balance.RechargePoints != state.RechargePoints || got.Balance.UserGroupMultiplier != "1.00000" || got.Balance.CNYPerPoint != "0.31250" {
		t.Fatalf("balance snapshot lost: %s", raw)
	}
}

func TestImagePreflightBalanceSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, available, frozen string
		status                  int
		quotaErr                error
	}{
		{name: "incident_2257_for_5", available: "2257.00000", frozen: "0.00000"},
		{name: "exactly_enough", available: "5.00000", frozen: "0.00000"},
		{name: "invalid_balance", available: "invalid", frozen: "0.00000", status: 500},
		{name: "insufficient", available: "4.99999", frozen: "0.00000", status: 402},
		{name: "frozen_is_not_available", available: "4.00000", frozen: "2257.00000", status: 402},
		{name: "frozen_buckets_preserved", available: "2257.00000", frozen: "10.00000"},
		{name: "key_quota_denied", available: "2257.00000", frozen: "0.00000", status: 403, quotaErr: errs.New(403, "API_KEY_QUOTA_EXCEEDED", "quota exceeded")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := billing.BalanceState{AvailablePoints: tc.available, FrozenPoints: tc.frozen, RechargePoints: tc.available, Buckets: []domainbilling.BalanceBucket{{Bucket: "recharge", AvailablePoints: tc.available, FrozenPoints: tc.frozen}}}
			quota := &preflightKeyQuota{err: tc.quotaErr}
			svc, store, principal := preflightFixture(state, quota)
			out, err := svc.Execute(t.Context(), principal, "preflight_image_task", preflightInput)
			if store.reads != 1 {
				t.Fatalf("balance reads=%d", store.reads)
			}
			if tc.status != 0 {
				var app *errs.Error
				if !errors.As(err, &app) || app.StatusCode != tc.status || out != nil {
					t.Fatalf("out=%#v err=%v", out, err)
				}
				if tc.status == 402 && app.Code != "INSUFFICIENT_BALANCE" {
					t.Fatalf("code=%s", app.Code)
				}
				if tc.status == 403 && app.Code != "API_KEY_QUOTA_EXCEEDED" {
					t.Fatalf("quota code=%s", app.Code)
				}
				wantQuotaChecks := 0
				if tc.status == 403 {
					wantQuotaChecks = 1
				}
				if quota.calls != wantQuotaChecks {
					t.Fatalf("quota checks=%d, want %d", quota.calls, wantQuotaChecks)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if quota.calls != 1 {
				t.Fatalf("quota checks=%d", quota.calls)
			}
			raw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}
			assertPreflightBalance(t, raw, state)
		})
	}
}

type preflightBearerTransport struct{}

func (preflightBearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer fixture-token")
	return http.DefaultTransport.RoundTrip(r)
}

func assertMCPPreflightBalance(t *testing.T, raw []byte, state billing.BalanceState) {
	t.Helper()
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	assertPreflightBalance(t, envelope.Data, state)
}

func TestImagePreflightBalanceRESTAndMCPDTO(t *testing.T) {
	state := billing.BalanceState{AvailablePoints: "2257.00000", FrozenPoints: "10.00000", RechargePoints: "2257.00000", Buckets: []domainbilling.BalanceBucket{{Bucket: "recharge", AvailablePoints: "2257.00000", FrozenPoints: "10.00000"}}}
	t.Run("REST", func(t *testing.T) {
		svc, store, p := preflightFixture(state, &preflightKeyQuota{})
		h := handlers.NewOpenPlatform(svc, func(*http.Request) (openplatform.Principal, error) { return p, nil })
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/open/v1/image-tasks/preflight", bytes.NewReader(preflightInput)))
		if rec.Code != 200 {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var envelope struct {
			Data json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		assertPreflightBalance(t, envelope.Data, state)
		if store.reads != 1 {
			t.Fatalf("balance reads=%d", store.reads)
		}
	})
	t.Run("MCP", func(t *testing.T) {
		svc, store, p := preflightFixture(state, &preflightKeyQuota{})
		p.ClientID = "fixture-client"
		p.GrantID = "fixture-grant"
		h, err := mcpserver.NewHandler(mcpserver.Config{ResourceURL: "https://studio.example/mcp", Service: svc, Validator: mcpserver.ValidateAccessFunc(func(context.Context, string) (openplatform.Principal, error) { return p, nil })})
		if err != nil {
			t.Fatal(err)
		}
		server := httptest.NewServer(h)
		defer server.Close()
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		client := mcp.NewClient(&mcp.Implementation{Name: "balance-test", Version: "1"}, nil)
		session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: server.URL, HTTPClient: &http.Client{Transport: preflightBearerTransport{}}}, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer session.Close()
		var input map[string]any
		if err = json.Unmarshal(preflightInput, &input); err != nil {
			t.Fatal(err)
		}
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "preflight_image_task", Arguments: input})
		if err != nil || result.IsError {
			t.Fatalf("MCP err=%v result=%#v", err, result)
		}
		raw, err := json.Marshal(result.StructuredContent)
		if err != nil {
			t.Fatal(err)
		}
		assertMCPPreflightBalance(t, raw, state)
		if len(result.Content) != 1 {
			t.Fatalf("content=%#v", result.Content)
		}
		text, ok := result.Content[0].(*mcp.TextContent)
		if !ok {
			t.Fatalf("content=%T", result.Content[0])
		}
		assertMCPPreflightBalance(t, []byte(text.Text), state)
		if store.reads != 1 {
			t.Fatalf("balance reads=%d", store.reads)
		}
	})
}
