package deployctl

import (
	"strings"
	"testing"
)

func TestInstallSummaryFullDockerListsSelectedEndpointsAndNextSteps(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "full", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "s3", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	summary := InstallSummary(plan, InstallResult{RuntimeEnvPath: "runtime/config/runtime.env", SetupToken: "setup-secret"}, true)
	for _, expected := range []string{
		"Setup", "http://127.0.0.1:8080/setup", "Gateway", "http://127.0.0.1/", "User Web", "http://127.0.0.1:5173/",
		"Admin Web", "http://127.0.0.1:5174/", "Documentation", "http://127.0.0.1:5175/", "API", "http://127.0.0.1:8080/",
		"PostgreSQL", "postgres:5432", "Redis", "redis:6379", "MinIO API", "minio:9000", "Docker network only",
		"Setup token: setup-secret", "1.", "2.", "deployctl status", "deployctl doctor", "deployment node IP",
	} {
		if !strings.Contains(summary, expected) {
			t.Errorf("summary missing %q:\n%s", expected, summary)
		}
	}
	if strings.Contains(summary, "Monitoring") {
		t.Fatalf("summary listed an unselected component:\n%s", summary)
	}
}

func TestInstallSummaryRedirectedOutputHidesTokenAndQuotesRecoveryPath(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "native", Profile: "core", Topology: "single", Role: "single", RuntimeDir: "runtime with space", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	summary := InstallSummary(plan, InstallResult{SetupToken: "must-not-leak"}, false)
	if strings.Contains(summary, "must-not-leak") {
		t.Fatalf("redirected summary leaked token: %s", summary)
	}
	if !strings.Contains(summary, `deployctl setup token show --runtime-dir "runtime with space"`) {
		t.Fatalf("summary missing exact recovery command: %s", summary)
	}
}

func TestInstallSummaryCustomPlanOmitsUnselectedServices(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "custom", Topology: "single", Role: "single", Components: []Component{ComponentAPI, ComponentWorker}, RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	summary := InstallSummary(plan, InstallResult{}, false)
	for _, absent := range []string{"Gateway", "User Web", "Admin Web", "Documentation", "PostgreSQL", "Redis", "MinIO"} {
		if strings.Contains(summary, absent) {
			t.Errorf("custom summary contains %q:\n%s", absent, summary)
		}
	}
}
