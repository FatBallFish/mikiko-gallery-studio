package apikey

import (
	"context"
	"errors"
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

	identity, err := svc.AuthenticateNative(context.Background(), created.Key.AccessKey, "sk-task4-secret")
	if err != nil {
		t.Fatalf("AuthenticateNative: %v", err)
	}
	if identity.UserID != 42 || identity.APIKeyID == 0 || identity.GroupCode != "plus" {
		t.Fatalf("unexpected native identity %#v", identity)
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

	if _, err := svc.AuthenticateNative(context.Background(), "ak-missing", "sk-reject"); err == nil {
		t.Fatal("expected missing access key to be rejected")
	}
	if _, err := svc.AuthenticateNative(context.Background(), created.Key.AccessKey, "wrong-secret"); err == nil {
		t.Fatal("expected invalid signature to be rejected")
	}

	if err := svc.SetStatusForTest(context.Background(), created.Key.ID, "disabled"); err != nil {
		t.Fatalf("SetStatusForTest disabled: %v", err)
	}
	if _, err := svc.AuthenticateNative(context.Background(), created.Key.AccessKey, "sk-reject"); err == nil {
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
	if _, err := svc.AuthenticateNative(ctx, created.Key.AccessKey, "sk-old"); err == nil {
		t.Fatal("expected old secret to stop authenticating after reset")
	}
	if _, err := svc.AuthenticateNative(ctx, created.Key.AccessKey, reset.Secret); err != nil {
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
	if _, err := svc.AuthenticateNative(ctx, created.Key.AccessKey, reset.Secret); err == nil {
		t.Fatal("expected revoked/deleted key to stop authenticating")
	}
}

func TestVerifyCanonicalHMACChecksTimestampBodyHashAndSignature(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	created, err := svc.CreateKey(ctx, CreateRequest{UserID: 42, Secret: "sk-hmac"})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}

	body := []byte(`{"prompt":"cat"}`)
	now := time.Now().UTC()
	bodyHash := BodySHA256(body)
	signature := SignCanonicalHMAC("sk-hmac", "POST", "/v1/images?size=1", now, bodyHash)
	identity, err := svc.VerifyCanonicalHMAC(ctx, HMACRequest{
		AccessKey:  created.Key.AccessKey,
		Method:     "post",
		Path:       "/v1/images?size=1",
		Timestamp:  now,
		Body:       body,
		BodySHA256: bodyHash,
		Signature:  signature,
	})
	if err != nil {
		t.Fatalf("VerifyCanonicalHMAC: %v", err)
	}
	if identity.UserID != 42 || identity.APIKeyID != created.Key.ID {
		t.Fatalf("unexpected identity %#v", identity)
	}

	if _, err := svc.VerifyCanonicalHMAC(ctx, HMACRequest{AccessKey: created.Key.AccessKey, Method: "POST", Path: "/v1/images?size=1", Timestamp: now.Add(-10 * time.Minute), Body: body, BodySHA256: bodyHash, Signature: signature}); err == nil {
		t.Fatal("expected stale timestamp to be rejected")
	}
	if _, err := svc.VerifyCanonicalHMAC(ctx, HMACRequest{AccessKey: created.Key.AccessKey, Method: "POST", Path: "/v1/images?size=1", Timestamp: now, Body: []byte(`{}`), BodySHA256: bodyHash, Signature: signature}); err == nil {
		t.Fatal("expected mismatched body hash to be rejected")
	}
	hashAsSecretSignature := signCanonicalHMACWithKey(created.Key.SecretHash, "POST", "/v1/images?size=1", now, bodyHash)
	if _, err := svc.VerifyCanonicalHMAC(ctx, HMACRequest{AccessKey: created.Key.AccessKey, Method: "POST", Path: "/v1/images?size=1", Timestamp: now, Body: body, BodySHA256: bodyHash, Signature: hashAsSecretSignature}); err == nil {
		t.Fatal("expected stored secret hash to be rejected as an HMAC signing credential")
	}
}

func TestCanonicalHMACCrossLanguageVector(t *testing.T) {
	const (
		secret            = "contract-signing-secret"
		method            = "POST"
		requestURI        = "/api/open/image/v1/tasks"
		timestampRFC3339  = "2026-07-17T00:00:00Z"
		body              = `{"task_type":"image_edit","route_model_code":"plus-image","requested_quality":"2k","requested_size":"2560x1440","requested_output_image_count":2,"reference_image_count":1,"prompt":"Paint a quiet harbor","reference_asset_ids":["ref-open-1"],"response_mode":"async"}`
		expectedBodyHash  = "F-y2oZ4DbiAMUoV5nPDNiDF-sPoMo4wRRW-4e6LPvK0"
		expectedSignature = "KFATqBMyGGl_aUHTGx052h5_mK04hszt0csDxqen2sA"
	)
	timestamp, err := time.Parse(time.RFC3339, timestampRFC3339)
	if err != nil {
		t.Fatalf("parse timestamp: %v", err)
	}
	if got := BodySHA256([]byte(body)); got != expectedBodyHash {
		t.Fatalf("cross-language body hash mismatch: got %q", got)
	}
	if got := SignCanonicalHMAC(secret, method, requestURI, timestamp, expectedBodyHash); got != expectedSignature {
		t.Fatalf("cross-language signature mismatch: got %q", got)
	}
}

func TestAPIKeyRPMAndQuotaAreEnforced(t *testing.T) {
	ctx := context.Background()
	svc := NewService(nil)
	totalQuota := "10.00000"
	dailyQuota := "6.00000"
	rpmLimit := 2
	created, err := svc.CreateKey(ctx, CreateRequest{
		UserID:           42,
		Secret:           "sk-limits",
		TotalQuotaPoints: &totalQuota,
		DailyQuotaPoints: &dailyQuota,
		RPMLimit:         &rpmLimit,
	})
	if err != nil {
		t.Fatalf("CreateKey: %v", err)
	}
	first, err := svc.AuthenticateNative(ctx, created.Key.AccessKey, "sk-limits")
	if err != nil {
		t.Fatalf("first auth: %v", err)
	}
	if _, err := svc.AuthenticateNative(ctx, created.Key.AccessKey, "sk-limits"); err != nil {
		t.Fatalf("second auth within RPM: %v", err)
	}
	if _, err := svc.AuthenticateNative(ctx, created.Key.AccessKey, "sk-limits"); err == nil {
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
	identity, err := svc1.AuthenticateNative(ctx, created.Key.AccessKey, "sk-persist")
	if err != nil {
		t.Fatalf("first auth: %v", err)
	}
	if _, err := svc1.AuthenticateNative(ctx, created.Key.AccessKey, "sk-persist"); err != nil {
		t.Fatalf("second auth: %v", err)
	}
	if _, err := svc2.AuthenticateNative(ctx, created.Key.AccessKey, "sk-persist"); !isServiceAppErrorCode(err, errs.CodeRateLimited) {
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
