package entstore

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/lib/pq"
)

func TestBillingStoreUpdatePlanPreservesLifecycleStateSQLite(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:billing-plan-update-lifecycle?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := NewBillingStore(client, 5)

	for _, testCase := range []struct {
		name            string
		status          string
		purchaseEnabled bool
	}{
		{name: "active", status: domainbilling.SubscriptionPlanStatusActive, purchaseEnabled: true},
		{name: "disabled", status: domainbilling.SubscriptionPlanStatusDisabled, purchaseEnabled: false},
		{name: "archived", status: domainbilling.SubscriptionPlanStatusArchived, purchaseEnabled: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			plan := mustCreateLifecyclePlan(t, ctx, store, "sqlite-"+testCase.name, testCase.status, testCase.purchaseEnabled)
			updated, err := store.UpdatePlan(ctx, lifecyclePlanUpdateRequest(plan.ID, "edited "+testCase.name))
			if err != nil {
				t.Fatalf("UpdatePlan: %v", err)
			}
			if updated.Status != testCase.status || updated.PurchaseEnabled != testCase.purchaseEnabled {
				t.Fatalf("UpdatePlan changed lifecycle state: %#v", updated)
			}
			if updated.PlanName != "edited "+testCase.name {
				t.Fatalf("UpdatePlan did not persist editable fields: %#v", updated)
			}
		})
	}
}

func TestBillingStoreUpdatePlanSerializesWithLifecycleTransitionsPostgres(t *testing.T) {
	for _, testCase := range []struct {
		name               string
		action             string
		initialStatus      string
		initialPurchase    bool
		transitionStatus   string
		transitionPurchase bool
	}{
		{name: "enable", action: domainbilling.SubscriptionPlanActionEnable, initialStatus: domainbilling.SubscriptionPlanStatusDisabled, initialPurchase: false, transitionStatus: domainbilling.SubscriptionPlanStatusActive, transitionPurchase: true},
		{name: "disable", action: domainbilling.SubscriptionPlanActionDisable, initialStatus: domainbilling.SubscriptionPlanStatusActive, initialPurchase: true, transitionStatus: domainbilling.SubscriptionPlanStatusDisabled, transitionPurchase: false},
		{name: "archive", action: domainbilling.SubscriptionPlanActionArchive, initialStatus: domainbilling.SubscriptionPlanStatusActive, initialPurchase: true, transitionStatus: domainbilling.SubscriptionPlanStatusArchived, transitionPurchase: false},
		{name: "restore", action: domainbilling.SubscriptionPlanActionRestore, initialStatus: domainbilling.SubscriptionPlanStatusArchived, initialPurchase: false, transitionStatus: domainbilling.SubscriptionPlanStatusDisabled, transitionPurchase: false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, clientA, clientB := openBillingPlanLockPostgres(t)
			storeA := NewBillingStore(clientA, 5)
			storeB := NewBillingStore(clientB, 5)
			plan := mustCreateLifecyclePlan(t, ctx, storeA, "postgres-"+testCase.name, testCase.initialStatus, testCase.initialPurchase)
			audit := domainbilling.PlanLifecycleAudit{
				ActorType: "admin", ActorID: "7", Action: "cashier.plan." + testCase.name, TargetType: "cashier_plan", TargetID: fmt.Sprint(plan.ID),
				RequestID: "plan-update-lock-" + testCase.name, IPAddr: "127.0.0.1", UserAgent: "billing-plan-lock-test",
			}
			transitionEntered := make(chan struct{}, 1)
			releaseTransition := make(chan struct{})
			clientA.SubscriptionPlan.Use(func(next repoent.Mutator) repoent.Mutator {
				return repoent.MutateFunc(func(ctx context.Context, mutation repoent.Mutation) (repoent.Value, error) {
					transitionEntered <- struct{}{}
					<-releaseTransition
					return next.Mutate(ctx, mutation)
				})
			})
			transitionDone := make(chan error, 1)
			go func() {
				_, err := storeA.TransitionPlanAudited(context.Background(), domainbilling.TransitionSubscriptionPlanRequest{PlanID: plan.ID, Action: testCase.action}, audit)
				transitionDone <- err
			}()
			select {
			case <-transitionEntered:
			case <-time.After(5 * time.Second):
				t.Fatal("lifecycle transition did not reach the locked mutation")
			}

			updateDone := make(chan error, 1)
			go func() {
				_, err := storeB.UpdatePlan(context.Background(), lifecyclePlanUpdateRequest(plan.ID, "edited "+testCase.name))
				updateDone <- err
			}()
			assertPostgresOperationBlocked(t, updateDone, "plan update while lifecycle transition holds FOR UPDATE")
			close(releaseTransition)
			if err := awaitPostgresOperation(t, transitionDone, testCase.name+" lifecycle transition"); err != nil {
				t.Fatalf("TransitionPlan %s: %v", testCase.name, err)
			}
			if err := awaitPostgresOperation(t, updateDone, "plan update after "+testCase.name); err != nil {
				t.Fatalf("UpdatePlan after %s: %v", testCase.name, err)
			}

			reloaded, err := clientA.SubscriptionPlan.Get(ctx, int(plan.ID))
			if err != nil {
				t.Fatalf("reload plan: %v", err)
			}
			if reloaded.Status != testCase.transitionStatus || reloaded.PurchaseEnabled != testCase.transitionPurchase {
				t.Fatalf("UpdatePlan overwrote %s lifecycle transition: status=%q purchase_enabled=%t", testCase.name, reloaded.Status, reloaded.PurchaseEnabled)
			}
			if reloaded.PlanName != "edited "+testCase.name {
				t.Fatalf("UpdatePlan did not persist editable fields: %#v", reloaded)
			}
			audits, err := clientA.AuditLog.Query().All(ctx)
			if err != nil {
				t.Fatalf("query lifecycle audit: %v", err)
			}
			if len(audits) != 1 || audits[0].Action != audit.Action || audits[0].TargetID != audit.TargetID || audits[0].Metadata["request_id"] != audit.RequestID {
				t.Fatalf("unexpected %s lifecycle audit: %#v", testCase.name, audits)
			}
		})
	}
}

func openBillingPlanLockPostgres(t *testing.T) (context.Context, *repoent.Client, *repoent.Client) {
	t.Helper()
	adminURL := strings.TrimSpace(os.Getenv("PIC_GALLERY_TEST_POSTGRES_URL"))
	if adminURL == "" {
		t.Skip("set PIC_GALLERY_TEST_POSTGRES_URL to run PostgreSQL billing plan lock integration")
	}
	ctx := context.Background()
	admin, err := sql.Open("postgres", adminURL)
	if err != nil {
		t.Fatalf("open integration database: %v", err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schemaName := fmt.Sprintf("billing_plan_lock_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+pq.QuoteIdentifier(schemaName)); err != nil {
		t.Fatalf("create integration schema: %v", err)
	}
	t.Cleanup(func() { _, _ = admin.Exec(`DROP SCHEMA IF EXISTS ` + pq.QuoteIdentifier(schemaName) + ` CASCADE`) })
	databaseURL := projectPostgresURLWithSearchPath(t, adminURL, schemaName)
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
	return ctx, clientA, clientB
}

func mustCreateLifecyclePlan(t *testing.T, ctx context.Context, store *BillingStore, code, status string, purchaseEnabled bool) domainbilling.SubscriptionPlan {
	t.Helper()
	durationDays := 30
	plan, err := store.CreatePlan(ctx, domainbilling.CreateSubscriptionPlanRequest{
		PlanCode: code, PlanName: code, PlanType: "points_package", Status: status, PurchaseEnabled: purchaseEnabled,
		PriceCNY: "10.00000", Points: "20.00000", BonusPoints: "0.00000", DurationDays: &durationDays, Currency: "CNY",
	})
	if err != nil {
		t.Fatalf("create lifecycle plan: %v", err)
	}
	return plan
}

func lifecyclePlanUpdateRequest(planID int64, name string) domainbilling.UpdateSubscriptionPlanRequest {
	durationDays := 45
	return domainbilling.UpdateSubscriptionPlanRequest{
		PlanID: planID, PlanName: name, PlanType: "points_package", PurchaseEnabled: true, Status: domainbilling.SubscriptionPlanStatusActive,
		PriceCNY: "12.00000", Points: "24.00000", BonusPoints: "2.00000", DurationDays: &durationDays, Currency: "CNY", Description: "edited",
	}
}
