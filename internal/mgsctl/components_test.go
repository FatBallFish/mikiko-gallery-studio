package mgsctl

import (
	"reflect"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestBuildInstallPlanExpandsApprovedPresets(t *testing.T) {
	tests := []struct {
		name       string
		input      InstallInput
		components []Component
	}{
		{
			name: "docker full",
			input: InstallInput{Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileFull, Topology: config.DeploymentTopologySingle,
				Role: config.DeploymentRoleSingle, RuntimeDir: ".", StorageDriver: "s3", ApplicationVersion: "v1.0.0"},
			components: []Component{ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb, ComponentGateway, ComponentPostgres, ComponentRedis, ComponentMinIO},
		},
		{
			name: "docker core single",
			input: InstallInput{Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
				Role: config.DeploymentRoleSingle, RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1.0.0"},
			components: []Component{ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb, ComponentGateway},
		},
		{
			name: "native core",
			input: InstallInput{Mode: config.DeploymentModeNative, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
				Role: config.DeploymentRoleSingle, RuntimeDir: "runtime", StorageDriver: "local", ApplicationVersion: "v1.0.0"},
			components: []Component{ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb, ComponentGateway},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			plan, err := BuildInstallPlan(testCase.input)
			if err != nil {
				t.Fatalf("BuildInstallPlan: %v", err)
			}
			if !reflect.DeepEqual(plan.Components, testCase.components) {
				t.Fatalf("components = %v, want %v", plan.Components, testCase.components)
			}
			if plan.RuntimeDir != testCase.input.RuntimeDir || plan.Mode != testCase.input.Mode || plan.Role != testCase.input.Role {
				t.Fatalf("plan lost deployment identity: %#v", plan)
			}
			wantProbe := "http://gateway/developer-docs/"
			if plan.Mode == config.DeploymentModeNative {
				wantProbe = "http://127.0.0.1:80/developer-docs/"
			}
			if plan.DocsProbeURL != wantProbe {
				t.Fatalf("documentation probe URL = %q, want %q", plan.DocsProbeURL, wantProbe)
			}
		})
	}
}

func TestBuildInstallPlanRejectsUnsafeDeploymentCombinations(t *testing.T) {
	base := InstallInput{Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
		Role: config.DeploymentRoleSingle, RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1.0.0"}
	tests := []struct {
		name   string
		mutate func(*InstallInput)
	}{
		{name: "native full", mutate: func(input *InstallInput) {
			input.Mode = config.DeploymentModeNative
			input.Profile = config.DeploymentProfileFull
		}},
		{name: "cluster local storage", mutate: func(input *InstallInput) {
			input.Topology = config.DeploymentTopologyCluster
			input.Role = config.DeploymentRoleControl
		}},
		{name: "uninitialized joined worker", mutate: func(input *InstallInput) {
			input.Topology = config.DeploymentTopologyCluster
			input.Role = config.DeploymentRoleWorker
			input.StorageDriver = "s3"
			input.InstallationInitialized = false
		}},
		{name: "web without public api", mutate: func(input *InstallInput) {
			input.Topology = config.DeploymentTopologyCluster
			input.Role = config.DeploymentRoleWeb
			input.StorageDriver = ""
			input.InstallationInitialized = true
		}},
		{name: "web without gateway confirmation", mutate: func(input *InstallInput) {
			input.Profile = config.DeploymentProfileCustom
			input.Components = []Component{ComponentUserWeb}
			input.ExternalGatewayConfirmed = false
		}},
		{name: "native middleware", mutate: func(input *InstallInput) {
			input.Mode = config.DeploymentModeNative
			input.Profile = config.DeploymentProfileCustom
			input.Components = []Component{ComponentAPI, ComponentPostgres}
		}},
		{name: "migration by worker", mutate: func(input *InstallInput) {
			input.Topology = config.DeploymentTopologyCluster
			input.Role = config.DeploymentRoleWorker
			input.StorageDriver = "s3"
			input.InstallationInitialized = true
			input.MigrationRequested = true
		}},
		{name: "public api credentials", mutate: func(input *InstallInput) { input.PublicAPIURL = "https://user:secret@api.example.test" }},
		{name: "docs probe credentials", mutate: func(input *InstallInput) { input.DocsProbeURL = "https://user:secret@docs.example.test" }},
		{name: "docs probe relative", mutate: func(input *InstallInput) { input.DocsProbeURL = "/developer-docs/" }},
		{name: "docs URL encoded traversal", mutate: func(input *InstallInput) { input.DocsURL = "https://docs.example.test/developer-docs/%2e%2e/healthz" }},
		{name: "full local storage", mutate: func(input *InstallInput) {
			input.Profile = config.DeploymentProfileFull
			input.StorageDriver = "local"
		}},
		{name: "invalid public api port", mutate: func(input *InstallInput) { input.PublicAPIURL = "http://api.example.test:99999" }},
		{name: "public api empty query marker", mutate: func(input *InstallInput) { input.PublicAPIURL = "http://api.example.test/?" }},
		{name: "cluster managed middleware", mutate: func(input *InstallInput) {
			input.Profile = config.DeploymentProfileCustom
			input.Topology = config.DeploymentTopologyCluster
			input.Role = config.DeploymentRoleControl
			input.StorageDriver = "s3"
			input.Components = []Component{ComponentAPI, ComponentPostgres}
		}},
		{name: "MinIO with local storage", mutate: func(input *InstallInput) {
			input.Profile = config.DeploymentProfileCustom
			input.Components = []Component{ComponentAPI, ComponentMinIO}
			input.StorageDriver = "local"
		}},
		{name: "gateway without local web suite", mutate: func(input *InstallInput) {
			input.Profile = config.DeploymentProfileCustom
			input.Components = []Component{ComponentAPI, ComponentGateway}
		}},
		{name: "web role gateway with partial local web suite", mutate: func(input *InstallInput) {
			input.Profile = config.DeploymentProfileCustom
			input.Topology = config.DeploymentTopologyCluster
			input.Role = config.DeploymentRoleWeb
			input.StorageDriver = ""
			input.InstallationInitialized = true
			input.PublicAPIURL = "https://api.example.test"
			input.Components = []Component{ComponentUserWeb, ComponentGateway}
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			input := base
			testCase.mutate(&input)
			if _, err := BuildInstallPlan(input); err == nil {
				t.Fatalf("unsafe input accepted: %#v", input)
			}
		})
	}
}

func TestBuildInstallPlanResolvesDocumentationTargetsAtomically(t *testing.T) {
	base := InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
		Role: config.DeploymentRoleSingle, RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1",
	}

	relative, err := BuildInstallPlan(base)
	if err != nil {
		t.Fatal(err)
	}
	if relative.DocsURL != "/developer-docs/" || relative.DocsProbeURL != "http://gateway/developer-docs/" {
		t.Fatalf("relative docs plan = user %q probe %q", relative.DocsURL, relative.DocsProbeURL)
	}

	absoluteInput := base
	absoluteInput.DocsURL = "https://docs.example.test/reference/"
	absolute, err := BuildInstallPlan(absoluteInput)
	if err != nil {
		t.Fatal(err)
	}
	if absolute.DocsURL != absoluteInput.DocsURL || absolute.DocsProbeURL != "" {
		t.Fatalf("absolute docs plan = user %q probe %q; internal probe must not override it", absolute.DocsURL, absolute.DocsProbeURL)
	}

	mismatch := base
	mismatch.DocsURL = "/developer-docs/"
	mismatch.DocsProbeURL = "https://gateway.example.test/not-the-docs/"
	if _, err := BuildInstallPlan(mismatch); err == nil || !strings.Contains(err.Error(), "path") {
		t.Fatalf("mismatched documentation paths error = %v", err)
	}

	absoluteWithProbe := absoluteInput
	absoluteWithProbe.DocsProbeURL = "https://gateway.example.test/reference/"
	if _, err := BuildInstallPlan(absoluteWithProbe); err == nil {
		t.Fatal("absolute documentation URL must reject an unused explicit probe")
	}
}

func TestBuildInstallPlanAcceptsExplicitSafeCustomAndJoinedRolePlans(t *testing.T) {
	custom, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCustom, Topology: config.DeploymentTopologySingle,
		Role: config.DeploymentRoleSingle, RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1",
		Components: []Component{ComponentAPI, ComponentWorker}, ExternalGatewayConfirmed: true,
	})
	if err != nil || !reflect.DeepEqual(custom.Components, []Component{ComponentAPI, ComponentWorker}) {
		t.Fatalf("safe custom plan = %#v, %v", custom, err)
	}
	managedCustom, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCustom, Topology: config.DeploymentTopologySingle,
		Role: config.DeploymentRoleSingle, RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1",
		Components: []Component{ComponentAPI, ComponentPostgres, ComponentRedis},
	})
	if err != nil || !reflect.DeepEqual(managedCustom.Components, []Component{ComponentAPI, ComponentPostgres, ComponentRedis}) {
		t.Fatalf("Docker custom managed middleware plan = %#v, %v", managedCustom, err)
	}

	worker, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeNative, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologyCluster,
		Role: config.DeploymentRoleWorker, RuntimeDir: ".", StorageDriver: "s3", ApplicationVersion: "v1", InstallationInitialized: true,
	})
	if err != nil || !reflect.DeepEqual(worker.Components, []Component{ComponentWorker}) || !worker.RequiresEnrollment {
		t.Fatalf("joined worker plan = %#v, %v", worker, err)
	}

	api, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologyCluster,
		Role: config.DeploymentRoleAPI, RuntimeDir: ".", StorageDriver: "s3", ApplicationVersion: "v1", InstallationInitialized: true,
	})
	if err != nil || !reflect.DeepEqual(api.Components, []Component{ComponentAPI}) || !api.RequiresEnrollment {
		t.Fatalf("joined API plan = %#v, %v", api, err)
	}

	web, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeNative, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologyCluster,
		Role: config.DeploymentRoleWeb, RuntimeDir: ".", PublicAPIURL: "https://api.example.test", ApplicationVersion: "v1", InstallationInitialized: true,
	})
	if err != nil || !reflect.DeepEqual(web.Components, []Component{ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb, ComponentGateway}) || !web.RequiresEnrollment {
		t.Fatalf("joined Web plan = %#v, %v", web, err)
	}
}

func TestBuildInstallPlanRejectsInvalidPortsAndComponentNames(t *testing.T) {
	base := InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1"}
	for _, port := range []string{"0", "65536", "not-a-port"} {
		input := base
		input.APIPort = port
		if _, err := BuildInstallPlan(input); err == nil {
			t.Errorf("invalid API port %q was accepted", port)
		}
	}
	for _, components := range [][]Component{{ComponentAPI, ComponentAPI}, {ComponentAPI, "unknown"}} {
		input := base
		input.Profile = config.DeploymentProfileCustom
		input.Components = components
		if _, err := BuildInstallPlan(input); err == nil {
			t.Errorf("invalid component selection %v was accepted", components)
		}
	}
}

func TestValidateInstallPlanRejectsMutatedOrIncompletePlans(t *testing.T) {
	valid, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "core", Topology: "single", Role: "single", RuntimeDir: ".", StorageDriver: "local", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	mutations := []func(*InstallPlan){
		func(plan *InstallPlan) { plan.Components = append(plan.Components, ComponentPostgres) },
		func(plan *InstallPlan) { plan.RequiresEnrollment = true },
		func(plan *InstallPlan) { plan.APIPort = "70000" },
		func(plan *InstallPlan) { plan.ApplicationVersion = "" },
		func(plan *InstallPlan) { plan.RuntimeDir = "" },
	}
	for _, mutate := range mutations {
		plan := valid
		plan.Components = append([]Component(nil), valid.Components...)
		mutate(&plan)
		if err := ValidateInstallPlan(plan); err == nil {
			t.Errorf("mutated plan was accepted: %#v", plan)
		}
	}
}

func TestValidateInstallPlanRechecksFullStorageInvariant(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{Mode: "docker", Profile: "full", Topology: "single", Role: "single", RuntimeDir: ".", StorageDriver: "s3", ApplicationVersion: "v1"})
	if err != nil {
		t.Fatal(err)
	}
	plan.StorageDriver = "local"
	if err := ValidateInstallPlan(plan); err == nil {
		t.Fatal("direct full-local plan was accepted")
	}
}
