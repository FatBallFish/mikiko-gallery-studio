package handlers

import "testing"

func TestVideoReadinessFailureDoesNotBlockImageGeneration(t *testing.T) {
	status, summary := summarizeReadinessChecks([]adminReadinessCheck{
		{Key: "route_models", Status: "pass", Blocking: false},
		{Key: "video_routes", Status: "fail", Blocking: false},
	})
	if status != "warn" || summary["fail"] != 1 || summary["pass"] != 1 {
		t.Fatalf("status=%s summary=%#v", status, summary)
	}
	status, _ = summarizeReadinessChecks([]adminReadinessCheck{{Key: "route_models", Status: "fail", Blocking: true}})
	if status != "fail" {
		t.Fatalf("blocking image readiness status=%s", status)
	}
}
