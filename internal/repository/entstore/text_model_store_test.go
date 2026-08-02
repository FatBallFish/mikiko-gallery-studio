package entstore_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

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

func TestTextModelStoreRejectsModelsForSoftDeletedAccount(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:text-model-deleted-account?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := entstore.NewTextModelStore(client)
	source, err := store.CreateAccount(ctx, domaintextmodel.AccountRecord{Name: "Source", PlatformType: "openai_compatible", APIStyle: "responses", BaseURL: "https://text.example.com", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateAccount source: %v", err)
	}
	target, err := store.CreateAccount(ctx, domaintextmodel.AccountRecord{Name: "Deleted target", PlatformType: "openai_compatible", APIStyle: "responses", BaseURL: "https://text.example.com", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateAccount target: %v", err)
	}
	model, err := store.CreateModel(ctx, domaintextmodel.Model{AccountID: source.ID, ModelCode: "model-a", DisplayName: "Model A", InputPricePerMTok: "0.000000", OutputPricePerMTok: "0.000000", Currency: "USD", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateModel source: %v", err)
	}
	if err := store.DeleteAccount(ctx, target.ID); err != nil {
		t.Fatalf("DeleteAccount target: %v", err)
	}

	if _, err := store.CreateModel(ctx, domaintextmodel.Model{AccountID: target.ID, ModelCode: "model-b", DisplayName: "Model B", InputPricePerMTok: "0.000000", OutputPricePerMTok: "0.000000", Currency: "USD", Enabled: true, Version: 1}); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("create for soft-deleted account: want conflict, got %v", err)
	}
	model.AccountID = target.ID
	model.Version++
	if _, err := store.UpdateModel(ctx, model); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("move to soft-deleted account: want conflict, got %v", err)
	}
}

func TestTextModelStoreModelWritesSharePostgresDefaultLock(t *testing.T) {
	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL text-model lock integration")
	}
	database, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	defer database.Close()
	schemaName := fmt.Sprintf("text_model_lock_%d", time.Now().UnixNano())
	if _, err := database.ExecContext(t.Context(), `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() { _, _ = database.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`) })
	databaseURL := postgresURLWithSearchPath(t, adminURL, schemaName)
	clientA, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatalf("open first ent integration client: %v", err)
	}
	defer clientA.Close()
	clientB, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatalf("open second ent integration client: %v", err)
	}
	defer clientB.Close()
	if err := clientA.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create integration schema tables: %v", err)
	}
	storeA := entstore.NewTextModelStore(clientA)
	storeB := entstore.NewTextModelStore(clientB)
	account, err := storeA.CreateAccount(t.Context(), domaintextmodel.AccountRecord{Name: "Concurrent", PlatformType: "openai_compatible", APIStyle: "responses", BaseURL: "https://text.example.com", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	model, err := storeA.CreateModel(t.Context(), domaintextmodel.Model{AccountID: account.ID, ModelCode: "model-a", DisplayName: "Model A", InputPricePerMTok: "0.000000", OutputPricePerMTok: "0.000000", Currency: "USD", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateModel seed: %v", err)
	}

	assertBlockedByDefaultLock := func(name string, operation func() error) {
		t.Helper()
		lockTx, err := database.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatalf("%s begin lock transaction: %v", name, err)
		}
		if _, err := lockTx.ExecContext(t.Context(), `SET LOCAL search_path TO `+schemaName); err != nil {
			_ = lockTx.Rollback()
			t.Fatalf("%s set search path: %v", name, err)
		}
		if _, err := lockTx.ExecContext(t.Context(), `SELECT id FROM text_model_accounts WHERE deleted_at IS NULL ORDER BY id FOR UPDATE`); err != nil {
			_ = lockTx.Rollback()
			t.Fatalf("%s lock accounts: %v", name, err)
		}
		done := make(chan error, 1)
		go func() { done <- operation() }()
		select {
		case operationErr := <-done:
			_ = lockTx.Rollback()
			t.Fatalf("%s bypassed default-model database lock: %v", name, operationErr)
		case <-time.After(150 * time.Millisecond):
		}
		if err := lockTx.Commit(); err != nil {
			t.Fatalf("%s release account lock: %v", name, err)
		}
		select {
		case operationErr := <-done:
			if operationErr != nil {
				t.Fatalf("%s after account lock release: %v", name, operationErr)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("%s did not finish after account lock release", name)
		}
	}

	assertBlockedByDefaultLock("create model", func() error {
		_, err := storeB.CreateModel(context.Background(), domaintextmodel.Model{AccountID: account.ID, ModelCode: "model-b", DisplayName: "Model B", InputPricePerMTok: "0.000000", OutputPricePerMTok: "0.000000", Currency: "USD", Enabled: true, Version: 1})
		return err
	})
	model.Enabled = false
	model.Version++
	assertBlockedByDefaultLock("update model", func() error {
		_, err := storeB.UpdateModel(context.Background(), model)
		return err
	})
	created, err := storeB.CreateModel(t.Context(), domaintextmodel.Model{AccountID: account.ID, ModelCode: "model-c", DisplayName: "Model C", InputPricePerMTok: "0.000000", OutputPricePerMTok: "0.000000", Currency: "USD", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateModel delete target: %v", err)
	}
	assertBlockedByDefaultLock("delete model", func() error {
		return storeB.DeleteModel(context.Background(), created.ID)
	})
}

func TestTextModelStoreSerializesAccountDeleteWithPostgresModelWrites(t *testing.T) {
	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL text-model delete integration")
	}
	database, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	defer database.Close()
	schemaName := fmt.Sprintf("text_model_delete_%d", time.Now().UnixNano())
	if _, err := database.ExecContext(t.Context(), `CREATE SCHEMA `+schemaName); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() { _, _ = database.Exec(`DROP SCHEMA IF EXISTS ` + schemaName + ` CASCADE`) })
	databaseURL := postgresURLWithSearchPath(t, adminURL, schemaName)
	clientA, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatalf("open first ent integration client: %v", err)
	}
	defer clientA.Close()
	clientB, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatalf("open second ent integration client: %v", err)
	}
	defer clientB.Close()
	if err := clientA.Schema.Create(t.Context()); err != nil {
		t.Fatalf("create integration schema tables: %v", err)
	}
	storeA := entstore.NewTextModelStore(clientA)
	storeB := entstore.NewTextModelStore(clientB)

	newAccount := func(name string) domaintextmodel.AccountRecord {
		t.Helper()
		account, err := storeA.CreateAccount(t.Context(), domaintextmodel.AccountRecord{Name: name, PlatformType: "openai_compatible", APIStyle: "responses", BaseURL: "https://text.example.com", Enabled: true, Version: 1})
		if err != nil {
			t.Fatalf("CreateAccount %s: %v", name, err)
		}
		return account
	}
	beginAccountLock := func(accountID int64) *sql.Tx {
		t.Helper()
		tx, err := database.BeginTx(t.Context(), nil)
		if err != nil {
			t.Fatalf("begin account lock: %v", err)
		}
		if _, err := tx.ExecContext(t.Context(), `SET LOCAL search_path TO `+schemaName); err != nil {
			_ = tx.Rollback()
			t.Fatalf("set search path: %v", err)
		}
		if _, err := tx.ExecContext(t.Context(), `SELECT id FROM text_model_accounts WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, accountID); err != nil {
			_ = tx.Rollback()
			t.Fatalf("lock account %d: %v", accountID, err)
		}
		return tx
	}
	assertBlocked := func(name string, done <-chan error) {
		t.Helper()
		select {
		case operationErr := <-done:
			t.Fatalf("%s bypassed account lock: %v", name, operationErr)
		case <-time.After(150 * time.Millisecond):
		}
	}
	await := func(name string, done <-chan error) error {
		t.Helper()
		select {
		case operationErr := <-done:
			return operationErr
		case <-time.After(3 * time.Second):
			t.Fatalf("%s did not finish after account lock release", name)
			return nil
		}
	}

	createTarget := newAccount("Create target")
	createLock := beginAccountLock(createTarget.ID)
	createDone := make(chan error, 1)
	go func() {
		_, createErr := storeB.CreateModel(context.Background(), domaintextmodel.Model{AccountID: createTarget.ID, ModelCode: "model-create", DisplayName: "Model Create", InputPricePerMTok: "0.000000", OutputPricePerMTok: "0.000000", Currency: "USD", Enabled: true, Version: 1})
		createDone <- createErr
	}()
	assertBlocked("create model", createDone)
	if _, err := createLock.ExecContext(t.Context(), `UPDATE text_model_accounts SET deleted_at = NOW() WHERE id = $1`, createTarget.ID); err != nil {
		_ = createLock.Rollback()
		t.Fatalf("soft-delete create target: %v", err)
	}
	if err := createLock.Commit(); err != nil {
		t.Fatalf("commit create target delete: %v", err)
	}
	if err := await("create model", createDone); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("create after concurrent account delete: want conflict, got %v", err)
	}

	source := newAccount("Move source")
	moveTarget := newAccount("Move target")
	model, err := storeA.CreateModel(t.Context(), domaintextmodel.Model{AccountID: source.ID, ModelCode: "model-move", DisplayName: "Model Move", InputPricePerMTok: "0.000000", OutputPricePerMTok: "0.000000", Currency: "USD", Enabled: true, Version: 1})
	if err != nil {
		t.Fatalf("CreateModel move source: %v", err)
	}
	moveLock := beginAccountLock(moveTarget.ID)
	moveDone := make(chan error, 1)
	go func() {
		model.AccountID = moveTarget.ID
		model.Version++
		_, updateErr := storeB.UpdateModel(context.Background(), model)
		moveDone <- updateErr
	}()
	assertBlocked("move model", moveDone)
	if _, err := moveLock.ExecContext(t.Context(), `UPDATE text_model_accounts SET deleted_at = NOW() WHERE id = $1`, moveTarget.ID); err != nil {
		_ = moveLock.Rollback()
		t.Fatalf("soft-delete move target: %v", err)
	}
	if err := moveLock.Commit(); err != nil {
		t.Fatalf("commit move target delete: %v", err)
	}
	if err := await("move model", moveDone); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("move after concurrent account delete: want conflict, got %v", err)
	}

	deleteTarget := newAccount("Delete target")
	deleteLock := beginAccountLock(deleteTarget.ID)
	deleteDone := make(chan error, 1)
	go func() { deleteDone <- storeB.DeleteAccount(context.Background(), deleteTarget.ID) }()
	assertBlocked("delete account", deleteDone)
	if _, err := deleteLock.ExecContext(t.Context(), `INSERT INTO text_models (account_id, model_code, display_name, input_price_per_million_tokens, output_price_per_million_tokens, currency, enabled, is_default, version, created_at, updated_at) VALUES ($1, 'model-delete-race', 'Model Delete Race', 0, 0, 'USD', TRUE, FALSE, 1, NOW(), NOW())`, deleteTarget.ID); err != nil {
		_ = deleteLock.Rollback()
		t.Fatalf("insert concurrent model: %v", err)
	}
	if err := deleteLock.Commit(); err != nil {
		t.Fatalf("commit concurrent model: %v", err)
	}
	if err := await("delete account", deleteDone); !errors.Is(err, repoerr.ErrConflict) {
		t.Fatalf("delete after concurrent model create: want conflict, got %v", err)
	}
	if _, err := storeA.GetAccount(t.Context(), deleteTarget.ID); err != nil {
		t.Fatalf("account with concurrent model must remain active: %v", err)
	}
}
