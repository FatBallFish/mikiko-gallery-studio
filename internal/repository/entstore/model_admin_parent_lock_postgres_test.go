package entstore

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainmodeladmin "github.com/fatballfish/pic-gallery/internal/domain/modeladmin"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/modelaccountmodel"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelcandidate"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/routemodelprice"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/lib/pq"
)

func TestStableModelAdminIDsSortsAndDeduplicatesParents(t *testing.T) {
	if got, want := stableModelAdminIDs(9, 3, 9, 0, -1, 5), []int{3, 5, 9}; !reflect.DeepEqual(got, want) {
		t.Fatalf("stable parent IDs = %#v, want %#v", got, want)
	}
}

func TestModelAdminParentLocksPreventWritesAcrossDeletionPostgres(t *testing.T) {
	ctx, database, clientA, clientB := openModelAdminParentLockPostgres(t)
	storeA := NewModelAdminStore(clientA)
	storeB := NewModelAdminStore(clientB)

	t.Run("model create rechecks deleted account", func(t *testing.T) {
		account := mustCreateModelAdminAccount(t, ctx, storeA, "create-parent")
		deleteTx := mustLockModelAdminParent(t, ctx, database, "model_accounts", account.ID)
		done := make(chan error, 1)
		go func() {
			_, err := storeB.CreateModelAccountModel(context.Background(), modelAdminAccountModelRequest(account.ID, "blocked-create"))
			done <- err
		}()
		assertPostgresOperationBlocked(t, done, "model create while account delete holds FOR UPDATE")
		if _, err := deleteTx.ExecContext(ctx, `UPDATE model_accounts SET status = 'disabled', deleted_at = CURRENT_TIMESTAMP WHERE id = $1`, account.ID); err != nil {
			_ = deleteTx.Rollback()
			t.Fatalf("soft-delete locked account: %v", err)
		}
		if err := deleteTx.Commit(); err != nil {
			t.Fatalf("commit account delete: %v", err)
		}
		if err := awaitPostgresOperation(t, done, "model create after account delete"); !errors.Is(err, repoerr.ErrConflict) {
			t.Fatalf("model create after account delete = %v, want conflict", err)
		}
		if count, err := clientA.ModelAccountModel.Query().Where(modelaccountmodel.ModelCodeEQ("blocked-create")).Count(ctx); err != nil || count != 0 {
			t.Fatalf("model crossed deleted account boundary: count=%d err=%v", count, err)
		}
	})

	t.Run("model move rechecks deleted target account", func(t *testing.T) {
		source := mustCreateModelAdminAccount(t, ctx, storeA, "move-source")
		target := mustCreateModelAdminAccount(t, ctx, storeA, "move-target")
		model, err := storeA.CreateModelAccountModel(ctx, modelAdminAccountModelRequest(source.ID, "move-model"))
		if err != nil {
			t.Fatalf("create moving model: %v", err)
		}
		request := modelAdminAccountModelRequest(target.ID, model.ModelCode)
		deleteTx := mustLockModelAdminParent(t, ctx, database, "model_accounts", target.ID)
		done := make(chan error, 1)
		go func() { _, err := storeB.UpdateModelAccountModel(context.Background(), model.ID, request); done <- err }()
		assertPostgresOperationBlocked(t, done, "model move while target account delete holds FOR UPDATE")
		if _, err := deleteTx.ExecContext(ctx, `UPDATE model_accounts SET status = 'disabled', deleted_at = CURRENT_TIMESTAMP WHERE id = $1`, target.ID); err != nil {
			_ = deleteTx.Rollback()
			t.Fatalf("soft-delete target account: %v", err)
		}
		if err := deleteTx.Commit(); err != nil {
			t.Fatalf("commit target account delete: %v", err)
		}
		if err := awaitPostgresOperation(t, done, "model move after target account delete"); !errors.Is(err, repoerr.ErrConflict) {
			t.Fatalf("model move after account delete = %v, want conflict", err)
		}
		reloaded, err := clientA.ModelAccountModel.Get(ctx, int(model.ID))
		if err != nil || reloaded.AccountID != source.ID {
			t.Fatalf("failed model move changed parent: %#v err=%v", reloaded, err)
		}
	})

	t.Run("candidate move rechecks deleted target route", func(t *testing.T) {
		account := mustCreateModelAdminAccount(t, ctx, storeA, "candidate-account")
		model, err := storeA.CreateModelAccountModel(ctx, modelAdminAccountModelRequest(account.ID, "candidate-model"))
		if err != nil {
			t.Fatalf("create candidate model: %v", err)
		}
		source := mustCreateModelAdminRoute(t, ctx, storeA, "candidate-source")
		target := mustCreateModelAdminRoute(t, ctx, storeA, "candidate-target")
		candidate, err := storeA.CreateRouteModelCandidate(ctx, domainmodeladmin.RouteModelCandidateWriteRequest{RouteModelID: source.ID, AccountModelID: model.ID, Weight: 100, Enabled: true})
		if err != nil {
			t.Fatalf("create moving candidate: %v", err)
		}
		deleteTx := mustLockModelAdminParent(t, ctx, database, "route_models", target.ID)
		done := make(chan error, 1)
		go func() {
			_, err := storeB.UpdateRouteModelCandidate(context.Background(), candidate.ID, domainmodeladmin.RouteModelCandidateWriteRequest{RouteModelID: target.ID, AccountModelID: model.ID, Weight: 100, Enabled: true})
			done <- err
		}()
		assertPostgresOperationBlocked(t, done, "candidate move while target route delete holds FOR UPDATE")
		if _, err := deleteTx.ExecContext(ctx, `UPDATE route_models SET enabled = false, deleted_at = CURRENT_TIMESTAMP WHERE id = $1`, target.ID); err != nil {
			_ = deleteTx.Rollback()
			t.Fatalf("soft-delete target route: %v", err)
		}
		if err := deleteTx.Commit(); err != nil {
			t.Fatalf("commit target route delete: %v", err)
		}
		if err := awaitPostgresOperation(t, done, "candidate move after target route delete"); !errors.Is(err, repoerr.ErrConflict) {
			t.Fatalf("candidate move after route delete = %v, want conflict", err)
		}
		reloaded, err := clientA.RouteModelCandidate.Query().Where(routemodelcandidate.IDEQ(int(candidate.ID))).Only(ctx)
		if err != nil || reloaded.RouteModelID != source.ID {
			t.Fatalf("failed candidate move changed parent: %#v err=%v", reloaded, err)
		}
	})

	t.Run("price move rechecks deleted target route", func(t *testing.T) {
		source := mustCreateModelAdminRoute(t, ctx, storeA, "price-source")
		target := mustCreateModelAdminRoute(t, ctx, storeA, "price-target")
		price, err := storeA.CreateRouteModelPrice(ctx, modelAdminPriceRequest(source.ID, "1k"))
		if err != nil {
			t.Fatalf("create moving price: %v", err)
		}
		deleteTx := mustLockModelAdminParent(t, ctx, database, "route_models", target.ID)
		done := make(chan error, 1)
		go func() {
			_, err := storeB.UpdateRouteModelPrice(context.Background(), price.ID, modelAdminPriceRequest(target.ID, "1k"))
			done <- err
		}()
		assertPostgresOperationBlocked(t, done, "price move while target route delete holds FOR UPDATE")
		if _, err := deleteTx.ExecContext(ctx, `UPDATE route_models SET enabled = false, deleted_at = CURRENT_TIMESTAMP WHERE id = $1`, target.ID); err != nil {
			_ = deleteTx.Rollback()
			t.Fatalf("soft-delete target price route: %v", err)
		}
		if err := deleteTx.Commit(); err != nil {
			t.Fatalf("commit target price route delete: %v", err)
		}
		if err := awaitPostgresOperation(t, done, "price move after target route delete"); !errors.Is(err, repoerr.ErrConflict) {
			t.Fatalf("price move after route delete = %v, want conflict", err)
		}
		reloaded, err := clientA.RouteModelPrice.Query().Where(routemodelprice.IDEQ(int(price.ID))).Only(ctx)
		if err != nil || reloaded.RouteModelID != source.ID {
			t.Fatalf("failed price move changed parent: %#v err=%v", reloaded, err)
		}
	})
}

func openModelAdminParentLockPostgres(t *testing.T) (context.Context, *sql.DB, *repoent.Client, *repoent.Client) {
	t.Helper()
	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL model administration parent-lock integration")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schemaName := fmt.Sprintf("model_admin_parent_lock_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+pq.QuoteIdentifier(schemaName)); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + pq.QuoteIdentifier(schemaName) + ` CASCADE`) })
	databaseURL := projectPostgresURLWithSearchPath(t, adminURL, schemaName)
	database, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("open scoped integration database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	clientA, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatalf("open first ent client: %v", err)
	}
	t.Cleanup(func() { _ = clientA.Close() })
	clientB, err := repoent.Open(dialect.Postgres, databaseURL)
	if err != nil {
		t.Fatalf("open second ent client: %v", err)
	}
	t.Cleanup(func() { _ = clientB.Close() })
	if err := clientA.Schema.Create(ctx); err != nil {
		t.Fatalf("create integration tables: %v", err)
	}
	return ctx, database, clientA, clientB
}

func mustLockModelAdminParent(t *testing.T, ctx context.Context, database *sql.DB, tableName string, id int64) *sql.Tx {
	t.Helper()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin parent delete transaction: %v", err)
	}
	query := fmt.Sprintf(`SELECT id FROM %s WHERE id = $1 FOR UPDATE`, pq.QuoteIdentifier(tableName))
	if _, err := tx.ExecContext(ctx, query, id); err != nil {
		_ = tx.Rollback()
		t.Fatalf("lock %s parent: %v", tableName, err)
	}
	return tx
}

func mustCreateModelAdminAccount(t *testing.T, ctx context.Context, store *ModelAdminStore, name string) domainmodeladmin.ModelAccount {
	t.Helper()
	account, err := store.CreateModelAccount(ctx, domainmodeladmin.ModelAccountWriteRequest{
		Name: name, AdapterType: "openai_compatible", AuthType: "api_key", BaseURL: "https://example.com", Status: "enabled", ConcurrencyLimit: 1, TimeoutMS: 30000,
	})
	if err != nil {
		t.Fatalf("create account %s: %v", name, err)
	}
	return account
}

func mustCreateModelAdminRoute(t *testing.T, ctx context.Context, store *ModelAdminStore, code string) domainmodeladmin.RouteModel {
	t.Helper()
	route, err := store.CreateRouteModel(ctx, domainmodeladmin.RouteModelWriteRequest{Code: code, Name: code, Visibility: "public", Enabled: true})
	if err != nil {
		t.Fatalf("create route %s: %v", code, err)
	}
	return route
}

func modelAdminAccountModelRequest(accountID int64, code string) domainmodeladmin.ModelAccountModelWriteRequest {
	return domainmodeladmin.ModelAccountModelWriteRequest{
		AccountID: accountID, ModelCode: code, DisplayName: code, TaskTypes: []string{"text_to_image"}, SizeModes: []string{"auto"}, MaxImageCount: 1, CostPerImage: "0.10000", Currency: "USD", Enabled: true,
	}
}

func modelAdminPriceRequest(routeID int64, resolution string) domainmodeladmin.RouteModelPriceWriteRequest {
	return domainmodeladmin.RouteModelPriceWriteRequest{
		RouteModelID: routeID, TaskType: "text_to_image", BaseResolution: resolution, BasePoints: "1.00000", ReferenceMultiplier: "1.00000", Enabled: true,
	}
}
