package admincallrecord

import (
	"testing"
	"time"

	domainadmincallrecord "github.com/fatballfish/pic-gallery/internal/domain/admincallrecord"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
)

func TestCallDistributionCountsRealAttemptsAndReconcilesRoutes(t *testing.T) {
	from := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	to := from.Add(24 * time.Hour)
	insideA := from.Add(time.Hour)
	insideB := from.Add(2 * time.Hour)
	outside := from.Add(-time.Minute)
	service := NewServiceWithStore(NewMemoryStore(
		domainadmincallrecord.Record{
			TaskID: "route-a-task", RouteModelCode: "route-a", Status: domainimagetask.StatusSucceeded, CreatedAt: insideA,
			Attempts: []domainadmincallrecord.Attempt{{Status: domainimagetask.StatusFailed, StartedAt: &insideA}, {Status: domainimagetask.StatusSucceeded, StartedAt: &insideB}},
		},
		domainadmincallrecord.Record{
			TaskID: "unrouted-task", Status: domainimagetask.StatusFailed, CreatedAt: insideB,
			Attempts: []domainadmincallrecord.Attempt{{Status: domainimagetask.StatusFailed, StartedAt: &insideB}},
		},
		domainadmincallrecord.Record{
			TaskID: "preflight-task", Status: domainimagetask.StatusFailed, CreatedAt: insideB,
		},
		domainadmincallrecord.Record{
			TaskID: "outside-task", RouteModelCode: "route-b", Status: domainimagetask.StatusSucceeded, CreatedAt: outside,
			Attempts: []domainadmincallrecord.Attempt{{Status: domainimagetask.StatusSucceeded, StartedAt: &outside}},
		},
	))

	distribution, err := service.CallDistribution(t.Context(), domainadmincallrecord.DistributionRequest{From: from, To: to})
	if err != nil {
		t.Fatalf("CallDistribution: %v", err)
	}
	if distribution.TotalCalls != 3 || distribution.PreflightFailureCount != 1 {
		t.Fatalf("distribution totals = %#v", distribution)
	}
	want := map[string]int{"route-a": 2, "unrouted": 1}
	sum := 0
	for _, group := range distribution.Groups {
		sum += group.Calls
		if want[group.Key] != group.Calls {
			t.Fatalf("group %q calls=%d want=%d", group.Key, group.Calls, want[group.Key])
		}
	}
	if sum != distribution.TotalCalls || len(distribution.Groups) != len(want) {
		t.Fatalf("groups do not reconcile: %#v", distribution)
	}
	if distribution.Groups[0].Key != "route-a" || distribution.Groups[0].Percentage < 66.66 || distribution.Groups[0].Percentage > 66.67 {
		t.Fatalf("sorted percentage = %#v", distribution.Groups)
	}
}

func TestCallDistributionRequiresBoundedWindow(t *testing.T) {
	service := NewServiceWithStore(NewMemoryStore())
	if _, err := service.CallDistribution(t.Context(), domainadmincallrecord.DistributionRequest{}); err == nil {
		t.Fatal("missing distribution window must fail")
	}
	from := time.Now().UTC()
	if _, err := service.CallDistribution(t.Context(), domainadmincallrecord.DistributionRequest{From: from, To: from}); err == nil {
		t.Fatal("empty distribution window must fail")
	}
	if _, err := service.CallDistribution(t.Context(), domainadmincallrecord.DistributionRequest{From: from, To: from.Add(32 * 24 * time.Hour)}); err == nil {
		t.Fatal("distribution window over 31 days must fail")
	}
}

func TestCallDistributionUsesHalfOpenAttemptWindowAndExcludesArtifactFailureFromPreflight(t *testing.T) {
	from := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	atFrom, atTo := from, to
	upstreamSucceeded := from.Add(30 * time.Minute)
	service := NewServiceWithStore(NewMemoryStore(
		domainadmincallrecord.Record{TaskID: "lower-bound", RouteModelCode: "route", Status: domainimagetask.StatusSucceeded, CreatedAt: from, Attempts: []domainadmincallrecord.Attempt{{StartedAt: &atFrom}}},
		domainadmincallrecord.Record{TaskID: "upper-bound", RouteModelCode: "route", Status: domainimagetask.StatusSucceeded, CreatedAt: to, Attempts: []domainadmincallrecord.Attempt{{StartedAt: &atTo}}},
		domainadmincallrecord.Record{TaskID: "artifact", Status: domainimagetask.StatusFailed, CreatedAt: from.Add(time.Minute), UpstreamSucceededAt: &upstreamSucceeded},
	))
	distribution, err := service.CallDistribution(t.Context(), domainadmincallrecord.DistributionRequest{From: from, To: to})
	if err != nil {
		t.Fatal(err)
	}
	if distribution.TotalCalls != 1 || len(distribution.Groups) != 1 || distribution.Groups[0].Calls != 1 || distribution.PreflightFailureCount != 0 {
		t.Fatalf("half-open distribution = %#v", distribution)
	}
}
