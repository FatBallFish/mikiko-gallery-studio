package entstore

import (
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestCallDistributionProviderTraceBoundariesPostgres(t *testing.T) {
	ctx, database, client, _ := openProjectTaskPostgres(t)
	exactAt := time.Date(2026, 8, 10, 6, 0, 0, 0, time.UTC)
	exactTask := providerTraceTaskWithCompactSize(t, maxProviderTraceSemanticBytes)
	exactTrace, err := buildProviderTrace(exactTask)
	if err != nil {
		t.Fatalf("build exact semantic-limit trace: %v", err)
	}
	exactID := uuid.MustParse("50000000-0000-4000-8000-000000000001")
	seedProviderTraceTask(t, ctx, client, exactID, exactAt, exactTrace)

	var transportBytes int
	if err := database.QueryRowContext(ctx, `SELECT octet_length(provider_trace::text) FROM image_tasks WHERE id = $1`, exactID).Scan(&transportBytes); err != nil {
		t.Fatalf("query PostgreSQL transport bytes: %v", err)
	}
	if transportBytes <= maxProviderTraceSemanticBytes || transportBytes > maxCallDistributionTraceTransportBytes {
		t.Fatalf("PostgreSQL transport bytes = %d, want semantic limit < transport <= transport limit", transportBytes)
	}
	distribution, err := NewAdminCallRecordStore(client).CallDistribution(ctx, providerTraceDistributionRequest(exactAt))
	if err != nil || distribution.TotalCalls != 1 {
		t.Fatalf("read exact semantic-limit PostgreSQL trace: distribution=%#v err=%v", distribution, err)
	}

	overAt := exactAt.Add(24 * time.Hour)
	overTrace := historicalProviderTraceWithCompactSize(t, maxProviderTraceSemanticBytes+1)
	overID := uuid.MustParse("50000000-0000-4000-8000-000000000002")
	seedProviderTraceTask(t, ctx, client, overID, overAt, overTrace)
	_, err = NewAdminCallRecordStore(client).CallDistribution(ctx, providerTraceDistributionRequest(overAt))
	if !errors.Is(err, errInvalidCallDistributionTrace) || err.Error() != "invalid call distribution trace" {
		t.Fatalf("read over semantic-limit PostgreSQL trace error = %v, want sanitized invalid trace", err)
	}
}
