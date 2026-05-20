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
