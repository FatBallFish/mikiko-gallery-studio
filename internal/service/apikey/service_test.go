package apikey

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/entstore"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	_ "github.com/mattn/go-sqlite3"
)

func TestCreateKeyStoresOnlySecretHashAndAuthenticates(t *testing.T) {
	svc := NewService(nil)

	created, err := svc.CreateKey(context.Background(), CreateRequest{
		UserID:    42,
		Name:      "task4",
		GroupCode: "plus",
		Secret:    "sk-task4-secret",
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	if created.Secret != "sk-task4-secret" {
		t.Fatalf("expected plaintext secret to be returned once on create")
	}
	if created.Key.SecretHash == "" || created.Key.SecretHash == "sk-task4-secret" {
		t.Fatalf("expected stored key to contain only a hash, got %#v", created.Key)
	}

	identity, err := svc.AuthenticateBearer(context.Background(), "sk-task4-secret")
	if err != nil {
		t.Fatalf("AuthenticateBearer: %v", err)
	}
	if identity.UserID != 42 || identity.APIKeyID == 0 || identity.GroupCode != "plus" {
		t.Fatalf("unexpected bearer identity %#v", identity)
	}

	bearer, err := svc.AuthenticateBearer(context.Background(), "sk-task4-secret")
	if err != nil {
		t.Fatalf("AuthenticateBearer: %v", err)
	}
	if bearer.APIKeyID != identity.APIKeyID {
		t.Fatalf("expected bearer identity to match native identity, got %#v want %#v", bearer, identity)
	}
}

func TestAuthenticateRejectsMissingInvalidDisabledAndExpiredKeys(t *testing.T) {
	svc := NewService(nil)
	created, err := svc.CreateKey(context.Background(), CreateRequest{UserID: 7, Name: "reject", GroupCode: "basic", Secret: "sk-reject"})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	if _, err := svc.AuthenticateBearer(context.Background(), "sk-missing"); err == nil {
		t.Fatal("expected unknown bearer token to be rejected")
	}
	if _, err := svc.AuthenticateBearer(context.Background(), "wrong-secret"); err == nil {
		t.Fatal("expected invalid bearer token to be rejected")
	}

	if err := svc.SetStatusForTest(context.Background(), created.Key.ID, "disabled"); err != nil {
		t.Fatalf("SetStatusForTest disabled: %v", err)
	}
	if _, err := svc.AuthenticateBearer(context.Background(), "sk-reject"); err == nil {
		t.Fatal("expected disabled key to be rejected")
	}

	if err := svc.SetStatusForTest(context.Background(), created.Key.ID, "active"); err != nil {
		t.Fatalf("SetStatusForTest active: %v", err)
	}
	if err := svc.SetExpiresAtForTest(context.Background(), created.Key.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatalf("SetExpiresAtForTest: %v", err)
	}
	if _, err := svc.AuthenticateBearer(context.Background(), "sk-reject"); err == nil {
		t.Fatal("expected expired bearer key to be rejected")
	}
}

func TestLifecycleMethodsAreScopedToUserAndSupportQuotaFields(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	totalQuota := "100.00000"
	dailyQuota := "10.00000"
	rpmLimit := 60

	owned, err := svc.CreateKey(ctx, CreateRequest{
		UserID:           42,
		Name:             "owned",
		GroupCode:        "plus",
		Secret:           "sk-owned",
		TotalQuotaPoints: &totalQuota,
		DailyQuotaPoints: &dailyQuota,
		RPMLimit:         &rpmLimit,
	})
	if err != nil {
		t.Fatalf("CreateKey owned: %v", err)
	}
	other, err := svc.CreateKey(ctx, CreateRequest{UserID: 99, Name: "other", Secret: "sk-other"})
	if err != nil {
		t.Fatalf("CreateKey other: %v", err)
	}

	list, err := svc.ListByUser(ctx, 42)
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(list) != 1 || list[0].ID != owned.Key.ID || list[0].TotalQuotaPoints == nil || *list[0].TotalQuotaPoints != totalQuota || list[0].RPMLimit == nil || *list[0].RPMLimit != rpmLimit {
		t.Fatalf("unexpected user-scoped list: %#v", list)
	}
	if _, err := svc.GetByID(ctx, 42, other.Key.ID); err == nil {
		t.Fatal("expected GetByID to reject keys owned by another user")
	}

	updatedTotal := "250.00000"
	updatedRPM := 120
	metadata, err := svc.Update(ctx, 42, owned.Key.ID, UpdateRequest{
		Name:             stringPtr("renamed"),
		TotalQuotaPoints: &updatedTotal,
		RPMLimit:         &updatedRPM,
	})
	if err != nil {
		t.Fatalf("Update owner metadata: %v", err)
	}
	if metadata.Name != "renamed" || metadata.TotalQuotaPoints == nil || *metadata.TotalQuotaPoints != updatedTotal || metadata.RPMLimit == nil || *metadata.RPMLimit != updatedRPM {
		t.Fatalf("unexpected metadata update: %#v", metadata)
	}
	if _, err := svc.Update(ctx, 99, owned.Key.ID, UpdateRequest{Name: stringPtr("stolen")}); err == nil {
		t.Fatal("expected Update to reject keys owned by another user")
	}

	if err := svc.UpdateStatus(ctx, 99, owned.Key.ID, "disabled"); err == nil {
		t.Fatal("expected UpdateStatus to reject keys owned by another user")
	}
	if err := svc.UpdateStatus(ctx, 42, owned.Key.ID, "disabled"); err != nil {
		t.Fatalf("UpdateStatus owner: %v", err)
	}
	updated, err := svc.GetByID(ctx, 42, owned.Key.ID)
	if err != nil {
		t.Fatalf("GetByID owner: %v", err)
	}
	if updated.Status != "disabled" {
		t.Fatalf("expected disabled status, got %#v", updated)
	}
}

func TestCreateAndUpdateRejectInvalidRPMLimit(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)

	zero := 0
	if _, err := svc.CreateKey(ctx, CreateRequest{UserID: 42, Name: "zero-rpm", RPMLimit: &zero}); !isServiceAppErrorCode(err, errs.CodeBadRequest) {
		t.Fatalf("expected create with zero rpm_limit to fail with bad_request, got %v", err)
	}

	negative := -1
	if _, err := svc.CreateKey(ctx, CreateRequest{UserID: 42, Name: "negative-rpm", RPMLimit: &negative}); !isServiceAppErrorCode(err, errs.CodeBadRequest) {
		t.Fatalf("expected create with negative rpm_limit to fail with bad_request, got %v", err)
	}

	created, err := svc.CreateKey(ctx, CreateRequest{UserID: 42, Name: "valid-rpm"})
	if err != nil {
		t.Fatalf("CreateKey valid: %v", err)
	}
	if _, err := svc.Update(ctx, 42, created.Key.ID, UpdateRequest{RPMLimit: &zero}); !isServiceAppErrorCode(err, errs.CodeBadRequest) {
		t.Fatalf("expected update with zero rpm_limit to fail with bad_request, got %v", err)
	}

	one := 1
	updated, err := svc.Update(ctx, 42, created.Key.ID, UpdateRequest{RPMLimit: &one})
	if err != nil {
		t.Fatalf("Update rpm_limit one: %v", err)
	}
	if updated.RPMLimit == nil || *updated.RPMLimit != one {
		t.Fatalf("expected rpm_limit=1 to persist, got %#v", updated.RPMLimit)
	}
}

func TestDeleteRevokesAndResetSecretRotatesCredentials(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	created, err := svc.CreateKey(ctx, CreateRequest{UserID: 42, Name: "rotate", Secret: "sk-old"})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	reset, err := svc.ResetSecret(ctx, 42, created.Key.ID)
	if err != nil {
		t.Fatalf("ResetSecret: %v", err)
	}
	if reset.Secret == "" || reset.Secret == "sk-old" || reset.Key.SecretHash == created.Key.SecretHash {
		t.Fatalf("expected rotated secret and hash, got %#v", reset)
	}
	if _, err := svc.AuthenticateBearer(ctx, "sk-old"); err == nil {
		t.Fatal("expected old secret to stop authenticating after reset")
	}
	if _, err := svc.AuthenticateBearer(ctx, reset.Secret); err != nil {
		t.Fatalf("expected reset secret to authenticate: %v", err)
	}

	if err := svc.Delete(ctx, 7, created.Key.ID); err == nil {
		t.Fatal("expected Delete to reject another user")
	}
	if err := svc.Delete(ctx, 42, created.Key.ID); err != nil {
		t.Fatalf("Delete owner: %v", err)
	}
	if _, err := svc.GetByID(ctx, 42, created.Key.ID); err == nil {
		t.Fatal("expected deleted key to be hidden from scoped lookup")
	}
	if _, err := svc.AuthenticateBearer(ctx, reset.Secret); err == nil {
		t.Fatal("expected revoked/deleted key to stop authenticating")
	}
}

func TestScopesDefaultsExplicitEmptyValidationAndRotation(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	legacy, err := svc.CreateKey(ctx, CreateRequest{UserID: 42})
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.Key.Scopes) != 4 {
		t.Fatalf("legacy scopes: %v", legacy.Key.Scopes)
	}
	for _, scope := range legacy.Key.Scopes {
		if strings.HasSuffix(scope, ":delete") || strings.HasPrefix(scope, "videos:") {
			t.Fatal("legacy key gained permissions")
		}
	}
	if legacy.Key.SecretCiphertext != "" || legacy.Key.SigningSecret != "" {
		t.Fatal("key must persist only hash")
	}
	empty := []string{}
	updated, err := svc.Update(ctx, 42, legacy.Key.ID, UpdateRequest{Scopes: &empty})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Scopes == nil || len(updated.Scopes) != 0 {
		t.Fatalf("explicit empty scopes lost: %#v", updated.Scopes)
	}
	scopes := []string{"assets:read", "assets:read", "assets:delete"}
	if _, err = svc.Update(ctx, 42, legacy.Key.ID, UpdateRequest{Scopes: &scopes}); err != nil {
		t.Fatal(err)
	}
	reset, err := svc.ResetSecret(ctx, 42, legacy.Key.ID)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := svc.AuthenticateBearer(ctx, reset.Secret)
	if err != nil {
		t.Fatal(err)
	}
	if len(identity.Scopes) != 2 {
		t.Fatalf("rotation lost scopes: %v", identity.Scopes)
	}
	if _, err = svc.AuthenticateBearer(ctx, legacy.Secret); err == nil {
		t.Fatal("old key authenticated")
	}
	if _, err = svc.CreateKey(ctx, CreateRequest{UserID: 42, Scopes: []string{"*"}}); err == nil {
		t.Fatal("wildcard scope accepted")
	}
}

func TestAPIKeyRPMAndQuotaAreEnforced(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	totalQuota := "10.00000"
	dailyQuota := "6.00000"
	rpmLimit := 2
	_, err := svc.CreateKey(ctx, CreateRequest{
		UserID:           42,
		Secret:           "sk-limits",
		TotalQuotaPoints: &totalQuota,
		DailyQuotaPoints: &dailyQuota,
		RPMLimit:         &rpmLimit,
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	first, err := svc.AuthenticateBearer(ctx, "sk-limits")
	if err != nil {
		t.Fatalf("first auth: %v", err)
	}
	if _, err := svc.AuthenticateBearer(ctx, "sk-limits"); err != nil {
		t.Fatalf("second auth within RPM: %v", err)
	}
	if _, err := svc.AuthenticateBearer(ctx, "sk-limits"); err == nil {
		t.Fatalf("expected third auth to exceed RPM")
	}

	if err := svc.ReserveQuota(ctx, first, "task-1", "4.00000"); err != nil {
		t.Fatalf("ReserveQuota task-1: %v", err)
	}
	if err := svc.ReserveQuota(ctx, first, "task-1", "4.00000"); err != nil {
		t.Fatalf("ReserveQuota must be idempotent for same task: %v", err)
	}
	if err := svc.ReserveQuota(ctx, first, "task-1", "5.00000"); !isServiceAppErrorCode(err, errs.CodeConflict) {
		t.Fatalf("expected different points for active reservation to conflict, got %v", err)
	}
	if err := svc.ReserveQuota(ctx, first, "task-2", "3.00000"); err == nil {
		t.Fatalf("expected daily quota to reject over-limit reservation")
	}
	if err := svc.ReleaseQuota(ctx, first, "task-1"); err != nil {
		t.Fatalf("ReleaseQuota task-1: %v", err)
	}
	if err := svc.ReleaseQuota(ctx, first, "task-1"); err != nil {
		t.Fatalf("ReleaseQuota task-1 must be idempotent: %v", err)
	}
	if err := svc.ReserveQuota(ctx, first, "task-1", "5.00000"); !isServiceAppErrorCode(err, errs.CodeConflict) {
		t.Fatalf("expected different points for released reservation to conflict, got %v", err)
	}
	if err := svc.ReserveQuota(ctx, first, "task-1", "4.00000"); err != nil {
		t.Fatalf("ReserveQuota same reservation after release should re-reserve: %v", err)
	}
	if err := svc.ReleaseQuota(ctx, first, "task-1"); err != nil {
		t.Fatalf("ReleaseQuota re-reserved task-1: %v", err)
	}
	if err := svc.ReserveQuota(ctx, first, "task-2", "3.00000"); err != nil {
		t.Fatalf("ReserveQuota after release: %v", err)
	}
}

func TestAPIKeyRPMAndQuotaPersistAcrossServiceInstances(t *testing.T) {
	ctx := context.Background()
	client, err := ent.Open(dialect.SQLite, "file:apikey-service-persist?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	store := entstore.NewAPIKeyStore(client)
	svc1 := NewService(store)
	svc2 := NewService(store)
	totalQuota := "10.00000"
	dailyQuota := "5.00000"
	rpmLimit := 2
	created, err := svc1.CreateKey(ctx, CreateRequest{
		UserID:           42,
		Secret:           "sk-persist",
		TotalQuotaPoints: &totalQuota,
		DailyQuotaPoints: &dailyQuota,
		RPMLimit:         &rpmLimit,
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	identity, err := svc1.AuthenticateBearer(ctx, "sk-persist")
	if err != nil {
		t.Fatalf("first auth: %v", err)
	}
	if _, err := svc1.AuthenticateBearer(ctx, "sk-persist"); err != nil {
		t.Fatalf("second auth: %v", err)
	}
	if _, err := svc2.AuthenticateBearer(ctx, "sk-persist"); !isServiceAppErrorCode(err, errs.CodeRateLimited) {
		t.Fatalf("expected persisted RPM limit across service instances, got %v", err)
	}

	if err := svc1.ReserveQuota(ctx, identity, "task-1", "4.00000"); err != nil {
		t.Fatalf("ReserveQuota task-1: %v", err)
	}
	if err := svc2.ReserveQuota(ctx, identity, "task-2", "2.00000"); !isServiceAppErrorCode(err, errs.CodeInsufficientPoints) {
		t.Fatalf("expected persisted daily quota across service instances, got %v", err)
	}
	reloaded, err := svc2.GetByID(ctx, 42, created.Key.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if reloaded.DailyQuotaUsedPoints != "4.00000" || reloaded.TotalQuotaUsedPoints != "4.00000" {
		t.Fatalf("expected persisted quota usage, got %#v", reloaded)
	}
}

func isServiceAppErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}
	var appErr *errs.Error
	return errors.As(err, &appErr) && appErr.Code == code
}

func stringPtr(value string) *string { return &value }

func TestCheckQuotaDoesNotReserveAndIncludesExistingReservations(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	total := "10.00000"
	daily := "4.00000"
	created, err := svc.CreateKey(ctx, CreateRequest{UserID: 42, TotalQuotaPoints: &total, DailyQuotaPoints: &daily})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := svc.AuthenticateBearer(ctx, created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := svc.CheckQuota(ctx, identity, "4.00000"); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.ReserveQuota(ctx, identity, "task", "3.00000"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckQuota(ctx, identity, "2.00000"); err == nil {
		t.Fatal("daily reservation ignored")
	}
	if err := svc.CheckQuota(ctx, identity, "11.00000"); err == nil {
		t.Fatal("total limit ignored")
	}
	if err := svc.CheckQuota(ctx, identity, "1.00000"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckQuota(ctx, identity, "-1"); err == nil {
		t.Fatal("negative points allowed")
	}
	if err := svc.SetStatusForTest(ctx, created.Key.ID, "disabled"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckQuota(ctx, identity, "0"); err == nil {
		t.Fatal("disabled key accepted")
	}
}

type keyUsageReaderStub struct {
	total, daily string
	err          error
}

func (s keyUsageReaderStub) APIKeyUsage(ctx context.Context, keyID int64, since *time.Time) (string, error) {
	if since != nil {
		return s.daily, s.err
	}
	return s.total, s.err
}
func TestCheckQuotaIncludesCanonicalBillingUsage(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	total := "10.00000"
	daily := "4.00000"
	created, err := svc.CreateKey(ctx, CreateRequest{UserID: 42, TotalQuotaPoints: &total, DailyQuotaPoints: &daily})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := svc.AuthenticateBearer(ctx, created.Secret)
	if err != nil {
		t.Fatal(err)
	}
	svc.SetUsageReader(keyUsageReaderStub{total: "9.00000", daily: "3.00000"})
	if err := svc.CheckQuota(ctx, identity, "1.00000"); err != nil {
		t.Fatal(err)
	}
	if err := svc.CheckQuota(ctx, identity, "2.00000"); err == nil {
		t.Fatal("canonical usage ignored")
	}
	svc.SetUsageReader(keyUsageReaderStub{err: errors.New("offline")})
	if err := svc.CheckQuota(ctx, identity, "0"); err == nil {
		t.Fatal("usage reader failure bypassed quota")
	}
}
