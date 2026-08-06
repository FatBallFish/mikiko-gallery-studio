package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
)

type reconciliationProviderStore struct {
	instance domaincashier.ProviderInstance
}

func (s *reconciliationProviderStore) ProviderInstances(context.Context) ([]domaincashier.ProviderInstance, error) {
	return []domaincashier.ProviderInstance{s.instance}, nil
}

func (*reconciliationProviderStore) CreateProviderInstance(context.Context, domaincashier.ProviderInstanceWriteRequest) (domaincashier.ProviderInstance, error) {
	panic("unexpected CreateProviderInstance")
}

func (*reconciliationProviderStore) UpdateProviderInstance(context.Context, int64, domaincashier.ProviderInstanceWriteRequest) (domaincashier.ProviderInstance, error) {
	panic("unexpected UpdateProviderInstance")
}

func (*reconciliationProviderStore) DeleteProviderInstance(context.Context, int64) (domaincashier.ProviderInstance, error) {
	panic("unexpected DeleteProviderInstance")
}

func TestAdminCashierSyncRejectsIncompletePaidEvidence(t *testing.T) {
	for _, tt := range []struct {
		name      string
		tradeNo   string
		amountCNY string
	}{
		{name: "missing amount", tradeNo: "EP-SYNC-001"},
		{name: "missing transaction", amountCNY: "19.90000"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			billingSvc := billingservice.NewService(config.BillingConfig{})
			order, err := billingSvc.CreateOrder(t.Context(), domainbilling.CreateOrderRequest{
				UserID: 1, OrderNo: "PGO-SYNC-INCOMPLETE", PlanCode: "basic-monthly", Provider: "easypay_alipay",
				PurchaseType: "plan", VisibleMethod: "alipay", ProviderType: "easypay_alipay", ProviderInstanceID: 77,
			})
			if err != nil {
				t.Fatalf("CreateOrder: %v", err)
			}

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Fatal(err)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": 1, "trade_status": "TRADE_SUCCESS", "out_trade_no": order.OrderNo, "pid": "1001",
					"trade_no": tt.tradeNo, "money": tt.amountCNY,
				})
			}))
			defer upstream.Close()

			api := NewAPIWithRuntimeServices(config.Config{}, nil, nil, nil, nil, billingSvc)
			api.SetCashierProviderInstanceStore(&reconciliationProviderStore{instance: domaincashier.ProviderInstance{
				ID: 77, ProviderType: "easypay_alipay", Enabled: true, Config: map[string]any{
					"gateway_url": upstream.URL, "query_url": upstream.URL, "pid": "1001", "key": "secret",
				},
			}})

			if _, syncErr := api.syncAdminCashierOrder(t.Context(), order.ID); syncErr == nil {
				t.Fatal("incomplete paid evidence must not complete the order")
			}
			stored, err := billingSvc.GetOrder(t.Context(), order.UserID, order.ID)
			if err != nil || stored.Status != "pending" || stored.LedgerID != 0 {
				t.Fatalf("incomplete paid evidence changed order: order=%#v err=%v", stored, err)
			}
			balance, err := billingSvc.GetBalance(t.Context(), order.UserID, "1.00000")
			if err != nil || balance.RechargePoints != "0.00000" || balance.AvailablePoints != "0.00000" {
				t.Fatalf("incomplete paid evidence credited balance: balance=%#v err=%v", balance, err)
			}
		})
	}
}

func TestCashierProviderInstanceLookupRequiresExactBinding(t *testing.T) {
	api := NewAPIWithRuntimeServices(config.Config{}, nil, nil, nil, nil, billingservice.NewService(config.BillingConfig{}))
	api.SetCashierProviderInstanceStore(&reconciliationProviderStore{instance: domaincashier.ProviderInstance{
		ID: 77, ProviderType: "easypay_alipay", Enabled: true,
	}})

	if instance, ok := api.cashierProviderInstanceForOrder(t.Context(), domainbilling.PaymentOrder{
		ProviderType: "easypay_alipay",
	}); ok {
		t.Fatalf("unbound real order must not select an arbitrary provider instance: %#v", instance)
	}
}
