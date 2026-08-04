package entstore

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	domainbilling "github.com/fatballfish/pic-gallery/internal/domain/billing"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/pointledger"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/walletgrant"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/walletreservationallocation"
	billingservice "github.com/fatballfish/pic-gallery/internal/service/billing"
	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/mattn/go-sqlite3"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestBillingStoreReserveFinalizeAndLedger(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 11, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("Adjust: %v", err)
	}
	if _, err := store.ReserveTask(ctx, billingservice.ReserveStoreRequest{UserID: 11, TaskID: "11111111-1111-1111-1111-111111111111", EstimatedPoints: "12.00000", Reason: "reserve"}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	if _, err := store.FinalizeTask(ctx, billingservice.FinalizeStoreRequest{UserID: 11, TaskID: "11111111-1111-1111-1111-111111111111", EstimatedPoints: "12.00000", ActualPoints: "8.00000", Reason: "finalize"}); err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}

	balance, err := store.GetBalance(ctx, 11)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "92.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("unexpected balance %#v", balance)
	}

	page, err := store.ListLedger(ctx, 11, 1, 10)
	if err != nil {
		t.Fatalf("ListLedger: %v", err)
	}
	if len(page.Items) != 4 {
		t.Fatalf("expected 4 ledger entries, got %d", len(page.Items))
	}
	if page.Items[0].LedgerType != "refund" || page.Items[1].LedgerType != "consume" || page.Items[2].LedgerType != "reserve" || page.Items[3].LedgerType != "admin_adjust" {
		t.Fatalf("unexpected ledger order %#v", page.Items)
	}
	if page.Items[1].ChangePoints != "-8.00000" {
		t.Fatalf("expected consume ledger to record actual spend, got %#v", page.Items[1])
	}
}

func TestBillingStoreBalanceBucketsAndTrialReservePriority(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-trial-buckets?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	trialExpires := time.Now().UTC().Add(48 * time.Hour)
	trialGrant, err := client.WalletGrant.Create().
		SetUserID(77).
		SetGrantType("trial").
		SetSourceType("signup").
		SetStatus("active").
		SetTotalPoints("20.00000").
		SetAvailablePoints("20.00000").
		SetFrozenPoints("0.00000").
		SetConsumedPoints("0.00000").
		SetExpiresAt(trialExpires).
		Save(ctx)
	if err != nil {
		t.Fatalf("create trial grant: %v", err)
	}
	if _, err := client.WalletGrant.Create().
		SetUserID(77).
		SetGrantType("recharge").
		SetSourceType("payment_order").
		SetStatus("active").
		SetTotalPoints("50.00000").
		SetAvailablePoints("50.00000").
		SetFrozenPoints("0.00000").
		SetConsumedPoints("0.00000").
		Save(ctx); err != nil {
		t.Fatalf("create recharge grant: %v", err)
	}

	balance, err := store.GetBalance(ctx, 77)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.TrialPoints != "20.00000" || balance.RechargePoints != "50.00000" || balance.GiftPoints != "20.00000" {
		t.Fatalf("expected trial and recharge bucket totals, got %#v", balance)
	}
	if len(balance.Buckets) != 2 {
		t.Fatalf("expected 2 balance buckets, got %#v", balance.Buckets)
	}
	if balance.Buckets[0].Bucket != "trial" || balance.Buckets[0].AvailablePoints != "20.00000" || balance.Buckets[0].ExpiresAt == nil || !balance.Buckets[0].ExpireWarning {
		t.Fatalf("unexpected trial bucket %#v", balance.Buckets[0])
	}
	if balance.Buckets[1].Bucket != "recharge" || balance.Buckets[1].AvailablePoints != "50.00000" || balance.Buckets[1].ExpiresAt != nil {
		t.Fatalf("unexpected recharge bucket %#v", balance.Buckets[1])
	}

	taskID := "77777777-7777-7777-7777-777777777777"
	if _, err := store.ReserveTask(ctx, billingservice.ReserveStoreRequest{UserID: 77, TaskID: taskID, EstimatedPoints: "12.00000", Reason: "reserve trial first"}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	taskUUID := uuid.MustParse(taskID)
	allocations, err := client.WalletReservationAllocation.Query().
		Where(walletreservationallocation.TaskIDEQ(taskUUID)).
		Order(repoent.Asc(walletreservationallocation.FieldID)).
		All(ctx)
	if err != nil {
		t.Fatalf("query allocations: %v", err)
	}
	if len(allocations) != 1 || allocations[0].WalletGrantID != int64(trialGrant.ID) || allocations[0].ReservedPoints != "12.00000" {
		t.Fatalf("expected reserve to use trial grant first, got %#v", allocations)
	}
}

func TestBillingStoreGetBalanceExpiresOldTrialAndSubscriptionGrants(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-expire-grants?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	expiredAt := time.Now().UTC().Add(-time.Hour)
	trialGrant, err := client.WalletGrant.Create().
		SetUserID(177).
		SetGrantType("trial").
		SetSourceType("signup").
		SetStatus("active").
		SetTotalPoints("20.00000").
		SetAvailablePoints("9.00000").
		SetFrozenPoints("0.00000").
		SetConsumedPoints("11.00000").
		SetExpiresAt(expiredAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create expired trial grant: %v", err)
	}
	subscriptionSourceID := int64(991)
	subscriptionGrant, err := client.WalletGrant.Create().
		SetUserID(177).
		SetGrantType("subscription").
		SetSourceType("payment_order").
		SetSourceID(subscriptionSourceID).
		SetStatus("active").
		SetTotalPoints("40.00000").
		SetAvailablePoints("5.00000").
		SetFrozenPoints("0.00000").
		SetConsumedPoints("35.00000").
		SetExpiresAt(expiredAt).
		Save(ctx)
	if err != nil {
		t.Fatalf("create expired subscription grant: %v", err)
	}
	if _, err := client.WalletGrant.Create().
		SetUserID(177).
		SetGrantType("recharge").
		SetSourceType("payment_order").
		SetStatus("active").
		SetTotalPoints("50.00000").
		SetAvailablePoints("50.00000").
		SetFrozenPoints("0.00000").
		SetConsumedPoints("0.00000").
		Save(ctx); err != nil {
		t.Fatalf("create recharge grant: %v", err)
	}

	balance, err := store.GetBalance(ctx, 177)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "50.00000" || balance.TrialPoints != "0.00000" || balance.SubscriptionPoints != "0.00000" || balance.RechargePoints != "50.00000" {
		t.Fatalf("expected only recharge balance after lazy expiry, got %#v", balance)
	}

	refreshedTrial, err := client.WalletGrant.Get(ctx, trialGrant.ID)
	if err != nil {
		t.Fatalf("get trial grant: %v", err)
	}
	refreshedSubscription, err := client.WalletGrant.Get(ctx, subscriptionGrant.ID)
	if err != nil {
		t.Fatalf("get subscription grant: %v", err)
	}
	if refreshedTrial.Status != "expired" || refreshedSubscription.Status != "expired" {
		t.Fatalf("expected expired statuses, got trial=%q subscription=%q", refreshedTrial.Status, refreshedSubscription.Status)
	}

	ledgers, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(177), pointledger.LedgerTypeEQ("expire")).
		Order(repoent.Asc(pointledger.FieldID)).
		All(ctx)
	if err != nil {
		t.Fatalf("query expire ledgers: %v", err)
	}
	if len(ledgers) != 2 {
		t.Fatalf("expected two expire ledgers, got %d", len(ledgers))
	}
	if ledgers[0].BalanceBucket != "trial" || ledgers[0].SourceType != "signup" || ledgers[0].ChangePoints != "-9.00000" || ledgers[0].BucketBalanceAfter != "0.00000" || ledgers[0].ExpiresAt == nil {
		t.Fatalf("unexpected trial expire ledger %#v", ledgers[0])
	}
	if ledgers[1].BalanceBucket != "subscription" || ledgers[1].SourceType != "payment_order" || ledgers[1].SourceID == nil || *ledgers[1].SourceID != subscriptionSourceID || ledgers[1].ChangePoints != "-5.00000" || ledgers[1].BucketBalanceAfter != "0.00000" || ledgers[1].ExpiresAt == nil {
		t.Fatalf("unexpected subscription expire ledger %#v", ledgers[1])
	}

	if _, err := store.GetBalance(ctx, 177); err != nil {
		t.Fatalf("GetBalance second: %v", err)
	}
	ledgerCount, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(177), pointledger.LedgerTypeEQ("expire")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count expire ledgers: %v", err)
	}
	if ledgerCount != 2 {
		t.Fatalf("expected expire ledger to be idempotent, got %d ledgers", ledgerCount)
	}
}

func TestBillingStoreEnsureSignupTrialGrantIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-signup-trial?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	req := billingservice.SignupTrialGrantStoreRequest{
		UserID:             78,
		Points:             "15.00000",
		ValidDays:          7,
		ExpiryReminderDays: 2,
		IdempotencyKey:     "signup_trial:78",
	}
	first, err := store.EnsureSignupTrialGrant(ctx, req)
	if err != nil {
		t.Fatalf("EnsureSignupTrialGrant first: %v", err)
	}
	if !first.Granted || first.Balance.TrialPoints != "15.00000" || first.Balance.AvailablePoints != "15.00000" {
		t.Fatalf("unexpected first signup trial result %#v", first)
	}
	second, err := store.EnsureSignupTrialGrant(ctx, req)
	if err != nil {
		t.Fatalf("EnsureSignupTrialGrant second: %v", err)
	}
	if second.Granted || second.Balance.TrialPoints != "15.00000" || second.Balance.AvailablePoints != "15.00000" {
		t.Fatalf("expected idempotent second signup trial result, got %#v", second)
	}
	ledgerCount, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(78), pointledger.LedgerTypeEQ("trial_grant")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count trial ledger: %v", err)
	}
	if ledgerCount != 1 {
		t.Fatalf("expected one trial grant ledger, got %d", ledgerCount)
	}
	grantCount, err := client.WalletGrant.Query().
		Where(walletgrant.UserIDEQ(78), walletgrant.GrantTypeEQ("trial"), walletgrant.SourceTypeEQ("signup")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count trial grants: %v", err)
	}
	if grantCount != 1 {
		t.Fatalf("expected one trial grant, got %d", grantCount)
	}
}

func TestBillingStoreSignupTrialExpiryWarningUsesGrantReminderDays(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-signup-trial-reminder?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	result, err := store.EnsureSignupTrialGrant(ctx, billingservice.SignupTrialGrantStoreRequest{
		UserID:             80,
		Points:             "15.00000",
		ValidDays:          5,
		ExpiryReminderDays: 6,
		IdempotencyKey:     "signup_trial:80",
	})
	if err != nil {
		t.Fatalf("EnsureSignupTrialGrant: %v", err)
	}
	if len(result.Balance.Buckets) != 1 || !result.Balance.Buckets[0].ExpireWarning {
		t.Fatalf("expected configured 6-day reminder to mark 5-day trial as expiring, got %#v", result.Balance.Buckets)
	}
}

func TestBillingStoreSignupTrialLedgerPersistsBucketMetadata(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-signup-trial-ledger-metadata?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.EnsureSignupTrialGrant(ctx, billingservice.SignupTrialGrantStoreRequest{
		UserID:             79,
		Points:             "15.00000",
		ValidDays:          7,
		ExpiryReminderDays: 2,
		IdempotencyKey:     "signup_trial:79",
	}); err != nil {
		t.Fatalf("EnsureSignupTrialGrant: %v", err)
	}
	ledger, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(79), pointledger.LedgerTypeEQ("trial_grant")).
		Only(ctx)
	if err != nil {
		t.Fatalf("query trial grant ledger: %v", err)
	}
	if ledger.BalanceBucket != "trial" || ledger.SourceType != "signup" || ledger.SourceID != nil {
		t.Fatalf("expected persisted trial ledger metadata, got bucket=%q source=%q source_id=%v", ledger.BalanceBucket, ledger.SourceType, ledger.SourceID)
	}
	if ledger.BucketBalanceAfter != "15.00000" {
		t.Fatalf("expected persisted bucket balance after 15.00000, got %q", ledger.BucketBalanceAfter)
	}
	if ledger.ExpiresAt == nil {
		t.Fatalf("expected trial ledger expires_at")
	}
}

func TestBillingStoreFinalizeIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-idem?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 22, ChangePoints: "20.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("Adjust: %v", err)
	}
	reserveReq := billingservice.ReserveStoreRequest{UserID: 22, TaskID: "22222222-2222-2222-2222-222222222222", EstimatedPoints: "6.00000", Reason: "reserve"}
	if _, err := store.ReserveTask(ctx, reserveReq); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	if _, err := store.ReserveTask(ctx, reserveReq); err != nil {
		t.Fatalf("ReserveTask second call: %v", err)
	}
	finalizeReq := billingservice.FinalizeStoreRequest{UserID: 22, TaskID: reserveReq.TaskID, EstimatedPoints: "6.00000", ActualPoints: "6.00000", Reason: "finalize"}
	if _, err := store.FinalizeTask(ctx, finalizeReq); err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}
	if _, err := store.FinalizeTask(ctx, finalizeReq); err != nil {
		t.Fatalf("FinalizeTask second call: %v", err)
	}

	balance, err := store.GetBalance(ctx, 22)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "14.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("unexpected balance %#v", balance)
	}

	page, err := store.ListLedger(ctx, 22, 1, 10)
	if err != nil {
		t.Fatalf("ListLedger: %v", err)
	}
	if len(page.Items) != 3 {
		t.Fatalf("expected 3 ledger entries, got %d", len(page.Items))
	}
}

func TestBillingStoreRejectsFinalizeWithoutReserve(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-noreserve?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 33, ChangePoints: "20.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("Adjust: %v", err)
	}
	if _, err := store.FinalizeTask(ctx, billingservice.FinalizeStoreRequest{
		UserID:          33,
		TaskID:          "33333333-3333-3333-3333-333333333333",
		EstimatedPoints: "6.00000",
		ActualPoints:    "6.00000",
		Reason:          "finalize",
	}); err == nil {
		t.Fatal("expected finalize without reserve to fail")
	}
}

func TestBillingStoreFinalizeUsesReservedAmountInsteadOfCallerEstimate(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-reserved-amount?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 45, ChangePoints: "10.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("Adjust: %v", err)
	}
	if _, err := store.ReserveTask(ctx, billingservice.ReserveStoreRequest{UserID: 45, TaskID: "45454545-4545-4545-4545-454545454545", EstimatedPoints: "6.00000", Reason: "reserve"}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	if _, err := store.FinalizeTask(ctx, billingservice.FinalizeStoreRequest{
		UserID:          45,
		TaskID:          "45454545-4545-4545-4545-454545454545",
		EstimatedPoints: "4.00000",
		ActualPoints:    "4.00000",
		Reason:          "finalize mismatched estimate",
	}); err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}

	balance, err := store.GetBalance(ctx, 45)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "6.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("expected finalize to settle full reserve, got %#v", balance)
	}

	page, err := store.ListLedger(ctx, 45, 1, 10)
	if err != nil {
		t.Fatalf("ListLedger: %v", err)
	}
	if len(page.Items) != 4 || page.Items[0].LedgerType != "refund" || page.Items[0].ChangePoints != "2.00000" || page.Items[1].LedgerType != "consume" || page.Items[1].ChangePoints != "-4.00000" {
		t.Fatalf("unexpected ledger settlement %#v", page.Items)
	}
}

func TestBillingStoreRejectsFinalizeForDifferentUser(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-wrong-user?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 55, ChangePoints: "20.00000", Reason: "seed owner"}); err != nil {
		t.Fatalf("Adjust owner: %v", err)
	}
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 56, ChangePoints: "20.00000", Reason: "seed intruder"}); err != nil {
		t.Fatalf("Adjust intruder: %v", err)
	}
	if _, err := store.ReserveTask(ctx, billingservice.ReserveStoreRequest{UserID: 55, TaskID: "55555555-5555-5555-5555-555555555555", EstimatedPoints: "6.00000", Reason: "reserve"}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}

	if _, err := store.FinalizeTask(ctx, billingservice.FinalizeStoreRequest{
		UserID:          56,
		TaskID:          "55555555-5555-5555-5555-555555555555",
		EstimatedPoints: "6.00000",
		ActualPoints:    "6.00000",
		Reason:          "wrong user finalize",
	}); err == nil {
		t.Fatal("expected wrong-user finalize to fail")
	} else {
		appErr, ok := err.(*errs.Error)
		if !ok || appErr.Code != errs.CodeConflict {
			t.Fatalf("expected conflict error, got %T %v", err, err)
		}
	}

	ownerBalance, err := store.GetBalance(ctx, 55)
	if err != nil {
		t.Fatalf("GetBalance owner: %v", err)
	}
	if ownerBalance.AvailablePoints != "14.00000" || ownerBalance.FrozenPoints != "6.00000" {
		t.Fatalf("expected owner reserve to remain intact, got %#v", ownerBalance)
	}

	intruderBalance, err := store.GetBalance(ctx, 56)
	if err != nil {
		t.Fatalf("GetBalance intruder: %v", err)
	}
	if intruderBalance.AvailablePoints != "20.00000" || intruderBalance.FrozenPoints != "0.00000" {
		t.Fatalf("expected intruder balance unchanged, got %#v", intruderBalance)
	}
}

func TestBillingStoreFinalizeSupportsZeroPointReserve(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-zero-reserve?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 57, ChangePoints: "5.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("Adjust: %v", err)
	}
	if _, err := store.ReserveTask(ctx, billingservice.ReserveStoreRequest{UserID: 57, TaskID: "57575757-5757-5757-5757-575757575757", EstimatedPoints: "0.00000", Reason: "reserve zero"}); err != nil {
		t.Fatalf("ReserveTask: %v", err)
	}
	if _, err := store.FinalizeTask(ctx, billingservice.FinalizeStoreRequest{
		UserID:          57,
		TaskID:          "57575757-5757-5757-5757-575757575757",
		EstimatedPoints: "0.00000",
		ActualPoints:    "0.00000",
		Reason:          "finalize zero",
	}); err != nil {
		t.Fatalf("FinalizeTask: %v", err)
	}

	balance, err := store.GetBalance(ctx, 57)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "5.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("expected zero-point reserve to settle cleanly, got %#v", balance)
	}
}

func TestBillingStoreConcurrentReserveAndFinalizeRemainIdempotent(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-concurrent-idem?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 58, ChangePoints: "20.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("Adjust: %v", err)
	}
	reserveReq := billingservice.ReserveStoreRequest{UserID: 58, TaskID: "58585858-5858-5858-5858-585858585858", EstimatedPoints: "6.00000", Reason: "reserve"}
	runConcurrent := func(fn func() error) {
		t.Helper()
		start := make(chan struct{})
		errCh := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				errCh <- fn()
			}()
		}
		close(start)
		wg.Wait()
		close(errCh)
		for err := range errCh {
			if err != nil {
				t.Fatalf("expected concurrent duplicate call to collapse idempotently, got %v", err)
			}
		}
	}

	runConcurrent(func() error {
		_, err := store.ReserveTask(ctx, reserveReq)
		return err
	})

	balance, err := store.GetBalance(ctx, 58)
	if err != nil {
		t.Fatalf("GetBalance after reserve: %v", err)
	}
	if balance.AvailablePoints != "14.00000" || balance.FrozenPoints != "6.00000" {
		t.Fatalf("expected single reserve after concurrent retries, got %#v", balance)
	}

	finalizeReq := billingservice.FinalizeStoreRequest{UserID: 58, TaskID: reserveReq.TaskID, EstimatedPoints: "6.00000", ActualPoints: "4.00000", Reason: "finalize"}
	runConcurrent(func() error {
		_, err := store.FinalizeTask(ctx, finalizeReq)
		return err
	})

	balance, err = store.GetBalance(ctx, 58)
	if err != nil {
		t.Fatalf("GetBalance after finalize: %v", err)
	}
	if balance.AvailablePoints != "16.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("expected single finalize settlement after concurrent retries, got %#v", balance)
	}
}

func TestBillingStoreAPIKeyQuotaConcurrentReserveIsAtomic(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-apikey-quota-concurrent?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 68, ChangePoints: "100.00000", Reason: "seed balance"}); err != nil {
		t.Fatalf("Adjust: %v", err)
	}

	totalQuota := "16.00000"
	dailyQuota := "16.00000"
	dayStart := time.Now()
	const workers = 8
	start := make(chan struct{})
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, err := store.ReserveTask(ctx, billingservice.ReserveStoreRequest{
				UserID:          68,
				APIKeyID:        9001,
				TaskID:          uuid.NewString(),
				EstimatedPoints: "8.00000",
				Reason:          "reserve with api key quota",
				APIKeyQuota: domainbilling.APIKeyQuota{
					APIKeyTotalQuotaPoints: &totalQuota,
					APIKeyDailyQuotaPoints: &dailyQuota,
					APIKeyQuotaDayStart:    &dayStart,
				},
			})
			errCh <- err
		}()
	}
	close(start)
	wg.Wait()
	close(errCh)

	successes := 0
	rateLimited := 0
	for err := range errCh {
		switch {
		case err == nil:
			successes++
		default:
			appErr, ok := err.(*errs.Error)
			if !ok || appErr.StatusCode != 429 || appErr.Code != errs.CodeRateLimited {
				t.Fatalf("expected only quota 429 errors, got %T %v", err, err)
			}
			rateLimited++
		}
	}
	if successes != 2 || rateLimited != workers-2 {
		t.Fatalf("expected exactly 2 successes and %d quota failures, got successes=%d failures=%d", workers-2, successes, rateLimited)
	}
	balance, err := store.GetBalance(ctx, 68)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "84.00000" || balance.FrozenPoints != "16.00000" {
		t.Fatalf("expected only quota-covered reserves to affect balance, got %#v", balance)
	}
	totalUsed, err := store.APIKeyUsage(ctx, 9001, nil)
	if err != nil {
		t.Fatalf("APIKeyUsage total: %v", err)
	}
	dailyUsed, err := store.APIKeyUsage(ctx, 9001, &dayStart)
	if err != nil {
		t.Fatalf("APIKeyUsage daily: %v", err)
	}
	if totalUsed != "16.00000" || dailyUsed != "16.00000" {
		t.Fatalf("expected usage to stop at quota, total=%s daily=%s", totalUsed, dailyUsed)
	}
}

func TestIsRetryableTxErrRecognizesSQLiteTableLock(t *testing.T) {
	if !isRetryableTxErr(errors.New("database table is locked: point_ledgers")) {
		t.Fatal("expected sqlite table lock error to be retryable")
	}

	sqliteLock := sqlite3.Error{Code: sqlite3.ErrLocked, ExtendedCode: sqlite3.ErrLockedSharedCache}
	if !isRetryableTxErr(fmt.Errorf("ent insert point_ledgers: %w", sqliteLock)) {
		t.Fatal("expected wrapped sqlite locked error to be retryable")
	}
}

func TestIsRetryableTxErrRecognizesPostgresSerializationCodes(t *testing.T) {
	if !isRetryableTxErr(fmt.Errorf("commit billing tx: %w", &pq.Error{Code: "40001"})) {
		t.Fatal("expected wrapped postgres serialization failure to be retryable")
	}
	if !isRetryableTxErr(fmt.Errorf("commit billing tx: %w", &pq.Error{Code: "40P01"})) {
		t.Fatal("expected wrapped postgres deadlock to be retryable")
	}
}

func TestBillingStoreRejectsNegativeFinalizeEstimate(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-negative-finalize?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	if _, err := store.FinalizeTask(ctx, billingservice.FinalizeStoreRequest{
		UserID:          44,
		TaskID:          "44444444-4444-4444-4444-444444444444",
		EstimatedPoints: "-1.00000",
		ActualPoints:    "0.00000",
		Reason:          "finalize",
	}); err == nil {
		t.Fatal("expected negative estimate to be rejected")
	}
}

func TestMapLedgerEntryIncludesTaskID(t *testing.T) {
	taskID := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	entry := mapLedgerEntry(&repoent.PointLedger{ID: 1, LedgerType: "reserve", ChangePoints: "-1.00000", BalanceAfter: "9.00000", FrozenAfter: "1.00000", Reason: "reserve", TaskID: &taskID})
	if entry.TaskID == "" {
		t.Fatal("expected task id to be mapped")
	}
}

func TestBillingStoreAlwaysFormatsFiveDecimals(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-fixed-scale?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 2)
	if _, err := store.Adjust(ctx, billingservice.AdjustStoreRequest{UserID: 99, ChangePoints: "1", Reason: "seed balance"}); err != nil {
		t.Fatalf("Adjust: %v", err)
	}
	balance, err := store.GetBalance(ctx, 99)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.AvailablePoints != "1.00000" || balance.FrozenPoints != "0.00000" {
		t.Fatalf("expected fixed 5-decimal formatting, got %#v", balance)
	}
	page, err := store.ListLedger(ctx, 99, 1, 10)
	if err != nil {
		t.Fatalf("ListLedger: %v", err)
	}
	if len(page.Items) != 1 || page.Items[0].ChangePoints != "1.00000" || page.Items[0].BalanceAfter != "1.00000" || page.Items[0].FrozenAfter != "0.00000" {
		t.Fatalf("expected fixed 5-decimal ledger formatting, got %#v", page.Items)
	}
}

func TestBillingStorePersistsPaymentOrderChannelFields(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-payment-order-channel-fields?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateOrder(ctx, domainbilling.CreateOrderRequest{
		UserID:             88,
		PlanCode:           "basic-monthly",
		Provider:           "alipay_direct",
		PurchaseType:       "plan",
		VisibleMethod:      "alipay",
		ProviderType:       "alipay_direct",
		ProviderInstanceID: 12,
		PaymentDisplay:     map[string]any{"type": "qr_code", "qr_code": "https://qr.example/order"},
		PaymentURL:         "https://pay.example.com/order",
		QRCode:             "https://qr.example/order",
		ClientToken:        "client-token",
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if order.VisibleMethod != "alipay" || order.ProviderType != "alipay_direct" || order.ProviderInstanceID != 12 {
		t.Fatalf("expected order response channel fields, got %#v", order)
	}
	if order.PaymentURL != "https://pay.example.com/order" || order.QRCode != "https://qr.example/order" || order.ClientToken != "client-token" {
		t.Fatalf("expected order response payment display fields, got %#v", order)
	}

	entity, err := client.PaymentOrder.Get(ctx, int(order.ID))
	if err != nil {
		t.Fatalf("get payment order entity: %v", err)
	}
	if entity.PurchaseType != "plan" || entity.VisibleMethod != "alipay" || entity.ProviderType != "alipay_direct" || entity.ProviderInstanceID == nil || *entity.ProviderInstanceID != 12 {
		t.Fatalf("expected payment_orders columns to persist channel fields, got %#v", entity)
	}
	if entity.PaymentDisplay["type"] != "qr_code" || entity.ProviderSnapshot["provider_type"] != "alipay_direct" {
		t.Fatalf("expected display and provider snapshot columns, display=%#v snapshot=%#v", entity.PaymentDisplay, entity.ProviderSnapshot)
	}
	if entity.PaymentURL == nil || *entity.PaymentURL != "https://pay.example.com/order" || entity.QrCode == nil || *entity.QrCode != "https://qr.example/order" || entity.ClientToken == nil || *entity.ClientToken != "client-token" {
		t.Fatalf("expected payment url, qr code and client token columns, got %#v", entity)
	}
}

func TestBillingStoreCreateOrderReusesIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-payment-order-idempotency?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	req := domainbilling.CreateOrderRequest{
		UserID:         188,
		PlanCode:       "basic-monthly",
		Provider:       "mock",
		PurchaseType:   "plan",
		VisibleMethod:  "mock",
		ProviderType:   "mock",
		PaymentDisplay: map[string]any{"type": "mock"},
		IdempotencyKey: "cashier-create-once",
	}
	first, err := store.CreateOrder(ctx, req)
	if err != nil {
		t.Fatalf("CreateOrder first: %v", err)
	}
	second, err := store.CreateOrder(ctx, req)
	if err != nil {
		t.Fatalf("CreateOrder second: %v", err)
	}
	if second.ID != first.ID || second.OrderNo != first.OrderNo || second.IdempotencyKey != "cashier-create-once" {
		t.Fatalf("expected idempotent create to reuse first order, first=%#v second=%#v", first, second)
	}
	count, err := client.PaymentOrder.Query().Count(ctx)
	if err != nil {
		t.Fatalf("count payment orders: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected one persisted payment order, got %d", count)
	}
}

func TestBillingStoreInitializationFailureDoesNotOverwriteCompletedCallback(t *testing.T) {
	ctx := context.Background()
	const dsn = "file:billingstore-initialization-callback-race?mode=memory&cache=shared&_fk=1"
	client, err := repoent.Open(dialect.SQLite, dsn)
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	callbackClient, err := repoent.Open(dialect.SQLite, dsn)
	if err != nil {
		t.Fatalf("open callback ent client: %v", err)
	}
	defer callbackClient.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateCustomAmountOrder(ctx, domainbilling.CreateCustomAmountOrderRequest{
		UserID: 188, OrderNo: "PGO-INIT-CALLBACK-RACE", AmountCNY: "12.50000", CNYPerPoint: "0.31250",
		Provider: "jeepay_alipay", PurchaseType: "custom_amount", VisibleMethod: "alipay", ProviderType: "jeepay_alipay", ProviderInstanceID: 8,
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}

	var callbackOnce sync.Once
	client.Use(func(next repoent.Mutator) repoent.Mutator {
		return repoent.MutateFunc(func(ctx context.Context, mutation repoent.Mutation) (repoent.Value, error) {
			if paymentMutation, ok := mutation.(*repoent.PaymentOrderMutation); ok {
				if status, exists := paymentMutation.Status(); exists && status == "failed" {
					callbackOnce.Do(func() {
						if _, updateErr := callbackClient.PaymentOrder.UpdateOneID(int(order.ID)).SetStatus("completed").Save(ctx); updateErr != nil {
							t.Fatalf("simulate completed callback: %v", updateErr)
						}
					})
				}
			}
			return next.Mutate(ctx, mutation)
		})
	})

	result, err := store.FailPaymentOrderInitialization(ctx, domainbilling.FailPaymentOrderInitializationRequest{
		UserID: order.UserID, OrderID: order.ID, FailureReason: errs.CodePaymentProviderUnavailable,
	})
	if err != nil {
		t.Fatalf("FailPaymentOrderInitialization: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("provider failure must not overwrite a completed callback: %#v", result)
	}
}

func TestBillingStoreCompleteRechargeOrderCompletesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-complete-recharge-order?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateOrder(ctx, domainbilling.CreateOrderRequest{
		UserID:         89,
		PlanCode:       "basic-monthly",
		Provider:       "mock",
		PurchaseType:   "plan",
		VisibleMethod:  "mock",
		ProviderType:   "mock",
		PaymentDisplay: map[string]any{"type": "mock"},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	first, err := store.CompleteRechargeOrder(ctx, domainbilling.CompleteRechargeOrderRequest{
		UserID:   89,
		OrderID:  order.ID,
		Provider: "mock",
		TradeNo:  "mock-trade-1",
	})
	if err != nil {
		t.Fatalf("CompleteRechargeOrder first: %v", err)
	}
	if first.Status != "completed" || first.PaidAt == nil || first.CompletedAt == nil || first.LedgerID == 0 {
		t.Fatalf("expected completed order with ledger id, got %#v", first)
	}

	second, err := store.CompleteRechargeOrder(ctx, domainbilling.CompleteRechargeOrderRequest{
		UserID:   89,
		OrderID:  order.ID,
		Provider: "mock",
		TradeNo:  "mock-trade-1",
	})
	if err != nil {
		t.Fatalf("CompleteRechargeOrder second: %v", err)
	}
	if second.Status != "completed" || second.LedgerID != first.LedgerID {
		t.Fatalf("expected idempotent completed order, got %#v", second)
	}

	balance, err := store.GetBalance(ctx, 89)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.RechargePoints != "100.00000" || balance.AvailablePoints != "100.00000" {
		t.Fatalf("expected one recharge grant after repeated completion, got %#v", balance)
	}
	ledgerCount, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(89), pointledger.LedgerTypeEQ("recharge")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count recharge ledger: %v", err)
	}
	if ledgerCount != 1 {
		t.Fatalf("expected one recharge ledger, got %d", ledgerCount)
	}
	ledger, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(89), pointledger.LedgerTypeEQ("recharge")).
		Only(ctx)
	if err != nil {
		t.Fatalf("load recharge ledger: %v", err)
	}
	if ledger.BalanceBucket != "recharge" || ledger.SourceType != "payment_order" || ledger.SourceID == nil || *ledger.SourceID != order.ID {
		t.Fatalf("expected persisted recharge ledger metadata, got bucket=%q source=%q source_id=%v", ledger.BalanceBucket, ledger.SourceType, ledger.SourceID)
	}
	if ledger.BucketBalanceAfter != "100.00000" || ledger.ExpiresAt != nil {
		t.Fatalf("expected recharge ledger bucket balance 100 and no expiry, got bucket_after=%q expires=%v", ledger.BucketBalanceAfter, ledger.ExpiresAt)
	}
	entity, err := client.PaymentOrder.Get(ctx, int(order.ID))
	if err != nil {
		t.Fatalf("get payment order entity: %v", err)
	}
	if entity.Status != "completed" || entity.CompletedAt == nil || entity.LedgerID == nil || *entity.LedgerID != first.LedgerID {
		t.Fatalf("expected completed entity with ledger id, got %#v", entity)
	}
}

func TestBillingStoreRecordChargebackSummaryPersistsOnOrder(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-chargeback-summary?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateCustomAmountOrder(ctx, domainbilling.CreateCustomAmountOrderRequest{
		UserID:         188,
		AmountCNY:      "10.00000",
		CNYPerPoint:    "0.50000",
		Provider:       "mock",
		PurchaseType:   "custom_amount",
		VisibleMethod:  "mock",
		ProviderType:   "mock",
		PaymentDisplay: map[string]any{"type": "mock"},
		IdempotencyKey: "chargeback-summary-order",
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}
	if _, err := store.CompleteRechargeOrder(ctx, domainbilling.CompleteRechargeOrderRequest{
		UserID:   188,
		OrderID:  order.ID,
		Provider: "mock",
		TradeNo:  "CHARGEBACK-SUMMARY-TRADE",
	}); err != nil {
		t.Fatalf("CompleteRechargeOrder: %v", err)
	}

	updated, err := store.RecordChargebackSummary(ctx, billingservice.ChargebackSummaryStoreRequest{
		OrderID:        order.ID,
		ChargePoints:   "5.00000",
		Reason:         "provider dispute accepted",
		IdempotencyKey: "chargeback-summary-once",
	})
	if err != nil {
		t.Fatalf("RecordChargebackSummary: %v", err)
	}
	if updated.ChargebackPoints != "5.00000" || updated.ChargebackReason != "provider dispute accepted" || updated.ChargebackKey != "chargeback-summary-once" || updated.ChargebackAt == nil {
		t.Fatalf("unexpected chargeback summary on updated order %#v", updated)
	}

	reloaded, err := store.GetOrderForAdmin(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrderForAdmin: %v", err)
	}
	if reloaded.ChargebackPoints != "5.00000" || reloaded.ChargebackReason != "provider dispute accepted" || reloaded.ChargebackKey != "chargeback-summary-once" || reloaded.ChargebackAt == nil {
		t.Fatalf("expected chargeback summary to persist on order detail, got %#v", reloaded)
	}
}

func TestBillingStoreRefundPaymentOrderDeductsRechargeGrantAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-refund-payment-order?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateOrder(ctx, domainbilling.CreateOrderRequest{
		UserID:         91,
		PlanCode:       "basic-monthly",
		Provider:       "mock",
		PurchaseType:   "plan",
		VisibleMethod:  "mock",
		ProviderType:   "mock",
		PaymentDisplay: map[string]any{"type": "mock"},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}
	if _, err := store.CompleteRechargeOrder(ctx, domainbilling.CompleteRechargeOrderRequest{
		UserID:   91,
		OrderID:  order.ID,
		Provider: "mock",
		TradeNo:  "mock-trade-refund-1",
	}); err != nil {
		t.Fatalf("CompleteRechargeOrder: %v", err)
	}

	first, err := store.RefundPaymentOrder(ctx, domainbilling.RefundPaymentOrderRequest{
		UserID:          91,
		OrderID:         order.ID,
		RefundTradeNo:   "refund-trade-1",
		Reason:          "customer requested refund",
		OperatorAdminID: 7001,
	})
	if err != nil {
		t.Fatalf("RefundPaymentOrder first: %v", err)
	}
	if first.Status != "refunded" || first.RefundedAt == nil || first.RefundTradeNo != "refund-trade-1" {
		t.Fatalf("expected refunded order with refund trade no, got %#v", first)
	}
	balance, err := store.GetBalance(ctx, 91)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.RechargePoints != "0.00000" || balance.AvailablePoints != "0.00000" {
		t.Fatalf("expected refund to remove recharge balance, got %#v", balance)
	}
	refundedGrantCount, err := client.WalletGrant.Query().
		Where(walletgrant.UserIDEQ(91), walletgrant.SourceTypeEQ("payment_order"), walletgrant.SourceIDEQ(order.ID), walletgrant.StatusEQ("refunded")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count refunded grant: %v", err)
	}
	if refundedGrantCount != 1 {
		t.Fatalf("expected one refunded recharge grant, got %d", refundedGrantCount)
	}
	refundLedgerCount, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(91), pointledger.OrderIDEQ(order.ID), pointledger.LedgerTypeEQ("payment_refund")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count refund ledger: %v", err)
	}
	if refundLedgerCount != 1 {
		t.Fatalf("expected one refund ledger, got %d", refundLedgerCount)
	}
	refundLedger, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(91), pointledger.OrderIDEQ(order.ID), pointledger.LedgerTypeEQ("payment_refund")).
		Only(ctx)
	if err != nil {
		t.Fatalf("load refund ledger: %v", err)
	}
	if refundLedger.ChangePoints != "-100.00000" || refundLedger.BalanceAfter != "0.00000" || refundLedger.OperatorAdminID == nil || *refundLedger.OperatorAdminID != 7001 {
		t.Fatalf("unexpected refund ledger %#v", refundLedger)
	}
	if refundLedger.BalanceBucket != "recharge" || refundLedger.SourceType != "payment_order" || refundLedger.SourceID == nil || *refundLedger.SourceID != order.ID {
		t.Fatalf("expected persisted refund ledger metadata, got bucket=%q source=%q source_id=%v", refundLedger.BalanceBucket, refundLedger.SourceType, refundLedger.SourceID)
	}
	if refundLedger.BucketBalanceAfter != "0.00000" || refundLedger.ExpiresAt != nil {
		t.Fatalf("expected refund ledger bucket balance 0 and no expiry, got bucket_after=%q expires=%v", refundLedger.BucketBalanceAfter, refundLedger.ExpiresAt)
	}

	second, err := store.RefundPaymentOrder(ctx, domainbilling.RefundPaymentOrderRequest{
		UserID:          91,
		OrderID:         order.ID,
		RefundTradeNo:   "refund-trade-1",
		Reason:          "customer requested refund",
		OperatorAdminID: 7001,
	})
	if err != nil {
		t.Fatalf("RefundPaymentOrder second: %v", err)
	}
	if second.Status != "refunded" || second.RefundTradeNo != "refund-trade-1" {
		t.Fatalf("expected idempotent refunded order, got %#v", second)
	}
	refundLedgerCountAfterReplay, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(91), pointledger.OrderIDEQ(order.ID), pointledger.LedgerTypeEQ("payment_refund")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count refund ledger after replay: %v", err)
	}
	if refundLedgerCountAfterReplay != 1 {
		t.Fatalf("expected refund replay not to add ledger, got %d", refundLedgerCountAfterReplay)
	}
}

func TestBillingStoreRefundPaymentOrderSupportsPartialRefunds(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-partial-refund-payment-order?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateCustomAmountOrder(ctx, domainbilling.CreateCustomAmountOrderRequest{
		UserID:         191,
		AmountCNY:      "12.50000",
		CNYPerPoint:    "1.00000",
		Provider:       "mock",
		PurchaseType:   "custom_amount",
		VisibleMethod:  "mock",
		ProviderType:   "mock",
		PaymentDisplay: map[string]any{"type": "mock"},
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}
	if _, err := store.CompleteRechargeOrder(ctx, domainbilling.CompleteRechargeOrderRequest{
		UserID:   191,
		OrderID:  order.ID,
		Provider: "mock",
		TradeNo:  "mock-trade-partial-refund-1",
	}); err != nil {
		t.Fatalf("CompleteRechargeOrder: %v", err)
	}

	first, err := store.RefundPaymentOrder(ctx, domainbilling.RefundPaymentOrderRequest{
		UserID:          191,
		OrderID:         order.ID,
		RefundTradeNo:   "refund-partial-1",
		RefundAmountCNY: "5.00000",
		Reason:          "partial customer refund",
		OperatorAdminID: 7101,
	})
	if err != nil {
		t.Fatalf("RefundPaymentOrder first partial: %v", err)
	}
	if first.Status != "partially_refunded" || first.RefundTradeNo != "refund-partial-1" || first.RefundedAmountCNY != "5.00000" || first.RefundedPoints != "5.00000" || first.RefundedAt != nil {
		t.Fatalf("expected partially refunded order after first refund, got %#v", first)
	}
	balance, err := store.GetBalance(ctx, 191)
	if err != nil {
		t.Fatalf("GetBalance after first partial refund: %v", err)
	}
	if balance.AvailablePoints != "7.50000" || balance.RechargePoints != "7.50000" {
		t.Fatalf("expected first partial refund to leave 7.5 recharge points, got %#v", balance)
	}
	firstReplay, err := store.RefundPaymentOrder(ctx, domainbilling.RefundPaymentOrderRequest{
		UserID:          191,
		OrderID:         order.ID,
		RefundTradeNo:   "refund-partial-1",
		RefundAmountCNY: "5.00000",
		Reason:          "partial customer refund replay",
		OperatorAdminID: 7101,
	})
	if err != nil {
		t.Fatalf("RefundPaymentOrder first replay: %v", err)
	}
	if firstReplay.RefundedAmountCNY != "5.00000" || firstReplay.RefundedPoints != "5.00000" {
		t.Fatalf("expected first replay to be idempotent, got %#v", firstReplay)
	}

	second, err := store.RefundPaymentOrder(ctx, domainbilling.RefundPaymentOrderRequest{
		UserID:          191,
		OrderID:         order.ID,
		RefundTradeNo:   "refund-partial-2",
		RefundAmountCNY: "7.50000",
		Reason:          "remaining customer refund",
		OperatorAdminID: 7102,
	})
	if err != nil {
		t.Fatalf("RefundPaymentOrder second partial: %v", err)
	}
	if second.Status != "refunded" || second.RefundTradeNo != "refund-partial-2" || second.RefundedAmountCNY != "12.50000" || second.RefundedPoints != "12.50000" || second.RefundedAt == nil {
		t.Fatalf("expected fully refunded order after second refund, got %#v", second)
	}
	finalBalance, err := store.GetBalance(ctx, 191)
	if err != nil {
		t.Fatalf("GetBalance after second partial refund: %v", err)
	}
	if finalBalance.AvailablePoints != "0.00000" || finalBalance.RechargePoints != "0.00000" {
		t.Fatalf("expected final partial refund to clear recharge points, got %#v", finalBalance)
	}
	refundLedgerCount, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(191), pointledger.OrderIDEQ(order.ID), pointledger.LedgerTypeEQ("payment_refund")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count partial refund ledgers: %v", err)
	}
	if refundLedgerCount != 2 {
		t.Fatalf("expected two refund ledgers for two unique partial refunds, got %d", refundLedgerCount)
	}
	grant, err := client.WalletGrant.Query().
		Where(walletgrant.UserIDEQ(191), walletgrant.SourceTypeEQ("payment_order"), walletgrant.SourceIDEQ(order.ID)).
		Only(ctx)
	if err != nil {
		t.Fatalf("load partially refunded grant: %v", err)
	}
	if grant.Status != "refunded" || grant.AvailablePoints != "0.00000" {
		t.Fatalf("expected grant fully refunded after second partial refund, got %#v", grant)
	}
}

func TestBillingStoreRefundPaymentOrderCanCompleteFromFrozenRefundGrant(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-refund-freeze-payment-order?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateCustomAmountOrder(ctx, domainbilling.CreateCustomAmountOrderRequest{
		UserID:         92,
		AmountCNY:      "12.50000",
		CNYPerPoint:    "1.00000",
		Provider:       "mock",
		PurchaseType:   "custom_amount",
		VisibleMethod:  "mock",
		ProviderType:   "mock",
		PaymentDisplay: map[string]any{"type": "mock"},
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}
	if _, err := store.CompleteRechargeOrder(ctx, domainbilling.CompleteRechargeOrderRequest{
		UserID:   92,
		OrderID:  order.ID,
		Provider: "mock",
		TradeNo:  "mock-trade-refund-freeze-1",
	}); err != nil {
		t.Fatalf("CompleteRechargeOrder: %v", err)
	}

	freezeReq := domainbilling.RefundPaymentOrderRequest{
		UserID:          92,
		OrderID:         order.ID,
		RefundTradeNo:   "refund-trade-freeze-1",
		Reason:          "customer requested refund",
		OperatorAdminID: 7002,
	}
	if _, err := store.FreezeRefundPaymentOrder(ctx, freezeReq); err != nil {
		t.Fatalf("FreezeRefundPaymentOrder: %v", err)
	}
	frozenBalance, err := store.GetBalance(ctx, 92)
	if err != nil {
		t.Fatalf("GetBalance after freeze: %v", err)
	}
	if frozenBalance.AvailablePoints != "0.00000" || frozenBalance.FrozenPoints != "12.50000" || frozenBalance.RechargePoints != "0.00000" {
		t.Fatalf("expected refund freeze to move recharge grant to frozen, got %#v", frozenBalance)
	}
	if _, err := store.CheckRefundPaymentOrder(ctx, freezeReq); err != nil {
		t.Fatalf("CheckRefundPaymentOrder should allow existing refund freeze: %v", err)
	}
	changedFreezeReq := freezeReq
	changedFreezeReq.RefundAmountCNY = "10.00000"
	for name, check := range map[string]func() error{
		"check": func() error {
			_, err := store.CheckRefundPaymentOrder(ctx, changedFreezeReq)
			return err
		},
		"freeze": func() error {
			_, err := store.FreezeRefundPaymentOrder(ctx, changedFreezeReq)
			return err
		},
	} {
		t.Run("changed refund amount is rejected by "+name, func(t *testing.T) {
			err := check()
			var appErr *errs.Error
			if !errors.As(err, &appErr) || appErr.Code != errs.CodePaymentAmountMismatch {
				t.Fatalf("expected PAYMENT_AMOUNT_MISMATCH, got %T %v", err, err)
			}
		})
	}
	grant, err := client.WalletGrant.Query().
		Where(walletgrant.UserIDEQ(92), walletgrant.SourceTypeEQ("payment_order"), walletgrant.SourceIDEQ(order.ID), walletgrant.StatusEQ("active")).
		Only(ctx)
	if err != nil {
		t.Fatalf("load frozen grant: %v", err)
	}
	if grant.Metadata["refund_freeze_trade_no"] != "refund-trade-freeze-1" || grant.Metadata["refund_freeze_points"] != "12.50000" {
		t.Fatalf("expected refund freeze metadata, got %#v", grant.Metadata)
	}
	providerPending, err := store.RecordProviderRefundStatus(ctx, billingservice.ProviderRefundStatusRequest{
		UserID:              92,
		OrderID:             order.ID,
		RefundTradeNo:       "refund-trade-freeze-1",
		RefundAmountCNY:     "12.50000",
		ChannelRefundNo:     "re_pending_1",
		ChannelRefundStatus: "pending",
		Reason:              "customer requested refund",
		OperatorAdminID:     7002,
	})
	if err != nil {
		t.Fatalf("RecordProviderRefundStatus: %v", err)
	}
	if providerPending.Status != "completed" || providerPending.RefundTradeNo != "refund-trade-freeze-1" || providerPending.ChannelRefundNo != "re_pending_1" || providerPending.ChannelRefundStatus != "pending" {
		t.Fatalf("expected provider refund identity without local settlement, got %#v", providerPending)
	}

	if _, err := store.ReleaseRefundPaymentOrder(ctx, freezeReq); err != nil {
		t.Fatalf("ReleaseRefundPaymentOrder: %v", err)
	}
	releasedBalance, err := store.GetBalance(ctx, 92)
	if err != nil {
		t.Fatalf("GetBalance after release: %v", err)
	}
	if releasedBalance.AvailablePoints != "12.50000" || releasedBalance.FrozenPoints != "0.00000" || releasedBalance.RechargePoints != "12.50000" {
		t.Fatalf("expected release to restore recharge grant, got %#v", releasedBalance)
	}

	if _, err := store.FreezeRefundPaymentOrder(ctx, freezeReq); err != nil {
		t.Fatalf("FreezeRefundPaymentOrder second: %v", err)
	}
	refunded, err := store.RefundPaymentOrder(ctx, freezeReq)
	if err != nil {
		t.Fatalf("RefundPaymentOrder from frozen grant: %v", err)
	}
	if refunded.Status != "refunded" || refunded.RefundTradeNo != "refund-trade-freeze-1" {
		t.Fatalf("expected refunded order, got %#v", refunded)
	}
	finalBalance, err := store.GetBalance(ctx, 92)
	if err != nil {
		t.Fatalf("GetBalance after refund: %v", err)
	}
	if finalBalance.AvailablePoints != "0.00000" || finalBalance.FrozenPoints != "0.00000" || finalBalance.RechargePoints != "0.00000" {
		t.Fatalf("expected final refund to clear frozen recharge grant, got %#v", finalBalance)
	}
}

func TestBillingStoreRefundFinalizeFailureEventCanRetryLocalRefund(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-refund-finalize-compensation?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateCustomAmountOrder(ctx, domainbilling.CreateCustomAmountOrderRequest{
		UserID:         93,
		AmountCNY:      "12.50000",
		CNYPerPoint:    "1.00000",
		Provider:       "easypay_alipay",
		PurchaseType:   "custom_amount",
		VisibleMethod:  "alipay",
		ProviderType:   "easypay_alipay",
		PaymentDisplay: map[string]any{"type": "popup"},
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}
	if _, err := store.CompleteRechargeOrder(ctx, domainbilling.CompleteRechargeOrderRequest{
		UserID:   93,
		OrderID:  order.ID,
		Provider: "easypay_alipay",
		TradeNo:  "easypay-trade-compensation-1",
	}); err != nil {
		t.Fatalf("CompleteRechargeOrder: %v", err)
	}

	refundReq := domainbilling.RefundPaymentOrderRequest{
		UserID:          93,
		OrderID:         order.ID,
		RefundTradeNo:   "refund-trade-compensation-1",
		Reason:          "customer requested refund",
		OperatorAdminID: 7003,
	}
	if _, err := store.FreezeRefundPaymentOrder(ctx, refundReq); err != nil {
		t.Fatalf("FreezeRefundPaymentOrder: %v", err)
	}
	event, err := store.RecordRefundFinalizeFailure(ctx, billingservice.RefundFinalizeFailureRequest{
		RefundPaymentOrderRequest: refundReq,
		FailureReason:             "payment order recharge balance is insufficient for refund",
	})
	if err != nil {
		t.Fatalf("RecordRefundFinalizeFailure: %v", err)
	}
	if event.Status != "failed" || event.EventType != "refund.local_finalize_failed" || event.OrderID != order.ID || event.FailureReason != "payment order recharge balance is insufficient for refund" {
		t.Fatalf("unexpected refund compensation event %#v", event)
	}

	retried, err := store.RetryWebhookEvent(ctx, event.ID)
	if err != nil {
		t.Fatalf("RetryWebhookEvent: %v", err)
	}
	if retried.Status != "processed" || retried.ProcessedAt == nil {
		t.Fatalf("expected processed compensation event, got %#v", retried)
	}
	refunded, err := store.GetOrderForAdmin(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrderForAdmin after retry: %v", err)
	}
	if refunded.Status != "refunded" || refunded.RefundTradeNo != "refund-trade-compensation-1" {
		t.Fatalf("expected retry to finalize local refund, got %#v", refunded)
	}
	balance, err := store.GetBalance(ctx, 93)
	if err != nil {
		t.Fatalf("GetBalance after retry: %v", err)
	}
	if balance.AvailablePoints != "0.00000" || balance.FrozenPoints != "0.00000" || balance.RechargePoints != "0.00000" {
		t.Fatalf("expected retry to clear recharge balance, got %#v", balance)
	}
}

func TestBillingStoreProcessRefundFinalizeFailuresRetriesFailedEvents(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-refund-finalize-auto-compensation?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateCustomAmountOrder(ctx, domainbilling.CreateCustomAmountOrderRequest{
		UserID:         94,
		AmountCNY:      "12.50000",
		CNYPerPoint:    "1.00000",
		Provider:       "easypay_alipay",
		PurchaseType:   "custom_amount",
		VisibleMethod:  "alipay",
		ProviderType:   "easypay_alipay",
		PaymentDisplay: map[string]any{"type": "popup"},
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}
	if _, err := store.CompleteRechargeOrder(ctx, domainbilling.CompleteRechargeOrderRequest{
		UserID:   94,
		OrderID:  order.ID,
		Provider: "easypay_alipay",
		TradeNo:  "easypay-trade-auto-compensation-1",
	}); err != nil {
		t.Fatalf("CompleteRechargeOrder: %v", err)
	}

	refundReq := domainbilling.RefundPaymentOrderRequest{
		UserID:          94,
		OrderID:         order.ID,
		RefundTradeNo:   "refund-trade-auto-compensation-1",
		Reason:          "customer requested refund",
		OperatorAdminID: 7004,
	}
	if _, err := store.FreezeRefundPaymentOrder(ctx, refundReq); err != nil {
		t.Fatalf("FreezeRefundPaymentOrder: %v", err)
	}
	event, err := store.RecordRefundFinalizeFailure(ctx, billingservice.RefundFinalizeFailureRequest{
		RefundPaymentOrderRequest: refundReq,
		FailureReason:             "payment order recharge balance is insufficient for refund",
	})
	if err != nil {
		t.Fatalf("RecordRefundFinalizeFailure: %v", err)
	}
	if event.Status != "failed" || event.EventType != "refund.local_finalize_failed" {
		t.Fatalf("unexpected refund compensation event %#v", event)
	}

	processed, err := store.ProcessRefundFinalizeFailures(ctx, 10)
	if err != nil {
		t.Fatalf("ProcessRefundFinalizeFailures: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected one processed compensation event, got %d", processed)
	}
	reloadedEvent, err := client.PaymentWebhookEvent.Get(ctx, int(event.ID))
	if err != nil {
		t.Fatalf("reload compensation event: %v", err)
	}
	if reloadedEvent.Status != "processed" || reloadedEvent.ProcessedAt == nil {
		t.Fatalf("expected auto compensation event processed, got %#v", reloadedEvent)
	}
	refunded, err := store.GetOrderForAdmin(ctx, order.ID)
	if err != nil {
		t.Fatalf("GetOrderForAdmin after auto compensation: %v", err)
	}
	if refunded.Status != "refunded" || refunded.RefundTradeNo != "refund-trade-auto-compensation-1" {
		t.Fatalf("expected auto compensation to finalize local refund, got %#v", refunded)
	}
}

func TestBillingStoreMarkOrderPaidCompletesCashierRechargeOrderIdempotently(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-webhook-complete-recharge-order?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateOrder(ctx, domainbilling.CreateOrderRequest{
		UserID:         90,
		PlanCode:       "basic-monthly",
		Provider:       "mock",
		PurchaseType:   "plan",
		VisibleMethod:  "mock",
		ProviderType:   "mock",
		PaymentDisplay: map[string]any{"type": "mock"},
	})
	if err != nil {
		t.Fatalf("CreateOrder: %v", err)
	}

	first, err := store.MarkOrderPaid(ctx, domainbilling.MarkOrderPaidRequest{
		Provider: "mock",
		OrderNo:  order.OrderNo,
		TradeNo:  "mock-webhook-1",
	})
	if err != nil {
		t.Fatalf("MarkOrderPaid first: %v", err)
	}
	if first.Status != "completed" || first.CompletedAt == nil || first.LedgerID == 0 {
		t.Fatalf("expected webhook to complete cashier recharge order, got %#v", first)
	}

	second, err := store.MarkOrderPaid(ctx, domainbilling.MarkOrderPaidRequest{
		Provider: "mock",
		OrderNo:  order.OrderNo,
		TradeNo:  "mock-webhook-1",
	})
	if err != nil {
		t.Fatalf("MarkOrderPaid second: %v", err)
	}
	if second.Status != "completed" || second.LedgerID != first.LedgerID {
		t.Fatalf("expected idempotent completed order, got %#v", second)
	}

	balance, err := store.GetBalance(ctx, 90)
	if err != nil {
		t.Fatalf("GetBalance: %v", err)
	}
	if balance.RechargePoints != "100.00000" || balance.SubscriptionPoints != "0.00000" || balance.AvailablePoints != "100.00000" {
		t.Fatalf("expected webhook to credit recharge bucket once, got %#v", balance)
	}
	ledgerCount, err := client.PointLedger.Query().
		Where(pointledger.UserIDEQ(90), pointledger.LedgerTypeEQ("recharge")).
		Count(ctx)
	if err != nil {
		t.Fatalf("count recharge ledger: %v", err)
	}
	if ledgerCount != 1 {
		t.Fatalf("expected one recharge ledger, got %d", ledgerCount)
	}
}

func TestBillingStoreMarkOrderPaidRejectsDifferentProviderBinding(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:billingstore-webhook-provider-binding?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewBillingStore(client, 5)
	order, err := store.CreateCustomAmountOrder(ctx, domainbilling.CreateCustomAmountOrderRequest{
		UserID:             91,
		OrderNo:            "PGO-CALLBACK-BINDING-ENT",
		AmountCNY:          "10.00",
		CNYPerPoint:        "0.31250",
		Provider:           "stripe",
		PurchaseType:       "custom_amount",
		VisibleMethod:      "stripe",
		ProviderType:       "stripe",
		ProviderInstanceID: 51,
	})
	if err != nil {
		t.Fatalf("CreateCustomAmountOrder: %v", err)
	}

	if _, err := store.MarkOrderPaid(ctx, domainbilling.MarkOrderPaidRequest{
		Provider:           "stripe",
		ProviderInstanceID: 52,
		OrderNo:            order.OrderNo,
		TradeNo:            "pi_cross_instance",
		AmountCNY:          order.AmountCNY,
	}); err == nil {
		t.Fatal("expected callback from a different provider instance to be rejected")
	}
	reloaded, err := store.GetOrder(ctx, order.UserID, order.ID)
	if err != nil {
		t.Fatalf("GetOrder: %v", err)
	}
	if reloaded.Status != "pending" || reloaded.TradeNo != "" {
		t.Fatalf("cross-instance callback must not mutate order: %#v", reloaded)
	}
}
