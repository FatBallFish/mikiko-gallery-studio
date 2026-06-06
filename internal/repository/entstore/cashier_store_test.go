package entstore_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"entgo.io/ent/dialect"
	domaincashier "github.com/fatballfish/pic-gallery/internal/domain/cashier"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/paymentproviderinstance"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	_ "github.com/mattn/go-sqlite3"
)

func TestCashierStorePersistsProviderInstancesInDedicatedTable(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:cashierstore-provider-instances?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewCashierStoreWithConfigEncryptionKey(client, "cashier-store-test-encryption-key")
	created, err := store.CreateProviderInstance(ctx, domaincashier.ProviderInstance{
		ProviderType: "alipay_direct",
		Name:         "Alipay Sandbox A",
		Enabled:      true,
		Config:       map[string]any{"app_id": "app-1", "app_private_key": "secret"},
		Limits:       map[string]any{"min_amount_cny": "5", "max_amount_cny": "500"},
	})
	if err != nil {
		t.Fatalf("CreateProviderInstance: %v", err)
	}
	if created.ID <= 0 || created.ConfigStatus != "configured" || created.SupportedMethods[0] != "alipay" {
		t.Fatalf("expected normalized persisted provider instance, got %#v", created)
	}

	row, err := client.PaymentProviderInstance.Query().Where(paymentproviderinstance.IDEQ(int(created.ID))).Only(ctx)
	if err != nil {
		t.Fatalf("query persisted provider instance: %v", err)
	}
	if row.ProviderType != "alipay_direct" || row.CredentialsFingerprint == "" {
		t.Fatalf("expected dedicated table to carry provider metadata and fingerprint, got %#v", row)
	}
	rawConfig, err := json.Marshal(row.ConfigEncrypted)
	if err != nil {
		t.Fatalf("marshal persisted config: %v", err)
	}
	if strings.Contains(string(rawConfig), "secret") || strings.Contains(string(rawConfig), "app_private_key") {
		t.Fatalf("expected provider config to be encrypted at rest, got %s", rawConfig)
	}
	loaded, err := store.ProviderInstances(ctx)
	if err != nil {
		t.Fatalf("ProviderInstances after create: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Config["app_private_key"] != "secret" || loaded[0].Config["app_id"] != "app-1" {
		t.Fatalf("expected store to decrypt provider config for runtime use, got %#v", loaded)
	}

	updated, err := store.UpdateProviderInstance(ctx, created.ID, domaincashier.ProviderInstance{
		ProviderType: "alipay_direct",
		Name:         "Alipay Sandbox B",
		Enabled:      false,
		Config:       map[string]any{"app_id": "app-2"},
	})
	if err != nil {
		t.Fatalf("UpdateProviderInstance: %v", err)
	}
	if updated.Name != "Alipay Sandbox B" || updated.Enabled || updated.ID != created.ID {
		t.Fatalf("expected updated provider instance, got %#v", updated)
	}

	instances, err := store.ProviderInstances(ctx)
	if err != nil {
		t.Fatalf("ProviderInstances: %v", err)
	}
	if len(instances) != 1 || instances[0].Name != "Alipay Sandbox B" {
		t.Fatalf("expected persisted provider instance list, got %#v", instances)
	}

	deleted, err := store.DeleteProviderInstance(ctx, created.ID)
	if err != nil {
		t.Fatalf("DeleteProviderInstance: %v", err)
	}
	if deleted.ID != created.ID || deleted.Name != "Alipay Sandbox B" {
		t.Fatalf("expected deleted provider snapshot, got %#v", deleted)
	}
	afterDelete, err := store.ProviderInstances(ctx)
	if err != nil {
		t.Fatalf("ProviderInstances after delete: %v", err)
	}
	if len(afterDelete) != 0 {
		t.Fatalf("expected deleted provider instance to be removed, got %#v", afterDelete)
	}
}
