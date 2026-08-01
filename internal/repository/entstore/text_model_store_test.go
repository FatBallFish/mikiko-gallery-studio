package entstore_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"entgo.io/ent/dialect"
	domaintextmodel "github.com/fatballfish/pic-gallery/internal/domain/textmodel"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	textmodelservice "github.com/fatballfish/pic-gallery/internal/service/textmodel"
	_ "github.com/mattn/go-sqlite3"
)

var _ textmodelservice.Store = entstore.NewTextModelStore(nil)

func TestTextModelStorePersistsAccountsModelsAndOneDefault(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:text-model-store?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewTextModelStore(client)
	account, err := store.CreateAccount(ctx, domaintextmodel.AccountRecord{
		Name: "Primary", PlatformType: "openai_compatible", APIStyle: "responses",
		BaseURL: "https://text.example.com", SecretEncrypted: map[string]any{"ciphertext": "v1:encrypted"},
		SecretFingerprint: "sha256:1234", Enabled: true, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	first, err := store.CreateModel(ctx, domaintextmodel.Model{
		AccountID: account.ID, ModelCode: "model-a", DisplayName: "Model A",
		InputPricePerMTok: "1.250000", OutputPricePerMTok: "10.000000", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateModel first: %v", err)
	}
	second, err := store.CreateModel(ctx, domaintextmodel.Model{
		AccountID: account.ID, ModelCode: "model-b", DisplayName: "Model B",
		InputPricePerMTok: "2.500000", OutputPricePerMTok: "15.000000", Currency: "USD", Enabled: true,
	})
	if err != nil {
		t.Fatalf("CreateModel second: %v", err)
	}
	if _, err := store.SetDefaultModel(ctx, first.ID); err != nil {
		t.Fatalf("SetDefaultModel first: %v", err)
	}
	selected, err := store.SetDefaultModel(ctx, second.ID)
	if err != nil {
		t.Fatalf("SetDefaultModel second: %v", err)
	}
	if !selected.IsDefault {
		t.Fatal("selected model must be default")
	}
	defaultAccount, defaultModel, err := store.GetDefaultModel(ctx)
	if err != nil {
		t.Fatalf("GetDefaultModel: %v", err)
	}
	if defaultAccount.ID != account.ID || defaultModel.ID != second.ID || !defaultModel.IsDefault {
		t.Fatalf("unexpected default account/model: %#v %#v", defaultAccount, defaultModel)
	}

	rows, err := client.TextModel.Query().All(ctx)
	if err != nil {
		t.Fatalf("query models: %v", err)
	}
	defaults := 0
	for _, row := range rows {
		if row.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("expected exactly one default model, got %d", defaults)
	}
	rawSecret, err := json.Marshal(account.SecretEncrypted)
	if err != nil {
		t.Fatalf("marshal encrypted secret: %v", err)
	}
	if strings.Contains(string(rawSecret), "plain-secret") {
		t.Fatalf("account exposed plaintext secret: %s", rawSecret)
	}
}

func TestTextModelStoreReconcilesDefaultModel(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:text-model-default-reconcile?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := entstore.NewTextModelStore(client)
	account, err := store.CreateAccount(ctx, domaintextmodel.AccountRecord{
		Name: "Primary", PlatformType: "openai_compatible", APIStyle: "responses",
		BaseURL: "https://text.example.com", Enabled: true, Version: 1,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	first, err := store.CreateModel(ctx, domaintextmodel.Model{AccountID: account.ID, ModelCode: "a", DisplayName: "A", Currency: "USD", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateModel first: %v", err)
	}
	second, err := store.CreateModel(ctx, domaintextmodel.Model{AccountID: account.ID, ModelCode: "b", DisplayName: "B", Currency: "USD", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateModel second: %v", err)
	}

	preferred := first.ID
	selection, err := store.ReconcileDefaultModel(ctx, &preferred)
	if err != nil {
		t.Fatalf("ReconcileDefaultModel preferred: %v", err)
	}
	if selection.Account.ID != account.ID || selection.Model.ID != first.ID || !selection.Model.IsDefault {
		t.Fatalf("unexpected preferred selection: %#v", selection)
	}
	preferred = second.ID
	selection, err = store.ReconcileDefaultModel(ctx, &preferred)
	if err != nil {
		t.Fatalf("ReconcileDefaultModel preserve: %v", err)
	}
	if selection.Model.ID != first.ID {
		t.Fatalf("existing default should be preserved: %#v", selection)
	}

	first.Enabled, first.IsDefault, first.Version = false, false, selection.Model.Version+1
	if _, err := store.UpdateModel(ctx, first); err != nil {
		t.Fatalf("disable first: %v", err)
	}
	selection, err = store.ReconcileDefaultModel(ctx, nil)
	if err != nil {
		t.Fatalf("ReconcileDefaultModel unique replacement: %v", err)
	}
	if selection.Model.ID != second.ID || !selection.Model.IsDefault {
		t.Fatalf("second should replace disabled default: %#v", selection)
	}
}

func TestTextModelStoreRequiresExplicitDefaultForAmbiguousLegacyState(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:text-model-default-ambiguous?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := entstore.NewTextModelStore(client)
	account, err := store.CreateAccount(ctx, domaintextmodel.AccountRecord{Name: "Legacy", PlatformType: "openai_compatible", APIStyle: "responses", BaseURL: "https://text.example.com", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	for _, code := range []string{"a", "b"} {
		if _, err := store.CreateModel(ctx, domaintextmodel.Model{AccountID: account.ID, ModelCode: code, DisplayName: code, Currency: "USD", Enabled: true, Version: 1}); err != nil {
			t.Fatalf("CreateModel %s: %v", code, err)
		}
	}
	if _, err := store.ReconcileDefaultModel(ctx, nil); !errors.Is(err, repoerr.ErrDefaultModelRequired) {
		t.Fatalf("expected ErrDefaultModelRequired, got %v", err)
	}
}

func TestTextModelStoreConcurrentDefaultSelectionKeepsOneDefault(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:text-model-default-concurrent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := entstore.NewTextModelStore(client)
	account, err := store.CreateAccount(ctx, domaintextmodel.AccountRecord{Name: "Concurrent", PlatformType: "openai_compatible", APIStyle: "responses", BaseURL: "https://text.example.com", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	first, err := store.CreateModel(ctx, domaintextmodel.Model{AccountID: account.ID, ModelCode: "a", DisplayName: "a", Currency: "USD", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateModel first: %v", err)
	}
	second, err := store.CreateModel(ctx, domaintextmodel.Model{AccountID: account.ID, ModelCode: "b", DisplayName: "b", Currency: "USD", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateModel second: %v", err)
	}

	var wg sync.WaitGroup
	errsCh := make(chan error, 20)
	for index := 0; index < 20; index++ {
		wg.Add(1)
		go func(modelID int64) {
			defer wg.Done()
			_, err := store.SetDefaultModel(ctx, modelID)
			errsCh <- err
		}([]int64{first.ID, second.ID}[index%2])
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		if err != nil {
			t.Fatalf("concurrent SetDefaultModel: %v", err)
		}
	}
	rows, err := client.TextModel.Query().All(ctx)
	if err != nil {
		t.Fatalf("query models: %v", err)
	}
	defaults := 0
	for _, row := range rows {
		if row.IsDefault {
			defaults++
		}
	}
	if defaults != 1 {
		t.Fatalf("expected exactly one default after concurrent selection, got %d", defaults)
	}
}
