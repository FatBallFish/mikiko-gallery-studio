package entstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/google/uuid"

	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	repoent "github.com/fatballfish/pic-gallery/internal/repository/ent"
)

func TestBuildProviderTraceEnforcesCompactJSONByteLimit(t *testing.T) {
	exact := providerTraceTaskWithCompactSize(t, 8<<20)
	trace, err := buildProviderTrace(exact)
	if err != nil {
		t.Fatalf("build exact-limit provider trace: %v", err)
	}
	payload, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal exact-limit provider trace: %v", err)
	}
	if len(payload) != 8<<20 {
		t.Fatalf("exact provider trace size = %d, want %d", len(payload), 8<<20)
	}

	over := exact
	over.Attempts = append([]domainimagetask.Attempt(nil), exact.Attempts...)
	over.Attempts[0].ErrorDetail = map[string]any{
		"padding": exact.Attempts[0].ErrorDetail["padding"].(string) + "x",
	}
	if _, err := buildProviderTrace(over); err == nil || err.Error() != "provider trace exceeds limits" {
		t.Fatalf("over-limit provider trace error = %v, want sanitized limit error", err)
	}
}

func TestBuildProviderTraceEnforcesAttemptLimit(t *testing.T) {
	exact := domainimagetask.Task{Attempts: make([]domainimagetask.Attempt, 10_000)}
	if _, err := buildProviderTrace(exact); err != nil {
		t.Fatalf("build exact attempt-limit trace: %v", err)
	}

	over := domainimagetask.Task{Attempts: make([]domainimagetask.Attempt, 10_001)}
	if _, err := buildProviderTrace(over); err == nil || err.Error() != "provider trace exceeds limits" {
		t.Fatalf("over attempt-limit provider trace error = %v, want sanitized limit error", err)
	}
}

func TestImageTaskStoreRejectsProviderTraceLimitBeforeEveryPersistencePath(t *testing.T) {
	ctx := context.Background()
	client, _ := openProviderTraceDebugSQLite(t, "write-boundaries")
	store := NewImageTaskStore(client)
	now := time.Date(2026, 8, 10, 2, 30, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	running := domainimagetask.Task{
		ID:             "40000000-0000-4000-8000-000000000001",
		UserID:         4,
		Status:         domainimagetask.StatusRunning,
		LeaseOwner:     "provider-trace-worker",
		LeaseExpiresAt: &expiresAt,
		TaskType:       string(provider.TaskTypeTextToImage),
		Prompt:         "write boundary",
		AbstractModel:  "basic",
	}
	if err := store.Save(ctx, running); err != nil {
		t.Fatalf("seed running task: %v", err)
	}

	over := running
	over.Attempts = make([]domainimagetask.Attempt, maxProviderTraceAttempts+1)
	for name, persist := range map[string]func() error{
		"save": func() error {
			candidate := over
			candidate.ID = "40000000-0000-4000-8000-000000000002"
			return store.Save(ctx, candidate)
		},
		"save if owned": func() error {
			return store.SaveIfOwned(ctx, over, running.LeaseOwner, now)
		},
		"save terminal state": func() error {
			candidate := over
			candidate.Status = domainimagetask.StatusFailed
			return store.SaveTerminalState(ctx, candidate, running.LeaseOwner, now)
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := persist(); !errors.Is(err, errProviderTraceExceedsLimits) {
				t.Fatalf("persistence error = %v, want provider trace limit", err)
			}
		})
	}
	if count, err := client.ImageTask.Query().Count(ctx); err != nil || count != 1 {
		t.Fatalf("persisted task count = %d, want unchanged 1: %v", count, err)
	}
	entity, err := client.ImageTask.Get(ctx, uuid.MustParse(running.ID))
	if err != nil {
		t.Fatalf("reload running task: %v", err)
	}
	attempts, err := decodeAttempts(entity.ProviderTrace["attempts"])
	if err != nil || len(attempts) != 0 || entity.Status != domainimagetask.StatusRunning {
		t.Fatalf("running task changed after rejected persistence: status=%s attempts=%d err=%v", entity.Status, len(attempts), err)
	}
}

func TestCallDistributionRejectsOversizedHistoricalTraceBeforeRawFetch(t *testing.T) {
	ctx := context.Background()
	client, logs := openProviderTraceDebugSQLite(t, "distribution-preflight")
	inside := time.Date(2026, 8, 10, 3, 0, 0, 0, time.UTC)
	seedProviderTraceTask(t, ctx, client, uuid.MustParse("10000000-0000-4000-8000-000000000001"), inside, map[string]any{
		"padding": strings.Repeat("x", (16<<20)+1),
	})
	*logs = nil

	_, err := NewAdminCallRecordStore(client).CallDistribution(ctx, providerTraceDistributionRequest(inside))
	if err == nil || err.Error() != "invalid call distribution trace" {
		t.Fatalf("CallDistribution error = %v, want sanitized invalid trace error", err)
	}
	metadataQueries, rawQueries := providerTraceQueryCounts(*logs)
	if metadataQueries != 1 || rawQueries != 0 {
		t.Fatalf("provider trace queries: metadata=%d raw=%d, want metadata=1 raw=0\n%s", metadataQueries, rawQueries, strings.Join(*logs, "\n"))
	}
}

func TestCallDistributionBoundsRawTraceSubBatchesAcrossMetadataPages(t *testing.T) {
	ctx := context.Background()
	client, logs := openProviderTraceDebugSQLite(t, "distribution-batch-budget")
	inside := time.Date(2026, 8, 10, 4, 0, 0, 0, time.UTC)
	largeTask := providerTraceTaskWithCompactSize(t, 6<<20)
	trace, err := buildProviderTrace(largeTask)
	if err != nil {
		t.Fatalf("build large legal trace: %v", err)
	}
	for index := 1; index <= 4; index++ {
		seedProviderTraceTask(t, ctx, client, uuid.MustParse(fmt.Sprintf("20000000-0000-4000-8000-%012d", index)), inside, trace)
	}
	*logs = nil

	store := NewAdminCallRecordStore(client)
	store.batchSize = 3
	distribution, err := store.CallDistribution(ctx, providerTraceDistributionRequest(inside))
	if err != nil {
		t.Fatalf("CallDistribution: %v", err)
	}
	if distribution.TotalCalls != 4 {
		t.Fatalf("distribution total calls = %d, want 4", distribution.TotalCalls)
	}
	metadataQueries, rawQueries := providerTraceQueryCounts(*logs)
	if metadataQueries != 2 || rawQueries != 3 {
		t.Fatalf("provider trace queries: metadata=%d raw=%d, want metadata=2 raw=3\n%s", metadataQueries, rawQueries, strings.Join(*logs, "\n"))
	}
}

func TestCallDistributionObservesCancellationDuringLargeTraceDecode(t *testing.T) {
	ctx := context.Background()
	client, _ := openProviderTraceDebugSQLite(t, "distribution-cancel")
	inside := time.Date(2026, 8, 10, 5, 0, 0, 0, time.UTC)
	largeTask := providerTraceTaskWithCompactSize(t, 1<<20)
	trace, err := buildProviderTrace(largeTask)
	if err != nil {
		t.Fatalf("build large legal trace: %v", err)
	}
	seedProviderTraceTask(t, ctx, client, uuid.MustParse("30000000-0000-4000-8000-000000000001"), inside, trace)

	cancelDuringDecode := &providerTraceCancelContext{Context: context.Background(), cancelAfter: 2}
	_, err = NewAdminCallRecordStore(client).CallDistribution(cancelDuringDecode, providerTraceDistributionRequest(inside))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("CallDistribution error = %v, want context cancellation during trace decode", err)
	}
}

func TestDecodeCallDistributionAttemptsEnforcesExactAttemptBoundary(t *testing.T) {
	exact := providerTraceAttemptArrayJSON(maxProviderTraceAttempts)
	attempts, err := decodeCallDistributionAttempts(context.Background(), exact)
	if err != nil || len(attempts) != maxProviderTraceAttempts {
		t.Fatalf("decode exact attempt boundary: attempts=%d err=%v", len(attempts), err)
	}
	over := providerTraceAttemptArrayJSON(maxProviderTraceAttempts + 1)
	if _, err := decodeCallDistributionAttempts(context.Background(), over); !errors.Is(err, errInvalidCallDistributionTrace) {
		t.Fatalf("decode over attempt boundary error = %v, want sanitized invalid trace", err)
	}
}

func TestDecodeCallDistributionAttemptsEnforcesExactSemanticByteBoundary(t *testing.T) {
	exactTrace := historicalProviderTraceWithCompactSize(t, maxProviderTraceSemanticBytes)
	exact, err := json.Marshal(exactTrace)
	if err != nil {
		t.Fatalf("marshal exact semantic trace: %v", err)
	}
	if _, err := decodeCallDistributionAttempts(context.Background(), exact); err != nil {
		t.Fatalf("decode exact semantic byte boundary: %v", err)
	}

	overTrace := historicalProviderTraceWithCompactSize(t, maxProviderTraceSemanticBytes+1)
	over, err := json.Marshal(overTrace)
	if err != nil {
		t.Fatalf("marshal over semantic trace: %v", err)
	}
	if _, err := decodeCallDistributionAttempts(context.Background(), over); !errors.Is(err, errInvalidCallDistributionTrace) {
		t.Fatalf("decode over semantic byte boundary error = %v, want sanitized invalid trace", err)
	}
}

func TestDecodeCallDistributionAttemptsRejectsMalformedKnownFieldTypes(t *testing.T) {
	trace := []byte(`{"attempts":[{"provider":123,"account_model_id":"bad","started_at":"2026-08-10T00:00:00Z"}]}`)
	if _, err := decodeCallDistributionAttempts(context.Background(), trace); !errors.Is(err, errInvalidCallDistributionTrace) || err.Error() != "invalid call distribution trace" {
		t.Fatalf("malformed known-field error = %v, want sanitized invalid trace", err)
	}
}

func TestCallDistributionTraceLengthSelectionUsesDialectByteLength(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		dialect string
		want    string
	}{
		{name: "sqlite", dialect: dialect.SQLite, want: "length(CAST(`image_tasks`.`provider_trace` AS BLOB))"},
		{name: "postgres", dialect: dialect.Postgres, want: "octet_length(CAST(\"image_tasks\".\"provider_trace\" AS text))"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			table := entsql.Dialect(testCase.dialect).Table("image_tasks")
			selector := entsql.Dialect(testCase.dialect).Select(table.C("id")).From(table)
			callDistributionTraceLengthSelection()(selector)
			query, _ := selector.Query()
			if !strings.Contains(query, testCase.want) || !strings.Contains(query, callDistributionTraceBytesAlias) {
				t.Fatalf("byte-length query = %s, want %s and alias", query, testCase.want)
			}
		})
	}
}

func providerTraceTaskWithCompactSize(t *testing.T, target int) domainimagetask.Task {
	t.Helper()
	task := domainimagetask.Task{
		Attempts: []domainimagetask.Attempt{{ErrorDetail: map[string]any{"padding": ""}}},
	}
	trace, err := buildProviderTrace(task)
	if err != nil {
		t.Fatalf("build provider trace baseline: %v", err)
	}
	payload, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal provider trace baseline: %v", err)
	}
	paddingSize := target - len(payload)
	if paddingSize < 0 {
		t.Fatalf("target provider trace size %d is below baseline %d", target, len(payload))
	}
	task.Attempts[0].ErrorDetail["padding"] = strings.Repeat("x", paddingSize)
	return task
}

func historicalProviderTraceWithCompactSize(t *testing.T, target int) map[string]any {
	t.Helper()
	trace := map[string]any{
		"attempts": []any{map[string]any{"error_detail": map[string]any{"padding": ""}}},
	}
	payload, err := json.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal historical trace baseline: %v", err)
	}
	padding := target - len(payload)
	if padding < 0 {
		t.Fatalf("historical trace target %d is below baseline %d", target, len(payload))
	}
	trace["attempts"].([]any)[0].(map[string]any)["error_detail"].(map[string]any)["padding"] = strings.Repeat("x", padding)
	payload, err = json.Marshal(trace)
	if err != nil {
		t.Fatalf("marshal sized historical trace: %v", err)
	}
	if len(payload) != target {
		t.Fatalf("historical compact trace size = %d, want %d", len(payload), target)
	}
	return trace
}

func providerTraceAttemptArrayJSON(count int) []byte {
	var builder strings.Builder
	builder.Grow(16 + count*3)
	builder.WriteString(`{"attempts":[`)
	for index := 0; index < count; index++ {
		if index > 0 {
			builder.WriteByte(',')
		}
		builder.WriteString("{}")
	}
	builder.WriteString("]}")
	return []byte(builder.String())
}

type providerTraceCancelContext struct {
	context.Context
	errChecks   atomic.Int64
	cancelAfter int64
}

func (c *providerTraceCancelContext) Err() error {
	if c.errChecks.Add(1) >= c.cancelAfter {
		return context.Canceled
	}
	return nil
}

func openProviderTraceDebugSQLite(t *testing.T, name string) (*repoent.Client, *[]string) {
	t.Helper()
	logs := []string{}
	client, err := repoent.Open(
		dialect.SQLite,
		"file:"+name+"-"+uuid.NewString()+"?mode=memory&cache=shared&_fk=1",
		repoent.Debug(),
		repoent.Log(func(values ...any) { logs = append(logs, fmt.Sprint(values...)) }),
	)
	if err != nil {
		t.Fatalf("open ent client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if err := client.Schema.Create(context.Background()); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	return client, &logs
}

func seedProviderTraceTask(t *testing.T, ctx context.Context, client *repoent.Client, id uuid.UUID, at time.Time, trace map[string]any) {
	t.Helper()
	_, err := client.ImageTask.Create().
		SetID(id).
		SetUserID(1).
		SetTaskType(string(provider.TaskTypeTextToImage)).
		SetStatus(domainimagetask.StatusSucceeded).
		SetPrompt("provider trace boundary test").
		SetAbstractModel("basic").
		SetRouteModelCode("route-bounded").
		SetProviderTrace(trace).
		SetCreatedAt(at).
		SetUpdatedAt(at).
		Save(ctx)
	if err != nil {
		t.Fatalf("seed provider trace task %s: %v", id, err)
	}
}

func providerTraceDistributionRequest(inside time.Time) domainadmincallrecord.DistributionRequest {
	return domainadmincallrecord.DistributionRequest{From: inside.Add(-time.Hour), To: inside.Add(time.Hour)}
}

func providerTraceQueryCounts(logs []string) (metadata, raw int) {
	for _, entry := range logs {
		upper := strings.ToUpper(entry)
		selectStart := strings.Index(upper, "SELECT ")
		fromStart := strings.Index(upper, " FROM ")
		if selectStart < 0 || fromStart <= selectStart {
			continue
		}
		selection := strings.ToLower(entry[selectStart:fromStart])
		if strings.Contains(selection, "provider_trace_bytes") {
			metadata++
			continue
		}
		if strings.Contains(selection, "provider_trace") {
			raw++
		}
	}
	return metadata, raw
}
