package entstore

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	_ "github.com/mattn/go-sqlite3"

	domainapikey "github.com/fatballfish/pic-gallery/internal/domain/apikey"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestAPIKeyStoreRoundTripAndLastUsedAt(t *testing.T) {
	client, err := repoent.Open(dialect.SQLite, "file:apikey-store?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewAPIKeyStore(client)
	key, err := store.Create(context.Background(), domainapikey.APIKey{
		UserID:           101,
		AccessKey:        "ak-store",
		SecretHash:       domainapikey.HashSecret("sk-store"),
		SecretCiphertext: "v1:cipher-store",
		Name:             "store-test",
		Status:           "active",
		GroupCode:        "plus",
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if key.ID == 0 || key.SecretHash == "sk-store" || key.SecretCiphertext == "" {
		t.Fatalf("expected persisted key with hashed secret, got %#v", key)
	}

	byAccess, err := store.GetByAccessKey(context.Background(), "ak-store")
	if err != nil {
		t.Fatalf("GetByAccessKey: %v", err)
	}
	bySecret, err := store.GetBySecretHash(context.Background(), domainapikey.HashSecret("sk-store"))
	if err != nil {
		t.Fatalf("GetBySecretHash: %v", err)
	}
	if byAccess.ID != key.ID || bySecret.ID != key.ID {
		t.Fatalf("expected lookups to return created key, access=%#v secret=%#v", byAccess, bySecret)
	}

	now := time.Now().UTC().Truncate(time.Second)
	if err := store.UpdateLastUsedAt(context.Background(), key.ID, now); err != nil {
		t.Fatalf("UpdateLastUsedAt: %v", err)
	}
	reloaded, err := store.GetByAccessKey(context.Background(), "ak-store")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.LastUsedAt == nil || !reloaded.LastUsedAt.Equal(now) {
		t.Fatalf("expected last_used_at to round-trip, got %#v", reloaded.LastUsedAt)
	}
}

func TestAPIKeyStoreScopedLifecycleAndSoftDelete(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:apikey-store-lifecycle?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewAPIKeyStore(client)
	totalQuota := "100.00000"
	dailyQuota := "10.00000"
	rpmLimit := 60
	owned, err := store.Create(ctx, domainapikey.APIKey{
		UserID:           42,
		AccessKey:        "ak-owned-store",
		SecretHash:       domainapikey.HashSecret("sk-owned-store"),
		SecretCiphertext: "v1:cipher-owned",
		Name:             "owned",
		Status:           domainapikey.StatusActive,
		GroupCode:        "plus",
		TotalQuotaPoints: &totalQuota,
		DailyQuotaPoints: &dailyQuota,
		RPMLimit:         &rpmLimit,
	})
	if err != nil {
		t.Fatalf("Create owned: %v", err)
	}
	other, err := store.Create(ctx, domainapikey.APIKey{UserID: 99, AccessKey: "ak-other-store", SecretHash: domainapikey.HashSecret("sk-other-store"), SecretCiphertext: "v1:cipher-other", Name: "other", Status: domainapikey.StatusActive, GroupCode: "basic"})
	if err != nil {
		t.Fatalf("Create other: %v", err)
	}

	list, err := store.ListByUser(ctx, 42)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 1 || list[0].ID != owned.ID || list[0].TotalQuotaPoints == nil || *list[0].TotalQuotaPoints != totalQuota || list[0].RPMLimit == nil || *list[0].RPMLimit != rpmLimit {
		t.Fatalf("unexpected scoped list: %#v", list)
	}
	if _, err := store.GetByID(ctx, 42, other.ID); err == nil {
		t.Fatal("expected GetByID to reject another user's key")
	}

	owned.Name = "renamed"
	updatedQuota := "250.00000"
	owned.TotalQuotaPoints = &updatedQuota
	updated, err := store.UpdateForUser(ctx, 42, owned)
	if err != nil {
		t.Fatalf("UpdateForUser owner: %v", err)
	}
	if updated.Name != "renamed" || updated.TotalQuotaPoints == nil || *updated.TotalQuotaPoints != updatedQuota {
		t.Fatalf("unexpected UpdateForUser result: %#v", updated)
	}
	if _, err := store.UpdateStatusForUser(ctx, 99, owned.ID, domainapikey.StatusDisabled); err == nil {
		t.Fatal("expected UpdateStatusForUser to reject another user's key")
	}
	resetKey, err := store.UpdateSecretForUser(ctx, 42, owned.ID, domainapikey.HashSecret("sk-reset-store"), "v1:cipher-reset")
	if err != nil {
		t.Fatalf("UpdateSecretForUser: %v", err)
	}
	if resetKey.SecretCiphertext != "v1:cipher-reset" {
		t.Fatalf("expected reset ciphertext to round-trip, got %#v", resetKey)
	}
	if _, err := store.GetBySecretHash(ctx, domainapikey.HashSecret("sk-owned-store")); err == nil {
		t.Fatal("expected old secret hash lookup to fail after reset")
	}
	if _, err := store.GetBySecretHash(ctx, domainapikey.HashSecret("sk-reset-store")); err != nil {
		t.Fatalf("expected new secret hash lookup to work: %v", err)
	}

	if err := store.DeleteForUser(ctx, 99, owned.ID, time.Now().UTC()); err == nil {
		t.Fatal("expected DeleteForUser to reject another user's key")
	}
	if err := store.DeleteForUser(ctx, 42, owned.ID, time.Now().UTC()); err != nil {
		t.Fatalf("DeleteForUser owner: %v", err)
	}
	if _, err := store.GetByID(ctx, 42, owned.ID); err == nil {
		t.Fatal("expected soft-deleted key to be hidden from GetByID")
	}
	list, err = store.ListByUser(ctx, 42)
	if err != nil {
		t.Fatalf("ListByUser after delete: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected soft-deleted key to be hidden from list, got %#v", list)
	}
}

func TestAPIKeyStorePersistsRPMAndQuotaReservations(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:apikey-store-quota?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewAPIKeyStore(client)
	totalQuota := "10.00000"
	dailyQuota := "6.00000"
	rpmLimit := 2
	key, err := store.Create(ctx, domainapikey.APIKey{
		UserID:           42,
		AccessKey:        "ak-quota-store",
		SecretHash:       domainapikey.HashSecret("sk-quota-store"),
		SecretCiphertext: "v1:cipher-quota",
		Name:             "quota",
		Status:           domainapikey.StatusActive,
		GroupCode:        "plus",
		TotalQuotaPoints: &totalQuota,
		DailyQuotaPoints: &dailyQuota,
		RPMLimit:         &rpmLimit,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	now := time.Date(2026, 5, 21, 10, 0, 0, 0, time.UTC)
	if err := store.RecordRequest(ctx, key.ID, rpmLimit, now); err != nil {
		t.Fatalf("RecordRequest first: %v", err)
	}
	if err := store.RecordRequest(ctx, key.ID, rpmLimit, now.Add(10*time.Second)); err != nil {
		t.Fatalf("RecordRequest second: %v", err)
	}
	if err := store.RecordRequest(ctx, key.ID, rpmLimit, now.Add(20*time.Second)); !isAppErrorCode(err, errs.CodeRateLimited) {
		t.Fatalf("expected RPM limit error, got %v", err)
	}
	reloaded, err := store.GetByID(ctx, 42, key.ID)
	if err != nil {
		t.Fatalf("GetByID after rpm: %v", err)
	}
	if reloaded.RPMWindowStartedAt == nil || !reloaded.RPMWindowStartedAt.Equal(now) || reloaded.RPMWindowCount != 2 {
		t.Fatalf("expected rpm window to persist, got %#v", reloaded)
	}

	secondStore := NewAPIKeyStore(client)
	if err := secondStore.ReserveQuota(ctx, 42, key.ID, "task-1", "4.00000", now); err != nil {
		t.Fatalf("ReserveQuota task-1: %v", err)
	}
	if err := secondStore.ReserveQuota(ctx, 42, key.ID, "task-1", "4.00000", now); err != nil {
		t.Fatalf("ReserveQuota must be idempotent for same reservation: %v", err)
	}
	if err := secondStore.ReserveQuota(ctx, 42, key.ID, "task-1", "5.00000", now); !isAppErrorCode(err, errs.CodeConflict) {
		t.Fatalf("expected points conflict for same active reservation, got %v", err)
	}
	reloaded, err = store.GetByID(ctx, 42, key.ID)
	if err != nil {
		t.Fatalf("GetByID after reserve: %v", err)
	}
	if reloaded.TotalQuotaUsedPoints != "4.00000" || reloaded.DailyQuotaUsedPoints != "4.00000" || reloaded.QuotaUsageDay == nil || *reloaded.QuotaUsageDay != "2026-05-21" {
		t.Fatalf("expected quota usage to persist once, got %#v", reloaded)
	}
	if err := secondStore.ReserveQuota(ctx, 42, key.ID, "task-2", "3.00000", now); !isAppErrorCode(err, errs.CodeInsufficientPoints) {
		t.Fatalf("expected daily quota error, got %v", err)
	}
	if err := store.ReleaseQuota(ctx, key.ID, "task-1"); err != nil {
		t.Fatalf("ReleaseQuota task-1: %v", err)
	}
	if err := secondStore.ReleaseQuota(ctx, key.ID, "task-1"); err != nil {
		t.Fatalf("ReleaseQuota must be idempotent: %v", err)
	}
	reloaded, err = secondStore.GetByID(ctx, 42, key.ID)
	if err != nil {
		t.Fatalf("GetByID after release: %v", err)
	}
	if reloaded.TotalQuotaUsedPoints != "0.00000" || reloaded.DailyQuotaUsedPoints != "0.00000" {
		t.Fatalf("expected quota release to subtract once, got %#v", reloaded)
	}
	if err := secondStore.ReserveQuota(ctx, 42, key.ID, "task-1", "4.00000", now); err != nil {
		t.Fatalf("ReserveQuota after release with same reservation should re-reserve: %v", err)
	}
	reloaded, err = secondStore.GetByID(ctx, 42, key.ID)
	if err != nil {
		t.Fatalf("GetByID after re-reserve: %v", err)
	}
	if reloaded.TotalQuotaUsedPoints != "4.00000" || reloaded.DailyQuotaUsedPoints != "4.00000" {
		t.Fatalf("expected released reservation retry to charge quota again, got %#v", reloaded)
	}
	if err := store.ReleaseQuota(ctx, key.ID, "task-1"); err != nil {
		t.Fatalf("ReleaseQuota re-reserved task-1: %v", err)
	}
	if err := secondStore.ReserveQuota(ctx, 42, key.ID, "task-2", "3.00000", now); err != nil {
		t.Fatalf("ReserveQuota after release: %v", err)
	}
}

func TestAPIKeyStoreRecordRequestAllowsConcurrentRequestsBelowLimit(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:apikey-store-rpm-concurrent?mode=memory&cache=shared&_fk=1&_busy_timeout=5000")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	store := NewAPIKeyStore(client)
	rpmLimit := 80
	key, err := store.Create(ctx, domainapikey.APIKey{UserID: 42, AccessKey: "ak-rpm-concurrent", SecretHash: domainapikey.HashSecret("sk-rpm-concurrent"), SecretCiphertext: "v1:cipher-rpm", Name: "rpm", Status: domainapikey.StatusActive, GroupCode: "plus", RPMLimit: &rpmLimit})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	var wg sync.WaitGroup
	errsCh := make(chan error, 24)
	now := time.Date(2026, 5, 21, 11, 0, 0, 0, time.UTC)
	for i := 0; i < cap(errsCh); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errsCh <- store.RecordRequest(ctx, key.ID, rpmLimit, now)
		}()
	}
	wg.Wait()
	close(errsCh)
	for err := range errsCh {
		if err != nil {
			t.Fatalf("RecordRequest below limit should not fail under concurrency: %v", err)
		}
	}
	reloaded, err := store.GetByID(ctx, 42, key.ID)
	if err != nil {
		t.Fatalf("GetByID after concurrent rpm: %v", err)
	}
	if reloaded.RPMWindowCount != 24 {
		t.Fatalf("expected all concurrent requests to be counted, got %#v", reloaded)
	}
}

func isAppErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var appErr *errs.Error
	return errors.As(err, &appErr) && appErr.Code == code
}
