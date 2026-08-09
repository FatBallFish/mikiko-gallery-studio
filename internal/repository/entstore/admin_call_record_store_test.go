package entstore

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
	"github.com/fatballfish/pic-gallery/internal/repository/ent/imagetask"
	imagetaskservice "github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	_ "github.com/mattn/go-sqlite3"
)

type adminCallRecordStaticRoutingSource struct {
	snapshot modelhub.ModelRoutingSnapshot
}

func TestAdminCallRecordStoreAggregatesAttemptsWithoutPagination(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:admin-call-distribution?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	inside := time.Now().UTC()
	from := inside.Add(-time.Hour)
	store := NewImageTaskStore(client)
	seeds := []domainimagetask.Task{
		{UserID: 1, ID: "11111111-1111-1111-1111-111111111111", Status: domainimagetask.StatusSucceeded, RouteModelCode: "route-a", TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k", Attempts: []domainimagetask.Attempt{{StartedAt: &inside}, {StartedAt: &inside}}},
		{UserID: 1, ID: "22222222-2222-2222-2222-222222222222", Status: domainimagetask.StatusFailed, TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k", Attempts: []domainimagetask.Attempt{{StartedAt: &inside}}},
		{UserID: 1, ID: "33333333-3333-3333-3333-333333333333", Status: domainimagetask.StatusRejected, TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k"},
	}
	for _, seed := range seeds {
		if err := store.Save(ctx, seed); err != nil {
			t.Fatalf("save task %s: %v", seed.ID, err)
		}
	}

	distribution, err := NewAdminCallRecordStore(client).CallDistribution(ctx, domainadmincallrecord.DistributionRequest{From: from, To: from.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("CallDistribution: %v", err)
	}
	if distribution.TotalCalls != 3 || distribution.PreflightFailureCount != 1 || len(distribution.Groups) != 2 || distribution.Groups[0].Key != "route-a" || distribution.Groups[0].Calls != 2 || distribution.Groups[1].Key != "unrouted" {
		t.Fatalf("distribution = %#v", distribution)
	}
}

func TestAdminCallRecordStoreDistributionSurvivesTaskSoftDeletion(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:admin-call-distribution-soft-delete?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	inside := time.Now().UTC()
	request := domainadmincallrecord.DistributionRequest{From: inside.Add(-24 * time.Hour), To: inside.Add(24 * time.Hour)}
	taskStore := NewImageTaskStore(client)
	seeds := []domainimagetask.Task{
		{
			UserID: 88, ID: "44444444-4444-4444-8444-444444444444", Status: domainimagetask.StatusSucceeded,
			RouteModelCode: "route-soft-deleted", TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k",
			Attempts: []domainimagetask.Attempt{{StartedAt: &inside}, {StartedAt: &inside}},
		},
		{
			UserID: 88, ID: "55555555-5555-4555-8555-555555555555", Status: domainimagetask.StatusRejected,
			TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k",
		},
	}
	for _, seed := range seeds {
		if err := taskStore.Save(ctx, seed); err != nil {
			t.Fatalf("save task %s: %v", seed.ID, err)
		}
	}

	callRecords := NewAdminCallRecordStore(client)
	before, err := callRecords.CallDistribution(ctx, request)
	if err != nil {
		t.Fatalf("distribution before deletion: %v", err)
	}
	if before.TotalCalls != 2 || before.PreflightFailureCount != 1 || len(before.Groups) != 1 || before.Groups[0].Key != "route-soft-deleted" || before.Groups[0].Calls != 2 || before.Groups[0].Percentage != 100 {
		t.Fatalf("distribution before deletion = %#v", before)
	}
	visibleBefore, err := callRecords.ListCallRecords(ctx, domainadmincallrecord.ListRequest{Page: 1, PageSize: 10})
	if err != nil || visibleBefore.Total != 2 {
		t.Fatalf("visible records before deletion: page=%#v err=%v", visibleBefore, err)
	}

	for _, seed := range seeds {
		if err := taskStore.DeleteByID(ctx, seed.UserID, seed.ID); err != nil {
			t.Fatalf("soft-delete task %s: %v", seed.ID, err)
		}
	}

	after, err := callRecords.CallDistribution(ctx, request)
	if err != nil {
		t.Fatalf("distribution after deletion: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("distribution changed after soft deletion: before=%#v after=%#v", before, after)
	}
	visibleAfter, err := callRecords.ListCallRecords(ctx, domainadmincallrecord.ListRequest{Page: 1, PageSize: 10})
	if err != nil || visibleAfter.Total != 0 || len(visibleAfter.Items) != 0 {
		t.Fatalf("deleted records remain visible in admin list: page=%#v err=%v", visibleAfter, err)
	}
}

func TestAdminCallRecordStoreDistributionUsesStableKeysetAcrossBatches(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:admin-call-distribution-keyset?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	inside := time.Now().UTC()
	request := domainadmincallrecord.DistributionRequest{From: inside.Add(-time.Hour), To: inside.Add(time.Hour)}
	seeds := []domainimagetask.Task{
		{UserID: 91, ID: "00000000-0000-4000-8000-000000000001", Status: domainimagetask.StatusSucceeded, RouteModelCode: "route-a", TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k", Attempts: []domainimagetask.Attempt{{StartedAt: &inside}, {StartedAt: &inside}}},
		{UserID: 91, ID: "00000000-0000-4000-8000-000000000002", Status: domainimagetask.StatusSucceeded, RouteModelCode: "route-b", TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k", Attempts: []domainimagetask.Attempt{{StartedAt: &inside}}},
		{UserID: 91, ID: "00000000-0000-4000-8000-000000000003", Status: domainimagetask.StatusFailed, TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k", Attempts: []domainimagetask.Attempt{{StartedAt: &inside}}},
		{UserID: 91, ID: "00000000-0000-4000-8000-000000000004", Status: domainimagetask.StatusRejected, TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k"},
		{UserID: 91, ID: "00000000-0000-4000-8000-000000000005", Status: domainimagetask.StatusSucceeded, RouteModelCode: "route-a", TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k", Attempts: []domainimagetask.Attempt{{StartedAt: &inside}}},
		{UserID: 91, ID: "00000000-0000-4000-8000-000000000006", Status: domainimagetask.StatusSucceeded, RouteModelCode: "route-a", TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k", Attempts: []domainimagetask.Attempt{{StartedAt: &inside}, {StartedAt: &inside}}},
		{UserID: 91, ID: "00000000-0000-4000-8000-000000000007", Status: domainimagetask.StatusSucceeded, RouteModelCode: "route-b", TaskType: string(provider.TaskTypeTextToImage), AbstractModel: "basic", BaseResolution: "1k", Attempts: []domainimagetask.Attempt{{StartedAt: &inside}}},
	}
	for _, seed := range seeds {
		trace, err := buildProviderTrace(seed)
		if err != nil {
			t.Fatalf("build trace %s: %v", seed.ID, err)
		}
		id := uuid.MustParse(seed.ID)
		if _, err := client.ImageTask.Create().
			SetID(id).
			SetUserID(seed.UserID).
			SetTaskType(seed.TaskType).
			SetStatus(seed.Status).
			SetPrompt("distribution test").
			SetAbstractModel(seed.AbstractModel).
			SetBaseResolution(seed.BaseResolution).
			SetRouteModelCode(seed.RouteModelCode).
			SetProviderTrace(trace).
			SetCreatedAt(inside).
			SetUpdatedAt(inside).
			Save(ctx); err != nil {
			t.Fatalf("create task %s: %v", seed.ID, err)
		}
	}
	if _, err := client.ImageTask.UpdateOneID(uuid.MustParse(seeds[5].ID)).SetDeletedAt(inside).SetUpdatedAt(inside).Save(ctx); err != nil {
		t.Fatalf("soft-delete task: %v", err)
	}
	candidateCount, err := client.ImageTask.Query().Where(imagetask.CreatedAtLT(request.To), imagetask.UpdatedAtGTE(request.From)).Count(ctx)
	if err != nil || candidateCount != len(seeds) {
		t.Fatalf("distribution candidates = %d, want %d: %v", candidateCount, len(seeds), err)
	}

	callRecords := NewAdminCallRecordStore(client)
	callRecords.batchSize = 2
	distribution, err := callRecords.CallDistribution(ctx, request)
	if err != nil {
		t.Fatalf("CallDistribution: %v", err)
	}
	if distribution.TotalCalls != 8 || distribution.PreflightFailureCount != 1 {
		t.Fatalf("distribution totals = %#v", distribution)
	}
	wantGroups := []domainadmincallrecord.DistributionGroup{
		{Key: "route-a", Calls: 5, Percentage: float64(5) * 100 / 8},
		{Key: "route-b", Calls: 2, Percentage: float64(2) * 100 / 8},
		{Key: "unrouted", Calls: 1, Percentage: float64(1) * 100 / 8},
	}
	if !reflect.DeepEqual(distribution.Groups, wantGroups) {
		t.Fatalf("distribution groups = %#v, want %#v", distribution.Groups, wantGroups)
	}
}

func TestAdminCallRecordStoreDistributionRejectsMalformedLegacyTrace(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:admin-call-distribution-malformed?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	inside := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	_, err = client.ImageTask.Create().
		SetID(uuid.MustParse("00000000-0000-4000-8000-000000000099")).
		SetUserID(99).
		SetTaskType(string(provider.TaskTypeTextToImage)).
		SetStatus(domainimagetask.StatusFailed).
		SetPrompt("legacy").
		SetAbstractModel("basic").
		SetCreatedAt(inside).
		SetUpdatedAt(inside).
		SetProviderTrace(map[string]any{"attempts": "password=do-not-leak"}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create malformed legacy task: %v", err)
	}

	_, err = NewAdminCallRecordStore(client).CallDistribution(ctx, domainadmincallrecord.DistributionRequest{From: inside.Add(-time.Hour), To: inside.Add(time.Hour)})
	if err == nil || !strings.Contains(err.Error(), "invalid call distribution trace") {
		t.Fatalf("CallDistribution error = %v, want stable invalid trace error", err)
	}
	if strings.Contains(err.Error(), "password=do-not-leak") {
		t.Fatalf("CallDistribution leaked malformed trace: %v", err)
	}
}

func TestAdminCallRecordStoreClassifiesExhaustedArtifactRecoveryAsPlatformLoss(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:admin-call-record-artifact-loss?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	upstreamSucceededAt := time.Date(2026, 7, 16, 2, 3, 4, 0, time.UTC)
	task := domainimagetask.Task{
		UserID: 7, ID: "eeeeeeee-eeee-eeee-eeee-eeeeeeeeeeee", Status: domainimagetask.StatusFailed,
		Provider: "openrouter", AbstractModel: "plus", TaskType: string(provider.TaskTypeTextToImage),
		BaseResolution: "2k", Quality: "auto", OutputImageCount: 1,
		ProviderRequestID: "paid-request-123", ProviderCost: "0.12345", GrossMargin: "-0.12345",
		UpstreamSucceededAt: &upstreamSucceededAt, ErrorCode: "ARTIFACT_STORAGE_WRITE_FAILED", ErrorMessage: "artifact persistence failed after 4 attempts",
		ArtifactRecovery: domainimagetask.ArtifactRecovery{
			Status: "failed", AttemptCount: 4, EncryptedPayload: "ciphertext-must-not-leak",
			LastDiagnostic: domainimagetask.ArtifactDiagnostic{Code: "ARTIFACT_STORAGE_WRITE_FAILED", Stage: "store", Attempt: 4, StorageConfigID: "11111111-1111-1111-1111-111111111111", StorageVersion: 3, Retryable: true, Cause: "temporary storage failure"},
			Diagnostics:    []domainimagetask.ArtifactDiagnostic{{Code: "ARTIFACT_FETCH_TIMEOUT", Stage: "fetch", Attempt: 1, URLHost: "cdn.example.com", URLPath: "/private/result.png", Cause: "request timed out"}},
		},
	}
	if err := NewImageTaskStore(client).Save(ctx, task); err != nil {
		t.Fatalf("save task: %v", err)
	}

	page, err := NewAdminCallRecordStore(client).ListCallRecords(ctx, domainadmincallrecord.ListRequest{Page: 1, PageSize: 10, TaskID: task.ID})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list call record: page=%#v err=%v", page, err)
	}
	record := page.Items[0]
	if record.FailurePhase != "artifact_persistence" || !record.PlatformLoss {
		t.Fatalf("expected platform artifact loss classification, got %#v", record)
	}
	if record.ProviderRequestID != "paid-request-123" || record.ProviderCost != "0.12345" || record.UpstreamSucceededAt == nil {
		t.Fatalf("expected upstream success and provider cost evidence, got %#v", record)
	}
	if record.ArtifactRecovery == nil || record.ArtifactRecovery.AttemptCount != 4 || record.ArtifactRecovery.LastDiagnostic.Stage != "store" || len(record.ArtifactRecovery.Diagnostics) != 1 || record.ArtifactRecovery.Diagnostics[0].URLHost != "cdn.example.com" {
		t.Fatalf("expected artifact diagnostics, got %#v", record.ArtifactRecovery)
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	if bytes.Contains(encoded, []byte("ciphertext-must-not-leak")) || bytes.Contains(encoded, []byte("signature=")) {
		t.Fatalf("call record leaked recovery secret: %s", encoded)
	}
	lossPage, err := NewAdminCallRecordStore(client).ListCallRecords(ctx, domainadmincallrecord.ListRequest{Page: 1, PageSize: 1, PlatformLossOnly: true})
	if err != nil || lossPage.Total != 1 || len(lossPage.Items) != 1 || lossPage.Items[0].TaskID != task.ID {
		t.Fatalf("platform loss filter did not return the exhausted recovery: page=%#v err=%v", lossPage, err)
	}
}

func (s adminCallRecordStaticRoutingSource) ModelRoutingConfig(context.Context) (modelhub.ModelRoutingSnapshot, error) {
	return s.snapshot, nil
}

func TestAdminCallRecordStoreListsImageTasksWithFilters(t *testing.T) {
	ctx := context.Background()
	client, err := repoent.Open(dialect.SQLite, "file:admin-call-records?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	defer client.Close()
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}

	imageTasks := NewImageTaskStore(client)
	firstStarted := time.Date(2026, 5, 22, 8, 0, 0, 0, time.UTC)
	firstFinished := firstStarted.Add(2 * time.Second)
	seedTasks := []domainimagetask.Task{
		{
			UserID:            42,
			APIKeyID:          99,
			SourceChannel:     "openapi",
			ID:                "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
			Status:            domainimagetask.StatusFailed,
			Provider:          "openrouter",
			AccountModelID:    1201,
			ModelAccountID:    2201,
			UpstreamModelCode: "google/gemini-2.5-flash-image",
			AbstractModel:     "plus",
			TaskType:          string(provider.TaskTypeTextToImage),
			Prompt:            "failed",

			BaseResolution:      "2k",
			Quality:             "auto",
			OutputImageCount:    3,
			ReferenceImageCount: 1,
			EstimatedPoints:     "12.00000",
			ActualPoints:        "4.00000",
			ErrorCode:           "provider_error",
			ErrorMessage:        "upstream failed",
			Attempts: []domainimagetask.Attempt{
				{
					Provider:       "openai",
					AdapterType:    "openai_compatible",
					AccountModelID: 1200,
					ModelAccountID: 2200,
					ModelCode:      "gpt-image-1",
					Status:         domainimagetask.StatusFailed,
					Error:          "timeout",
					ErrorCode:      "timeout",
					ErrorMessage:   "provider timed out",
					ErrorDetail:    map[string]any{"http_status": 504.0, "request_id": "req-timeout", "family": "unavailable", "action": "retry"},
				},
				{
					Provider:       "openrouter",
					AdapterType:    "openrouter",
					AccountModelID: 1201,
					ModelAccountID: 2201,
					ModelCode:      "google/gemini-2.5-flash-image",
					Status:         domainimagetask.StatusFailed,
					Error:          "quota",
					ErrorCode:      "insufficient_quota",
					ErrorMessage:   "quota exceeded",
					ErrorDetail:    map[string]any{"http_status": 429.0, "request_id": "req-quota", "family": "rate_limited", "action": "retry"},
				},
			},
		},
		{
			UserID:        42,
			APIKeyID:      100,
			SourceChannel: "web",
			ID:            "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb",
			Status:        domainimagetask.StatusSucceeded,
			Provider:      "openai",
			AbstractModel: "basic",
			TaskType:      string(provider.TaskTypeTextToImage),
			Prompt:        "succeeded",

			BaseResolution:      "1k",
			OutputImageCount:    1,
			ReferenceImageCount: 0,
			EstimatedPoints:     "2.00000",
			ActualPoints:        "2.00000",
		},
		{
			UserID:        42,
			APIKeyID:      101,
			SourceChannel: "openapi",
			ID:            "dddddddd-dddd-dddd-dddd-dddddddddddd",
			Status:        domainimagetask.StatusFailed,
			Provider:      "openrouter",
			AbstractModel: "plus",
			TaskType:      string(provider.TaskTypeTextToImage),
			Prompt:        "failed with another code",

			BaseResolution:      "2k",
			OutputImageCount:    3,
			ReferenceImageCount: 1,
			EstimatedPoints:     "12.00000",
			ActualPoints:        "0.00000",
			ErrorCode:           "another_error",
			ErrorMessage:        "another failure",
		},
	}
	for _, task := range seedTasks {
		if err := imageTasks.Save(ctx, task); err != nil {
			t.Fatalf("Save %s: %v", task.ID, err)
		}
	}
	firstTask, err := client.ImageTask.Query().Where(imagetask.IDEQ(uuid.MustParse(seedTasks[0].ID))).Only(ctx)
	if err != nil {
		t.Fatalf("query first task: %v", err)
	}
	if err := client.ImageTask.UpdateOne(firstTask).
		SetStartedAt(firstStarted).
		SetFinishedAt(firstFinished).
		Exec(ctx); err != nil {
		t.Fatalf("set timestamps: %v", err)
	}

	store := NewAdminCallRecordStore(client)
	page, err := store.ListCallRecords(ctx, domainadmincallrecord.ListRequest{
		Page:          1,
		PageSize:      10,
		Status:        domainimagetask.StatusFailed,
		ErrorCode:     "provider_error",
		Provider:      "openrouter",
		SourceChannel: "openapi",
		UserID:        42,
		TaskID:        "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa",
		CreatedFrom:   firstTask.CreatedAt.Add(-time.Hour),
		CreatedTo:     firstTask.CreatedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("ListCallRecords: %v", err)
	}
	if page.Total != 1 || page.Page != 1 || page.PageSize != 10 || len(page.Items) != 1 {
		t.Fatalf("unexpected page %#v", page)
	}
	record := page.Items[0]
	if record.TaskID != seedTasks[0].ID || record.UserID != 42 || record.APIKeyID == nil || *record.APIKeyID != 99 {
		t.Fatalf("unexpected identity fields %#v", record)
	}
	if record.SourceChannel != "openapi" || record.TaskType != string(provider.TaskTypeTextToImage) || record.Status != domainimagetask.StatusFailed {
		t.Fatalf("unexpected classification fields %#v", record)
	}
	if record.FailurePhase != "upstream" || record.PlatformLoss {
		t.Fatalf("expected upstream failure without platform loss, got %#v", record)
	}
	if record.Provider != "openrouter" || record.AbstractModel != "plus" || record.BaseResolution != "2k" || record.Quality != "auto" {
		t.Fatalf("unexpected model fields %#v", record)
	}
	if record.AccountModelID == nil || *record.AccountModelID != 1201 || record.ModelAccountID == nil || *record.ModelAccountID != 2201 || record.UpstreamModelCode != "google/gemini-2.5-flash-image" {
		t.Fatalf("unexpected upstream model fields %#v", record)
	}
	if record.RequestedOutputImageCount != 3 || record.SuccessOutputImageCount != 0 || record.ReferenceImageCount != 1 {
		t.Fatalf("unexpected count fields %#v", record)
	}
	if record.EstimatedPoints != "12.00000" || record.ActualPoints != "4.00000" {
		t.Fatalf("unexpected points fields %#v", record)
	}
	if record.ErrorCode == nil || *record.ErrorCode != "provider_error" || record.ErrorMessage == nil || *record.ErrorMessage != "upstream failed" {
		t.Fatalf("unexpected error fields %#v", record)
	}
	if record.StartedAt == nil || !record.StartedAt.Equal(firstStarted) || record.FinishedAt == nil || !record.FinishedAt.Equal(firstFinished) {
		t.Fatalf("unexpected timestamps %#v", record)
	}
	if record.AttemptCount != 2 {
		t.Fatalf("expected 2 attempts, got %d", record.AttemptCount)
	}
	if len(record.Attempts) != 2 {
		t.Fatalf("expected attempts to be returned, got %#v", record.Attempts)
	}
	lastAttempt := record.Attempts[1]
	if lastAttempt.Provider != "openrouter" || lastAttempt.AccountModelID == nil || *lastAttempt.AccountModelID != 1201 || lastAttempt.ModelAccountID == nil || *lastAttempt.ModelAccountID != 2201 || lastAttempt.ModelCode != "google/gemini-2.5-flash-image" {
		t.Fatalf("unexpected last attempt routing fields %#v", lastAttempt)
	}
	if lastAttempt.ErrorCode != "insufficient_quota" || lastAttempt.ErrorMessage != "quota exceeded" {
		t.Fatalf("unexpected last attempt error summary %#v", lastAttempt)
	}
	if record.ErrorDetail["request_id"] != "req-quota" || record.ErrorDetail["family"] != "rate_limited" {
		t.Fatalf("unexpected record error detail %#v", record.ErrorDetail)
	}

	errorFilteredPage, err := store.ListCallRecords(ctx, domainadmincallrecord.ListRequest{
		Page:      1,
		PageSize:  10,
		Status:    domainimagetask.StatusFailed,
		ErrorCode: "provider_error",
		Provider:  "openrouter",
	})
	if err != nil {
		t.Fatalf("ListCallRecords by error_code: %v", err)
	}
	if errorFilteredPage.Total != 1 || len(errorFilteredPage.Items) != 1 || errorFilteredPage.Items[0].TaskID != seedTasks[0].ID {
		t.Fatalf("expected error_code filter to return only first task, got %#v", errorFilteredPage)
	}
}

func TestAdminCallRecordStoreProjectsImmutableRouteAndPricingSnapshots(t *testing.T) {
	ctx := t.Context()
	client, err := repoent.Open(dialect.SQLite, "file:admin-call-history?mode=memory&cache=shared&_fk=1")
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(ctx); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	task, err := client.ImageTask.Create().
		SetUserID(42).
		SetTaskType("text_to_image").
		SetPrompt("historical").
		SetAbstractModel("plus").
		SetRouteModelCode("route-snapshot").
		SetUpstreamModelCode("gpt-image-snapshot").
		SetProviderCost("0.42000").
		SetPricingSnapshot(map[string]any{"route_model_code": "route-snapshot", "base_unit_points": "8.00000", "total_points": "8.00000"}).
		Save(ctx)
	if err != nil {
		t.Fatalf("create historical task: %v", err)
	}
	page, err := NewAdminCallRecordStore(client).ListCallRecords(ctx, domainadmincallrecord.ListRequest{Page: 1, PageSize: 10, TaskID: task.ID.String()})
	if err != nil || len(page.Items) != 1 {
		t.Fatalf("list call record: %#v err=%v", page, err)
	}
	record := page.Items[0]
	if record.RouteModelCode != "route-snapshot" || record.UpstreamModelCode != "gpt-image-snapshot" || record.ProviderCost != "0.42000" {
		t.Fatalf("historical routing/cost snapshots were lost: %#v", record)
	}
	if record.PricingSnapshot["base_unit_points"] != "8.00000" || record.PricingSnapshot["total_points"] != "8.00000" {
		t.Fatalf("historical pricing snapshot was lost: %#v", record.PricingSnapshot)
	}
}

func TestAdminCallRecordStoreListsCreateTaskRoutePreflightFailures(t *testing.T) {
	testCases := []struct {
		name          string
		taskID        string
		routeCode     string
		snapshot      modelhub.ModelRoutingSnapshot
		expectedCode  string
		expectedModel string
	}{
		{
			name:          "missing route model",
			taskID:        "11111111-1111-4111-8111-111111111111",
			routeCode:     "missing-route-model-code-over-32-chars",
			snapshot:      modelhub.ModelRoutingSnapshot{},
			expectedCode:  "MODEL_ROUTE_NOT_FOUND",
			expectedModel: "missing-route-model-code-over-32-chars",
		},
		{
			name:      "route model has no candidate",
			taskID:    "22222222-2222-4222-8222-222222222222",
			routeCode: "plus",
			snapshot: modelhub.ModelRoutingSnapshot{
				RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
				Prices:      []modelhub.RoutePriceConfig{{RouteModelID: 1, TaskType: string(provider.TaskTypeTextToImage), BaseResolution: "2k", BasePoints: "4.00000", Enabled: true}},
				ProviderModels: []modelhub.ProviderCandidate{
					{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{string(provider.TaskTypeTextToImage)}, SupportedBaseResolution: []string{"1k"}},
				},
				Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
			},
			expectedCode:  errs.CodeImageCapabilityMismatch,
			expectedModel: "plus",
		},
		{
			name:      "route model price missing",
			taskID:    "33333333-3333-4333-8333-333333333333",
			routeCode: "plus",
			snapshot: modelhub.ModelRoutingSnapshot{
				RouteModels: []modelhub.RouteModelConfig{{ID: 1, Code: "plus", Name: "Plus", Visibility: "public", Enabled: true}},
				ProviderModels: []modelhub.ProviderCandidate{
					{AccountModelID: 12, ModelAccountID: 102, ModelCode: "gpt-image-1", SupportedTaskTypes: []string{string(provider.TaskTypeTextToImage)}, SupportedBaseResolution: []string{"2k"}},
				},
				Candidates: []modelhub.RouteCandidateConfig{{RouteModelID: 1, AccountModelID: 12, Priority: 1, Enabled: true}},
			},
			expectedCode:  "ROUTE_MODEL_PRICE_MISSING",
			expectedModel: "plus",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			client, err := repoent.Open(dialect.SQLite, "file:"+tc.taskID+"?mode=memory&cache=shared&_fk=1")
			if err != nil {
				t.Fatalf("open ent client: %v", err)
			}
			defer client.Close()
			if err := client.Schema.Create(ctx); err != nil {
				t.Fatalf("create schema: %v", err)
			}

			imageTasks := NewImageTaskStore(client)
			taskService := imagetaskservice.NewServiceWithProvidersAndStore(routePreflightTaskConfig(), nil, imageTasks)
			taskService.SetModelRoutingSource(adminCallRecordStaticRoutingSource{snapshot: tc.snapshot})

			_, err = taskService.CreateTask(ctx, domainimagetask.CreateRequest{
				TaskID:           tc.taskID,
				UserID:           902,
				SourceChannel:    "web",
				RouteModelCode:   tc.routeCode,
				TaskType:         string(provider.TaskTypeTextToImage),
				Prompt:           "preflight failure should be visible to ops",
				RequestedSize:    "auto",
				BaseResolution:   "auto",
				OutputImageCount: 1,
			})
			if err == nil {
				t.Fatal("expected CreateTask to fail preflight")
			}

			callRecords := NewAdminCallRecordStore(client)
			page, err := callRecords.ListCallRecords(ctx, domainadmincallrecord.ListRequest{
				Page:      1,
				PageSize:  10,
				Status:    domainimagetask.StatusFailed,
				ErrorCode: tc.expectedCode,
			})
			if err != nil {
				t.Fatalf("ListCallRecords: %v", err)
			}
			if page.Total != 1 || len(page.Items) != 1 {
				t.Fatalf("expected one failed call record for %s, got %#v", tc.expectedCode, page)
			}
			record := page.Items[0]
			if record.TaskID != tc.taskID || record.UserID != 902 || record.Status != domainimagetask.StatusFailed {
				t.Fatalf("unexpected record identity/status %#v", record)
			}
			if record.ErrorCode == nil || *record.ErrorCode != tc.expectedCode {
				t.Fatalf("expected error code %s, got %#v", tc.expectedCode, record.ErrorCode)
			}
			if record.AbstractModel != tc.expectedModel || record.ActualPoints != "0.00000" {
				t.Fatalf("unexpected model/points fields %#v", record)
			}
		})
	}
}

func routePreflightTaskConfig() config.Config {
	cfg := config.Config{}
	cfg.Billing.AutoBaseResolutionDefaultByGroup = map[string]string{"plus": "2k"}
	cfg.GenerationLimits.MaxImageCount = 5
	cfg.GenerationLimits.ReferenceImageMaxCount = 4
	return cfg
}
