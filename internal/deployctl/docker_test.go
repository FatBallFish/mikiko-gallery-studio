package deployctl

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestDockerServicesMatchFullCoreAndCustomPlans(t *testing.T) {
	tests := []struct {
		name  string
		input InstallInput
		want  []string
	}{
		{
			name:  "full",
			input: InstallInput{Mode: "docker", Profile: "full", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "s3", ApplicationVersion: "v1"},
			want:  []string{"api", "worker", "user-web", "admin-web", "docs-web", "gateway", "postgres", "redis", "minio", "minio-init"},
		},
		{
			name:  "core",
			input: InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1"},
			want:  []string{"api", "worker", "user-web", "admin-web", "docs-web", "gateway"},
		},
		{
			name:  "custom",
			input: InstallInput{Mode: "docker", Profile: "custom", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1", Components: []Component{ComponentAPI, ComponentWorker}},
			want:  []string{"api", "worker"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := BuildInstallPlan(testCase.input)
			if err != nil {
				t.Fatal(err)
			}
			services, err := DockerServices(plan)
			if err != nil || !reflect.DeepEqual(services, testCase.want) {
				t.Fatalf("DockerServices = %v, %v; want %v", services, err, testCase.want)
			}
			profiles, err := DockerProfiles(plan)
			if err != nil {
				t.Fatal(err)
			}
			if testCase.name == "full" && !reflect.DeepEqual(profiles, []string{"full"}) {
				t.Fatalf("full profiles = %v", profiles)
			}
			if testCase.name == "core" && !reflect.DeepEqual(profiles, []string{"core"}) {
				t.Fatalf("core profiles = %v", profiles)
			}
			if testCase.name == "custom" && !reflect.DeepEqual(profiles, []string{"api", "worker"}) {
				t.Fatalf("custom profiles = %v", profiles)
			}
		})
	}
}

func TestBuildDockerProcessSpecsUsesPortableRuntimeWithoutSecrets(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	absoluteRuntime, err := filepath.Abs("runtime")
	if err != nil {
		t.Fatal(err)
	}
	const installationID = "019d0000-0000-7000-8000-000000000123"
	tests := []struct {
		action DockerAction
		verbs  []string
		count  int
	}{
		{action: DockerActionInstall, verbs: []string{"pull", "up"}, count: 2},
		{action: DockerActionUpdate, verbs: []string{"pull", "up"}, count: 2},
		{action: DockerActionRestart, verbs: []string{"restart"}, count: 1},
		{action: DockerActionStatus, verbs: []string{"ps"}, count: 1},
		{action: DockerActionUninstall, verbs: []string{"down"}, count: 1},
		{action: DockerActionDestroy, verbs: []string{"down"}, count: 1},
	}
	for _, testCase := range tests {
		t.Run(string(testCase.action), func(t *testing.T) {
			specs, err := BuildDockerProcessSpecs(testCase.action, plan, installationID, "1000:1000", []string{
				"PATH=/usr/bin", "HOME=/home/operator", "COMPOSE_PROJECT_NAME=wrong", "API_PORT=9999",
				"MONITORING_PORT=19999", "PROMETHEUS_PORT=29999",
			})
			if err != nil || len(specs) != testCase.count {
				t.Fatalf("BuildDockerProcessSpecs = %#v, %v", specs, err)
			}
			for index, spec := range specs {
				if spec.Executable != "docker" || spec.Directory != absoluteRuntime {
					t.Fatalf("process identity = %#v", spec)
				}
				joined := strings.Join(spec.Arguments, " ")
				for _, required := range []string{"compose", "--project-directory " + absoluteRuntime, "--env-file " + filepath.Join(absoluteRuntime, "config", "runtime.env"), "--file " + filepath.Join(absoluteRuntime, "compose.yml"), "--project-name app-019d0000000070008000000000000123", "--profile core", testCase.verbs[index]} {
					if !strings.Contains(joined, required) {
						t.Errorf("arguments %q missing %q", joined, required)
					}
				}
				if strings.Contains(joined, "SETUP_TOKEN") || strings.Contains(joined, "PASSWORD") {
					t.Fatalf("process arguments leaked runtime secrets: %q", joined)
				}
				if !slices.Contains(spec.Environment, "DEPLOYCTL_RUNTIME_DIR="+absoluteRuntime) || !slices.Contains(spec.Environment, "DEPLOYCTL_RUNTIME_USER=1000:1000") {
					t.Fatalf("process environment = %v", spec.Environment)
				}
				if !slices.Contains(spec.Environment, "PATH=/usr/bin") || !slices.Contains(spec.Environment, "HOME=/home/operator") || slices.Contains(spec.Environment, "COMPOSE_PROJECT_NAME=wrong") || slices.Contains(spec.Environment, "API_PORT=9999") || slices.Contains(spec.Environment, "MONITORING_PORT=19999") || slices.Contains(spec.Environment, "PROMETHEUS_PORT=29999") {
					t.Fatalf("process environment was not sanitized: %v", spec.Environment)
				}
			}
			if testCase.action == DockerActionUninstall {
				joined := strings.Join(specs[0].Arguments, " ")
				if strings.Contains(joined, "--volumes") || strings.Contains(joined, " -v") {
					t.Fatalf("ordinary uninstall deletes persistent data: %q", joined)
				}
			}
			if testCase.action == DockerActionDestroy && !strings.Contains(strings.Join(specs[0].Arguments, " "), "--volumes") {
				t.Fatal("destructive Docker removal did not request named-volume deletion")
			}
		})
	}
}

func TestBuildDockerProcessSpecsPreparesManagedServicesBeforeApplications(t *testing.T) {
	tests := []struct {
		name         string
		input        InstallInput
		wantCommands []string
	}{
		{
			name:  "full",
			input: InstallInput{Mode: "docker", Profile: "full", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "s3", ApplicationVersion: "v1"},
			wantCommands: []string{
				"pull",
				"up --detach --wait postgres redis minio",
				"run --rm --no-deps minio-init",
				"up --detach --wait --remove-orphans api worker user-web admin-web docs-web gateway",
			},
		},
		{
			name: "custom postgres and redis",
			input: InstallInput{
				Mode: "docker", Profile: "custom", Topology: "single", Role: "single", RuntimeDir: "runtime",
				StorageDriver: "local", ApplicationVersion: "v1", Components: []Component{ComponentAPI, ComponentPostgres, ComponentRedis},
			},
			wantCommands: []string{
				"pull",
				"up --detach --wait postgres redis",
				"up --detach --wait --remove-orphans api",
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := BuildInstallPlan(testCase.input)
			if err != nil {
				t.Fatal(err)
			}
			specs, err := BuildDockerProcessSpecs(DockerActionInstall, plan, "019d0000-0000-7000-8000-000000000123", "1000:1000", nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(specs) != len(testCase.wantCommands) {
				t.Fatalf("process count = %d, want %d: %#v", len(specs), len(testCase.wantCommands), specs)
			}
			for index, want := range testCase.wantCommands {
				arguments := strings.Join(specs[index].Arguments, " ")
				if !strings.HasSuffix(arguments, want) {
					t.Errorf("process %d arguments = %q, want suffix %q", index, arguments, want)
				}
			}
		})
	}
}

func TestDockerExecutorRunsInOrderAndStopsOnFailure(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle, RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingProcessRunner{failAt: 2}
	executor := DockerExecutor{
		Runner: runner, RuntimeUser: func() string { return "1000:1000" }, Environment: func() []string { return []string{"PATH=/usr/bin"} },
		ReadFile: func(string) ([]byte, error) {
			return []byte("INSTALLATION_ID=019d0000-0000-7000-8000-000000000123\n"), nil
		},
	}
	err = executor.Run(context.Background(), DockerActionInstall, plan)
	if err == nil || len(runner.specs) != 2 || !strings.Contains(err.Error(), "docker compose up") {
		t.Fatalf("executor failure = %v, calls %#v", err, runner.specs)
	}
}

func TestDockerProcessSpecDiagnosticsRedactInheritedEnvironment(t *testing.T) {
	spec := ProcessSpec{Executable: "docker", Arguments: []string{"compose", "ps"}, Environment: []string{"HOST_SECRET=do-not-print"}}
	serialized, err := json.Marshal(spec)
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{spec.String(), spec.GoString(), string(serialized)} {
		if strings.Contains(rendered, "do-not-print") {
			t.Fatalf("process diagnostic leaked inherited environment: %s", rendered)
		}
	}
}

func TestDockerExecutorRejectsNativePlansBeforeRunningAProcess(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "native", Profile: "core", Topology: "single", Role: "single", RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingProcessRunner{}
	if _, err := BuildDockerProcessSpecs(DockerActionInstall, plan, "019d0000-0000-7000-8000-000000000123", "1000:1000", nil); err == nil || len(runner.specs) != 0 {
		t.Fatalf("native Docker execution = %v, calls %v", err, runner.specs)
	}
}

func TestDockerExecutorPreflightChecksDaemonAndComposeWithoutRuntimeConfig(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "cluster", Role: "worker", RuntimeDir: "runtime", StorageDriver: "s3", ApplicationVersion: "v1", InstallationInitialized: true})
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingProcessRunner{}
	executor := DockerExecutor{
		Runner: runner,
		ReadFile: func(string) ([]byte, error) {
			t.Fatal("Docker preflight must not read unpublished runtime config")
			return nil, nil
		},
	}
	if err := executor.Preflight(t.Context(), plan); err != nil {
		t.Fatal(err)
	}
	if len(runner.specs) != 2 || strings.Join(runner.specs[0].Arguments, " ") != "version --format {{.Server.Version}}" || strings.Join(runner.specs[1].Arguments, " ") != "compose version" {
		t.Fatalf("Docker preflight specs = %#v", runner.specs)
	}
}

type recordingProcessRunner struct {
	specs  []ProcessSpec
	failAt int
}

func (runner *recordingProcessRunner) Run(_ context.Context, spec ProcessSpec) error {
	runner.specs = append(runner.specs, spec)
	if runner.failAt == len(runner.specs) {
		return errors.New("process failed")
	}
	return nil
}
