package deployctl

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

const nativeTestInstallationID = "019d0000-0000-7000-8000-000000000123"

func TestNativeServicePlanUsesPortableRuntimeAndDependencyOrder(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: config.DeploymentModeNative, Profile: config.DeploymentProfileCore, Topology: config.DeploymentTopologySingle,
		Role: config.DeploymentRoleSingle, RuntimeDir: filepath.Join(t.TempDir(), "runtime with spaces"), StorageDriver: "local", ApplicationVersion: "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	services, err := BuildNativeServicePlan(plan, nativeTestInstallationID, NativePlatformLinux)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 3 {
		t.Fatalf("services = %#v", services)
	}
	installationName := "app-019d0000000070008000000000000123"
	wantComponents := []Component{ComponentAPI, ComponentWorker, ComponentGateway}
	for index, service := range services {
		if service.Component != wantComponents[index] || service.Name != installationName+"-"+string(service.Component) {
			t.Fatalf("service %d identity = %#v", index, service)
		}
		if service.WorkingDirectory != plan.RuntimeDir || !filepath.IsAbs(service.Executable) || service.RestartExitCode != 75 {
			t.Fatalf("service %d runtime contract = %#v", index, service)
		}
		if strings.Contains(strings.ToUpper(service.Executable), "PIC_GALLERY_ENV_FILE") || len(service.Environment) != 0 {
			t.Fatalf("service %d uses a retired or redundant env selector: %#v", index, service)
		}
	}
	if services[0].Executable != filepath.Join(plan.RuntimeDir, "bin", "pic-gallery-api") || len(services[0].Dependencies) != 0 {
		t.Fatalf("API service = %#v", services[0])
	}
	if !reflect.DeepEqual(services[1].Dependencies, []string{services[0].Name}) || !reflect.DeepEqual(services[2].Dependencies, []string{services[0].Name}) {
		t.Fatalf("service dependencies = %#v", services)
	}
}

func TestNativeReleaseInstallerVerifiesAndExtractsPortableBundle(t *testing.T) {
	archive := nativeReleaseArchiveForTest(t, map[string]nativeArchiveEntry{
		"bin/":                         {mode: 0o755, typeflag: tar.TypeDir},
		"bin/pic-gallery-api":          {content: "api", mode: 0o755},
		"bin/pic-gallery-worker":       {content: "worker", mode: 0o755},
		"bin/pic-gallery-gateway":      {content: "gateway", mode: 0o755},
		"web/user/index.html":          {content: "user", mode: 0o644},
		"web/admin/index.html":         {content: "admin", mode: 0o644},
		"web/docs/index.html":          {content: "docs", mode: 0o644},
		"api/openapi/openapi.yaml":     {content: "openapi", mode: 0o644},
		"api/openapi/components/x.yml": {content: "component", mode: 0o644},
	})
	digest := sha256.Sum256(archive)
	requests := make([]string, 0)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests = append(requests, request.URL.Path)
		if strings.HasSuffix(request.URL.Path, ".sha256") {
			_, _ = fmt.Fprintf(writer, "%x  pic-gallery-native-linux-amd64.tar.gz\n", digest)
			return
		}
		_, _ = writer.Write(archive)
	}))
	defer server.Close()
	plan := nativeCorePlanForTest(t)
	installer := NativeReleaseInstaller{Client: server.Client(), BaseURL: server.URL, Architecture: "amd64"}
	if err := installer.Install(context.Background(), plan, NativePlatformLinux); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		filepath.Join("bin", "pic-gallery-api"): "api", filepath.Join("bin", "pic-gallery-worker"): "worker",
		filepath.Join("bin", "pic-gallery-gateway"): "gateway", filepath.Join("web", "user", "index.html"): "user",
		filepath.Join("api", "openapi", "openapi.yaml"): "openapi",
	} {
		content, err := os.ReadFile(filepath.Join(plan.RuntimeDir, path))
		if err != nil || string(content) != want {
			t.Fatalf("release file %s = %q, %v", path, content, err)
		}
	}
	info, err := os.Stat(filepath.Join(plan.RuntimeDir, "bin", "pic-gallery-api"))
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("API executable mode = %v, %v", info.Mode(), err)
	}
	if len(requests) != 2 || requests[0] != "/download/v1/pic-gallery-native-linux-amd64.tar.gz.sha256" || requests[1] != "/download/v1/pic-gallery-native-linux-amd64.tar.gz" {
		t.Fatalf("release requests = %#v", requests)
	}
	if err := installer.Install(context.Background(), plan, NativePlatformLinux); err != nil {
		t.Fatalf("idempotent release install: %v", err)
	}
	apiPath := filepath.Join(plan.RuntimeDir, "bin", "pic-gallery-api")
	if err := os.Chmod(apiPath, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := installer.Install(context.Background(), plan, NativePlatformLinux); err == nil || !strings.Contains(err.Error(), "not executable") {
		t.Fatalf("release marker accepted a non-executable API binary: %v", err)
	}
	if err := os.Chmod(apiPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(apiPath); err != nil {
		t.Fatal(err)
	}
	if err := installer.Install(context.Background(), plan, NativePlatformLinux); err == nil || !strings.Contains(err.Error(), "missing required file") {
		t.Fatalf("release marker hid a missing installed file: %v", err)
	}
}

func TestNativeReleaseInstallerRejectsChecksumAndTraversalWithoutPublishing(t *testing.T) {
	tests := []struct {
		name     string
		entries  map[string]nativeArchiveEntry
		checksum string
	}{
		{name: "checksum", entries: map[string]nativeArchiveEntry{"bin/pic-gallery-api": {content: "api", mode: 0o755}}, checksum: strings.Repeat("0", 64)},
		{name: "traversal", entries: map[string]nativeArchiveEntry{"../outside.txt": {content: "outside", mode: 0o644}}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			archive := nativeReleaseArchiveForTest(t, testCase.entries)
			digest := sha256.Sum256(archive)
			checksum := testCase.checksum
			if checksum == "" {
				checksum = fmt.Sprintf("%x", digest)
			}
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if strings.HasSuffix(request.URL.Path, ".sha256") {
					_, _ = fmt.Fprintln(writer, checksum)
					return
				}
				_, _ = writer.Write(archive)
			}))
			defer server.Close()
			plan := nativeCorePlanForTest(t)
			installer := NativeReleaseInstaller{Client: server.Client(), BaseURL: server.URL, Architecture: "amd64"}
			if err := installer.Install(context.Background(), plan, NativePlatformLinux); err == nil {
				t.Fatal("unsafe native release was accepted")
			}
			if _, err := os.Stat(filepath.Join(plan.RuntimeDir, "bin")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("unsafe release published bin directory: %v", err)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(plan.RuntimeDir), "outside.txt")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("release traversal wrote outside runtime: %v", err)
			}
		})
	}
}

func TestNativeReleaseInstallerRecoversOnlyAnUnmodifiedJournaledPartialPublish(t *testing.T) {
	archive := nativeReleaseArchiveForTest(t, map[string]nativeArchiveEntry{
		"bin/pic-gallery-api":      {content: "api", mode: 0o755},
		"bin/pic-gallery-worker":   {content: "worker", mode: 0o755},
		"bin/pic-gallery-gateway":  {content: "gateway", mode: 0o755},
		"web/user/index.html":      {content: "user", mode: 0o644},
		"web/admin/index.html":     {content: "admin", mode: 0o644},
		"web/docs/index.html":      {content: "docs", mode: 0o644},
		"api/openapi/openapi.yaml": {content: "openapi", mode: 0o644},
	})
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, ".sha256") {
			_, _ = fmt.Fprintf(writer, "%x\n", digest)
			return
		}
		_, _ = writer.Write(archive)
	}))
	defer server.Close()

	for _, testCase := range []struct {
		name   string
		modify bool
	}{
		{name: "recover exact partial publish"},
		{name: "reject modified partial publish", modify: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			plan := nativeCorePlanForTest(t)
			renames := 0
			installer := NativeReleaseInstaller{
				Client: server.Client(), BaseURL: server.URL, Architecture: "amd64",
				Rename: func(source, target string) error {
					renames++
					if renames == 2 {
						return errors.New("simulated process interruption")
					}
					return os.Rename(source, target)
				},
			}
			if err := installer.Install(context.Background(), plan, NativePlatformLinux); err == nil || !strings.Contains(err.Error(), "simulated process interruption") {
				t.Fatalf("interrupted install error = %v", err)
			}
			pendingPath := filepath.Join(plan.RuntimeDir, ".native-release.pending.json")
			if _, err := os.Stat(pendingPath); err != nil {
				t.Fatalf("interrupted install did not retain its recovery journal: %v", err)
			}
			apiPath := filepath.Join(plan.RuntimeDir, "bin", "pic-gallery-api")
			if testCase.modify {
				if err := os.WriteFile(apiPath, []byte("operator change"), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			installer.Rename = nil
			err := installer.Install(context.Background(), plan, NativePlatformLinux)
			if testCase.modify {
				if err == nil || !strings.Contains(err.Error(), "pending release journal") {
					t.Fatalf("modified partial publish error = %v", err)
				}
				content, readErr := os.ReadFile(apiPath)
				if readErr != nil || string(content) != "operator change" {
					t.Fatalf("modified partial publish was changed: %q, %v", content, readErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("recover exact partial publish: %v", err)
			}
			if _, err := os.Stat(pendingPath); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("successful recovery retained pending journal: %v", err)
			}
		})
	}
}

func TestNativeReleaseInstallerUpdatesAndRecoversAnInterruptedDirectorySwap(t *testing.T) {
	archive := func(version string) []byte {
		return nativeReleaseArchiveForTest(t, map[string]nativeArchiveEntry{
			"bin/pic-gallery-api":      {content: "api-" + version, mode: 0o755},
			"bin/pic-gallery-worker":   {content: "worker-" + version, mode: 0o755},
			"bin/pic-gallery-gateway":  {content: "gateway-" + version, mode: 0o755},
			"web/user/index.html":      {content: "user-unchanged", mode: 0o644},
			"web/admin/index.html":     {content: "admin-" + version, mode: 0o644},
			"web/docs/index.html":      {content: "docs-" + version, mode: 0o644},
			"api/openapi/openapi.yaml": {content: "openapi-unchanged", mode: 0o644},
		})
	}
	archives := map[string][]byte{"v1": archive("v1"), "v2": archive("v2")}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		version := "v1"
		if strings.Contains(request.URL.Path, "/v2/") {
			version = "v2"
		}
		content := archives[version]
		if strings.HasSuffix(request.URL.Path, ".sha256") {
			digest := sha256.Sum256(content)
			_, _ = fmt.Fprintf(writer, "%x\n", digest)
			return
		}
		_, _ = writer.Write(content)
	}))
	defer server.Close()

	plan := nativeCorePlanForTest(t)
	installer := NativeReleaseInstaller{Client: server.Client(), BaseURL: server.URL, Architecture: "amd64"}
	if err := installer.Install(context.Background(), plan, NativePlatformLinux); err != nil {
		t.Fatal(err)
	}
	plan.ReleaseVersion = "v2"
	renames := 0
	installer.Rename = func(source, target string) error {
		renames++
		if renames == 4 {
			return errors.New("simulated update interruption")
		}
		return os.Rename(source, target)
	}
	if err := installer.Install(context.Background(), plan, NativePlatformLinux); err == nil || !strings.Contains(err.Error(), "simulated update interruption") {
		t.Fatalf("interrupted update error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(plan.RuntimeDir, ".native-release.pending.json")); err != nil {
		t.Fatalf("interrupted update did not retain its journal: %v", err)
	}
	installer.Rename = nil
	if err := installer.Install(context.Background(), plan, NativePlatformLinux); err != nil {
		t.Fatalf("recover interrupted update: %v", err)
	}
	for relativePath, want := range map[string]string{
		filepath.Join("bin", "pic-gallery-api"):         "api-v2",
		filepath.Join("web", "user", "index.html"):      "user-unchanged",
		filepath.Join("api", "openapi", "openapi.yaml"): "openapi-unchanged",
	} {
		content, err := os.ReadFile(filepath.Join(plan.RuntimeDir, relativePath))
		if err != nil || string(content) != want {
			t.Fatalf("updated file %s = %q, %v", relativePath, content, err)
		}
	}
	for _, path := range []string{
		".native-release.pending.json", ".native-release-backup-bin", ".native-release-backup-web", ".native-release-backup-api",
	} {
		if _, err := os.Stat(filepath.Join(plan.RuntimeDir, path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("successful update retained %s: %v", path, err)
		}
	}
}

func TestNativeReleaseInstallerCleansOnlyOwnedStaleStageDirectories(t *testing.T) {
	archive := nativeReleaseArchiveForTest(t, map[string]nativeArchiveEntry{
		"bin/pic-gallery-api":      {content: "api", mode: 0o755},
		"bin/pic-gallery-worker":   {content: "worker", mode: 0o755},
		"bin/pic-gallery-gateway":  {content: "gateway", mode: 0o755},
		"web/user/index.html":      {content: "user", mode: 0o644},
		"web/admin/index.html":     {content: "admin", mode: 0o644},
		"web/docs/index.html":      {content: "docs", mode: 0o644},
		"api/openapi/openapi.yaml": {content: "openapi", mode: 0o644},
	})
	digest := sha256.Sum256(archive)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, ".sha256") {
			_, _ = fmt.Fprintf(writer, "%x\n", digest)
			return
		}
		_, _ = writer.Write(archive)
	}))
	defer server.Close()
	plan := nativeCorePlanForTest(t)
	stale := filepath.Join(plan.RuntimeDir, ".native-release-stage-interrupted")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "large-partial-file"), []byte("partial"), 0o600); err != nil {
		t.Fatal(err)
	}
	unrelated := filepath.Join(plan.RuntimeDir, ".native-release-other")
	if err := os.MkdirAll(unrelated, 0o700); err != nil {
		t.Fatal(err)
	}
	installer := NativeReleaseInstaller{Client: server.Client(), BaseURL: server.URL, Architecture: "amd64"}
	if err := installer.Install(context.Background(), plan, NativePlatformLinux); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale native release stage was not removed: %v", err)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatalf("unrelated runtime directory was removed: %v", err)
	}
}

func TestNativeExecutorInstallsReleaseFilesThenServices(t *testing.T) {
	plan := nativeCorePlanForTest(t)
	if err := os.MkdirAll(filepath.Join(plan.RuntimeDir, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plan.RuntimeDir, "config", "runtime.env"), []byte("INSTALLATION_ID="+nativeTestInstallationID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	events := make([]string, 0)
	runner := processRunnerFunc(func(_ context.Context, spec ProcessSpec) error {
		events = append(events, "process:"+spec.Executable+":"+spec.Arguments[0])
		return nil
	})
	executor := NativeExecutor{
		Runner: runner, Platform: func() NativePlatform { return NativePlatformLinux },
		CheckPrivileges: func(NativePlatform) error { events = append(events, "privileges"); return nil },
		InstallRelease: func(context.Context, InstallPlan, NativePlatform) error {
			events = append(events, "release")
			return nil
		},
		WriteServiceFile: func(path string, _ []byte) error {
			events = append(events, "file:"+filepath.Base(path))
			return nil
		},
	}
	if err := executor.Run(context.Background(), NativeActionInstall, plan); err != nil {
		t.Fatal(err)
	}
	if len(events) < 7 || !reflect.DeepEqual(events[:5], []string{
		"privileges", "release",
		"file:app-019d0000000070008000000000000123-api.service",
		"file:app-019d0000000070008000000000000123-worker.service",
		"file:app-019d0000000070008000000000000123-gateway.service",
	}) {
		t.Fatalf("native install event order = %#v", events)
	}
	if events[5] != "process:systemctl:link" {
		t.Fatalf("service processes started before preparation: %#v", events)
	}
}

func TestNativeExecutorStopsBeforeServiceMutationOnPrivilegeOrReleaseFailure(t *testing.T) {
	plan := nativeCorePlanForTest(t)
	if err := os.MkdirAll(filepath.Join(plan.RuntimeDir, "config"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plan.RuntimeDir, "config", "runtime.env"), []byte("INSTALLATION_ID="+nativeTestInstallationID+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name       string
		privileges error
		release    error
	}{
		{name: "privileges", privileges: errors.New("administrator required")},
		{name: "release", release: errors.New("checksum mismatch")},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			mutations := 0
			executor := NativeExecutor{
				Runner:           processRunnerFunc(func(context.Context, ProcessSpec) error { mutations++; return nil }),
				Platform:         func() NativePlatform { return NativePlatformLinux },
				CheckPrivileges:  func(NativePlatform) error { return testCase.privileges },
				InstallRelease:   func(context.Context, InstallPlan, NativePlatform) error { return testCase.release },
				WriteServiceFile: func(string, []byte) error { mutations++; return nil },
			}
			if err := executor.Run(context.Background(), NativeActionInstall, plan); err == nil {
				t.Fatal("native install accepted expected failure")
			}
			if mutations != 0 {
				t.Fatalf("failure performed %d service mutations", mutations)
			}
		})
	}
}

func TestNativeWebServicePlanUsesExternalAPIWithoutLocalDependency(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: "native", Profile: "core", Topology: "cluster", Role: "web", RuntimeDir: t.TempDir(),
		PublicAPIURL: "https://api.example.test", ApplicationVersion: "v1", InstallationInitialized: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	services, err := BuildNativeServicePlan(plan, nativeTestInstallationID, NativePlatformWindows)
	if err != nil {
		t.Fatal(err)
	}
	if len(services) != 1 || services[0].Component != ComponentGateway || len(services[0].Dependencies) != 0 || !strings.HasSuffix(services[0].Executable, "pic-gallery-gateway.exe") {
		t.Fatalf("web services = %#v", services)
	}
}

func TestNativeAssetOnlyWebPlanInstallsReleaseWithoutCreatingServices(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: "native", Profile: "custom", Topology: "cluster", Role: "web", RuntimeDir: t.TempDir(),
		Components: []Component{ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb}, ExternalGatewayConfirmed: true,
		PublicAPIURL: "https://api.example.test", ApplicationVersion: "v1", InstallationInitialized: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	services, err := BuildNativeServicePlan(plan, nativeTestInstallationID, NativePlatformLinux)
	if err != nil || len(services) != 0 {
		t.Fatalf("asset-only services = %#v, %v", services, err)
	}
	runtimeEnv := []byte("INSTALLATION_ID=" + nativeTestInstallationID + "\n")
	releaseCalls := 0
	executor := NativeExecutor{
		Runner: processRunnerFunc(func(context.Context, ProcessSpec) error {
			t.Fatal("asset-only plan attempted a service process")
			return nil
		}),
		ReadFile: func(string) ([]byte, error) { return runtimeEnv, nil },
		Platform: func() NativePlatform { return NativePlatformLinux }, CheckPrivileges: func(NativePlatform) error { return nil },
		InstallRelease: func(context.Context, InstallPlan, NativePlatform) error { releaseCalls++; return nil },
		WriteServiceFile: func(string, []byte) error {
			t.Fatal("asset-only plan wrote a service file")
			return nil
		},
	}
	if err := executor.Run(context.Background(), NativeActionInstall, plan); err != nil {
		t.Fatal(err)
	}
	if releaseCalls != 1 {
		t.Fatalf("asset-only release calls = %d", releaseCalls)
	}

	for _, frontend := range []string{"user", "admin", "docs"} {
		path := filepath.Join(plan.RuntimeDir, "web", frontend, "index.html")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(frontend), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := validateNativeReleaseFiles(plan.RuntimeDir, plan, NativePlatformLinux); err != nil {
		t.Fatalf("validate asset-only release: %v", err)
	}
	if err := os.Remove(filepath.Join(plan.RuntimeDir, "web", "docs", "index.html")); err != nil {
		t.Fatal(err)
	}
	if err := validateNativeReleaseFiles(plan.RuntimeDir, plan, NativePlatformLinux); err == nil {
		t.Fatal("asset-only release accepted a missing documentation frontend")
	}
}

func TestNativeSystemdUnitsSetWorkingDirectoryDependenciesAndRestartContract(t *testing.T) {
	plan, err := BuildInstallPlan(InstallInput{
		Mode: "native", Profile: "core", Topology: "single", Role: "single", RuntimeDir: filepath.Join(t.TempDir(), "runtime with spaces"),
		StorageDriver: "local", ApplicationVersion: "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	files, err := BuildNativeServiceFiles(plan, nativeTestInstallationID, NativePlatformLinux)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 {
		t.Fatalf("service files = %#v", files)
	}
	apiName := "app-019d0000000070008000000000000123-api.service"
	for _, file := range files {
		content := string(file.Content)
		for _, required := range []string{"WorkingDirectory=", "ExecStart=", "Restart=on-failure", "RestartForceExitStatus=75", "UMask=0077", "After=network-online.target"} {
			if !strings.Contains(content, required) {
				t.Errorf("%s missing %q:\n%s", file.RelativePath, required, content)
			}
		}
		for _, forbidden := range []string{"PIC_GALLERY_ENV_FILE", "APP_ENV_FILE", "EnvironmentFile="} {
			if strings.Contains(content, forbidden) {
				t.Errorf("%s contains forbidden selector %q", file.RelativePath, forbidden)
			}
		}
		if !strings.Contains(file.RelativePath, "services") || !strings.HasSuffix(file.RelativePath, ".service") {
			t.Errorf("service file path = %q", file.RelativePath)
		}
		if !strings.Contains(file.RelativePath, "-api.service") {
			for _, dependency := range []string{"After=" + apiName, "Wants=" + apiName} {
				if !strings.Contains(content, dependency) {
					t.Errorf("%s missing dependency %q", file.RelativePath, dependency)
				}
			}
		}
	}
}

func TestNativeLinuxProcessSpecsInstallAndStartServicesInOrder(t *testing.T) {
	plan := nativeCorePlanForTest(t)
	services, err := BuildNativeServicePlan(plan, nativeTestInstallationID, NativePlatformLinux)
	if err != nil {
		t.Fatal(err)
	}
	specs, err := BuildNativeProcessSpecs(NativeActionInstall, plan, nativeTestInstallationID, NativePlatformLinux)
	if err != nil {
		t.Fatal(err)
	}
	if len(specs) != 8 {
		t.Fatalf("install specs = %#v", specs)
	}
	if specs[0].Executable != "systemctl" || !reflect.DeepEqual(specs[0].Arguments[:2], []string{"link", "--force"}) || specs[1].Arguments[0] != "daemon-reload" {
		t.Fatalf("systemd preparation specs = %#v", specs[:2])
	}
	for index, service := range services {
		spec := specs[index+2]
		if !reflect.DeepEqual(spec.Arguments, []string{"enable", "--now", service.Name + ".service"}) {
			t.Errorf("start spec %d = %#v", index, spec)
		}
	}
	for index, service := range services {
		spec := specs[5+index]
		if !reflect.DeepEqual(spec.Arguments, []string{"is-active", "--quiet", service.Name + ".service"}) {
			t.Errorf("health spec %d = %#v", index, spec)
		}
	}
}

func TestNativeUpdateRestartsEveryServiceAndChecksEachOne(t *testing.T) {
	plan := nativeCorePlanForTest(t)
	services, err := BuildNativeServicePlan(plan, nativeTestInstallationID, NativePlatformLinux)
	if err != nil {
		t.Fatal(err)
	}
	linux, err := BuildNativeProcessSpecs(NativeActionUpdate, plan, nativeTestInstallationID, NativePlatformLinux)
	if err != nil {
		t.Fatal(err)
	}
	for index, service := range services {
		if !reflect.DeepEqual(linux[2+index].Arguments, []string{"restart", service.Name + ".service"}) {
			t.Errorf("Linux update restart %d = %#v", index, linux[2+index])
		}
		if !reflect.DeepEqual(linux[2+len(services)+index].Arguments, []string{"is-active", "--quiet", service.Name + ".service"}) {
			t.Errorf("Linux update health %d = %#v", index, linux[2+len(services)+index])
		}
	}

	windows, err := BuildNativeProcessSpecs(NativeActionUpdate, plan, nativeTestInstallationID, NativePlatformWindows)
	if err != nil {
		t.Fatal(err)
	}
	all := make([]string, len(windows))
	for index, spec := range windows {
		all[index] = spec.Executable + " " + strings.Join(spec.Arguments, " ")
	}
	joined := strings.Join(all, "\n")
	if strings.Contains(joined, " create ") {
		t.Fatalf("Windows update attempted to recreate services:\n%s", joined)
	}
	for _, service := range services {
		for _, required := range []string{"config " + service.Name, "stop " + service.Name, "start " + service.Name, "Get-Service -Name '" + service.Name + "'", "Stopped"} {
			if !strings.Contains(joined, required) {
				t.Errorf("Windows update missing %q:\n%s", required, joined)
			}
		}
	}
	assertWindowsStopWaitStartOrder(t, windows, len(services))
	restart, err := BuildNativeProcessSpecs(NativeActionRestart, plan, nativeTestInstallationID, NativePlatformWindows)
	if err != nil {
		t.Fatal(err)
	}
	assertWindowsStopWaitStartOrder(t, restart, len(services))
	assertWindowsRunningChecksAfterStarts(t, restart, len(services))
	linuxRestart, err := BuildNativeProcessSpecs(NativeActionRestart, plan, nativeTestInstallationID, NativePlatformLinux)
	if err != nil {
		t.Fatal(err)
	}
	if len(linuxRestart) != len(services)*2 {
		t.Fatalf("Linux restart specs = %#v", linuxRestart)
	}
	for index, service := range services {
		if !reflect.DeepEqual(linuxRestart[index].Arguments, []string{"restart", service.Name + ".service"}) ||
			!reflect.DeepEqual(linuxRestart[len(services)+index].Arguments, []string{"is-active", "--quiet", service.Name + ".service"}) {
			t.Errorf("Linux restart/health %d = %#v / %#v", index, linuxRestart[index], linuxRestart[len(services)+index])
		}
	}
	uninstall, err := BuildNativeProcessSpecs(NativeActionUninstall, plan, nativeTestInstallationID, NativePlatformWindows)
	if err != nil {
		t.Fatal(err)
	}
	assertWindowsUninstallWaitsForRemoval(t, uninstall, len(services))
}

func assertWindowsRunningChecksAfterStarts(t *testing.T, specs []ProcessSpec, serviceCount int) {
	t.Helper()
	lastStart := -1
	runningChecks := 0
	for index, spec := range specs {
		arguments := strings.Join(spec.Arguments, " ")
		if len(spec.Arguments) > 0 && spec.Executable == "sc.exe" && spec.Arguments[0] == "start" {
			lastStart = index
		}
		if spec.Executable == "powershell.exe" && strings.Contains(arguments, "Running") {
			runningChecks++
			if index <= lastStart {
				t.Errorf("Running check %d precedes final start: %#v", index, specs)
			}
		}
	}
	if lastStart < 0 || runningChecks != serviceCount {
		t.Fatalf("Windows start/health sequence = %#v", specs)
	}
}

func assertWindowsStopWaitStartOrder(t *testing.T, specs []ProcessSpec, serviceCount int) {
	t.Helper()
	lastStop := -1
	firstStart := len(specs)
	stoppedChecks := 0
	for index, spec := range specs {
		if len(spec.Arguments) > 0 && spec.Executable == "sc.exe" && spec.Arguments[0] == "stop" {
			lastStop = index
		}
		if len(spec.Arguments) > 0 && spec.Executable == "sc.exe" && spec.Arguments[0] == "start" && firstStart == len(specs) {
			firstStart = index
		}
		if spec.Executable == "powershell.exe" && strings.Contains(strings.Join(spec.Arguments, " "), "Stopped") {
			stoppedChecks++
			if index <= lastStop || index >= firstStart {
				t.Errorf("Stopped check %d is not between stop and start batches: %#v", index, specs)
			}
		}
	}
	if lastStop < 0 || firstStart == len(specs) || stoppedChecks != serviceCount {
		t.Fatalf("Windows stop/wait/start sequence = %#v", specs)
	}
}

func assertWindowsUninstallWaitsForRemoval(t *testing.T, specs []ProcessSpec, serviceCount int) {
	t.Helper()
	lastStop := -1
	firstDelete := len(specs)
	lastDelete := -1
	stoppedChecks := 0
	absentChecks := 0
	for index, spec := range specs {
		arguments := strings.Join(spec.Arguments, " ")
		if len(spec.Arguments) > 0 && spec.Executable == "sc.exe" && spec.Arguments[0] == "stop" {
			lastStop = index
		}
		if len(spec.Arguments) > 0 && spec.Executable == "sc.exe" && spec.Arguments[0] == "delete" {
			if firstDelete == len(specs) {
				firstDelete = index
			}
			lastDelete = index
		}
		if spec.Executable == "powershell.exe" && strings.Contains(arguments, "Stopped") {
			stoppedChecks++
			if index <= lastStop || index >= firstDelete {
				t.Errorf("uninstall Stopped check %d is outside stop/delete boundary: %#v", index, specs)
			}
		}
		if spec.Executable == "powershell.exe" && strings.Contains(arguments, "$null -eq $service") && !strings.Contains(arguments, "Stopped") {
			absentChecks++
			if index <= lastDelete {
				t.Errorf("uninstall absence check %d precedes final delete: %#v", index, specs)
			}
		}
	}
	if lastStop < 0 || firstDelete == len(specs) || stoppedChecks != serviceCount || absentChecks != serviceCount {
		t.Fatalf("Windows uninstall stop/wait/delete/absent sequence = %#v", specs)
	}
}

func TestNativeWindowsProcessSpecsUseSCMServiceHostAndDependencies(t *testing.T) {
	plan := nativeCorePlanForTest(t)
	services, err := BuildNativeServicePlan(plan, nativeTestInstallationID, NativePlatformWindows)
	if err != nil {
		t.Fatal(err)
	}
	specs, err := BuildNativeProcessSpecs(NativeActionInstall, plan, nativeTestInstallationID, NativePlatformWindows)
	if err != nil {
		t.Fatal(err)
	}
	joined := make([]string, len(specs))
	healthChecks := make([]ProcessSpec, 0, len(services))
	for index, spec := range specs {
		joined[index] = spec.Executable + " " + strings.Join(spec.Arguments, " ")
		if spec.Executable == "powershell.exe" {
			healthChecks = append(healthChecks, spec)
		} else if spec.Executable != "sc.exe" {
			t.Errorf("Windows service spec uses %q", spec.Executable)
		}
	}
	all := strings.Join(joined, "\n")
	for _, service := range services {
		if !strings.Contains(all, "create "+service.Name) || !strings.Contains(all, "config "+service.Name) || !strings.Contains(all, "pic-gallery-service-host.exe") || !strings.Contains(all, "--working-directory") || !strings.Contains(all, "--executable") {
			t.Fatalf("Windows SCM specs missing service-host registration for %s:\n%s", service.Name, all)
		}
		if !strings.Contains(all, "failure "+service.Name) || !strings.Contains(all, "failureflag "+service.Name) || !strings.Contains(all, "start "+service.Name) {
			t.Fatalf("Windows SCM specs missing recovery/start for %s:\n%s", service.Name, all)
		}
	}
	if len(healthChecks) != len(services) {
		t.Fatalf("Windows health checks = %#v", healthChecks)
	}
	for index, service := range services {
		command := strings.Join(healthChecks[index].Arguments, " ")
		for _, required := range []string{service.Name, "Get-Service", "Running", "Start-Sleep"} {
			if !strings.Contains(command, required) {
				t.Errorf("Windows health check %d missing %q: %s", index, required, command)
			}
		}
	}
	for _, service := range services[1:] {
		if !strings.Contains(all, "depend= "+services[0].Name) {
			t.Fatalf("Windows service %s missing API dependency:\n%s", service.Name, all)
		}
	}
	lastStart := -1
	for _, service := range services {
		position := strings.Index(all, "start "+service.Name)
		if position <= lastStart {
			t.Fatalf("Windows service start order is wrong:\n%s", all)
		}
		lastStart = position
	}
	for _, forbidden := range []string{"schtasks", "Register-ScheduledTask", "PIC_GALLERY_ENV_FILE", "APP_ENV_FILE"} {
		if strings.Contains(all, forbidden) {
			t.Fatalf("Windows native specs contain %q:\n%s", forbidden, all)
		}
	}
}

func TestNativeExecutorOnlyIgnoresExpectedWindowsSCMIdempotencyCodes(t *testing.T) {
	plan := nativeCorePlanForTest(t)
	runtimeEnv := []byte("INSTALLATION_ID=" + nativeTestInstallationID + "\n")
	calls := 0
	executor := NativeExecutor{
		Runner: processRunnerFunc(func(_ context.Context, spec ProcessSpec) error {
			calls++
			switch spec.Arguments[0] {
			case "create":
				return fmt.Errorf("wrapped create: %w", nativeExitError(1073))
			case "start":
				return fmt.Errorf("wrapped start: %w", nativeExitError(1056))
			default:
				return nil
			}
		}),
		ReadFile: func(string) ([]byte, error) { return runtimeEnv, nil },
		Platform: func() NativePlatform { return NativePlatformWindows }, CheckPrivileges: func(NativePlatform) error { return nil },
		InstallRelease: func(context.Context, InstallPlan, NativePlatform) error { return nil }, WriteServiceFile: func(string, []byte) error { return nil },
	}
	if err := executor.Run(context.Background(), NativeActionInstall, plan); err != nil {
		t.Fatalf("idempotent Windows install: %v", err)
	}
	if calls == 0 {
		t.Fatal("Windows install did not run service commands")
	}
	executor.Runner = processRunnerFunc(func(context.Context, ProcessSpec) error { return nativeExitError(5) })
	if err := executor.Run(context.Background(), NativeActionInstall, plan); err == nil {
		t.Fatal("Windows install ignored an unexpected SCM error")
	}
}

func TestNativeExecutorHandlesStoppedOrMissingWindowsServicesByAction(t *testing.T) {
	plan := nativeCorePlanForTest(t)
	runtimeEnv := []byte("INSTALLATION_ID=" + nativeTestInstallationID + "\n")
	newExecutor := func(action NativeAction, exitCodes map[string]int) NativeExecutor {
		return NativeExecutor{
			Runner: processRunnerFunc(func(_ context.Context, spec ProcessSpec) error {
				if code := exitCodes[spec.Arguments[0]]; code != 0 {
					return nativeExitError(code)
				}
				return nil
			}),
			ReadFile: func(string) ([]byte, error) { return runtimeEnv, nil },
			Platform: func() NativePlatform { return NativePlatformWindows }, CheckPrivileges: func(NativePlatform) error { return nil },
		}
	}
	for _, testCase := range []struct {
		name   string
		action NativeAction
		codes  map[string]int
	}{
		{name: "restart stopped", action: NativeActionRestart, codes: map[string]int{"stop": 1062, "start": 1056}},
		{name: "uninstall stopped", action: NativeActionUninstall, codes: map[string]int{"stop": 1062}},
		{name: "uninstall missing", action: NativeActionUninstall, codes: map[string]int{"stop": 1060, "delete": 1060}},
		{name: "uninstall marked for deletion", action: NativeActionUninstall, codes: map[string]int{"stop": 1072, "delete": 1072}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			executor := newExecutor(testCase.action, testCase.codes)
			if err := executor.Run(context.Background(), testCase.action, plan); err != nil {
				t.Fatalf("idempotent %s: %v", testCase.action, err)
			}
		})
	}
	executor := newExecutor(NativeActionRestart, map[string]int{"stop": 1060})
	if err := executor.Run(context.Background(), NativeActionRestart, plan); err == nil {
		t.Fatal("restart ignored a missing Windows service")
	}
}

func TestNativeWindowsCommandLineQuotingPreservesPathSeparators(t *testing.T) {
	tests := map[string]string{
		`C:\runtime\bin.exe`:           `C:\runtime\bin.exe`,
		`C:\Program Files\app\bin.exe`: `"C:\Program Files\app\bin.exe"`,
		`C:\Program Files\app\`:        `"C:\Program Files\app\\"`,
		"":                             `""`,
	}
	for input, want := range tests {
		if got := windowsQuoteCommandLineArgument(input); got != want {
			t.Errorf("windowsQuoteCommandLineArgument(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestNativeProcessSpecsRejectUnknownAction(t *testing.T) {
	plan := nativeCorePlanForTest(t)
	for _, platform := range []NativePlatform{NativePlatformLinux, NativePlatformWindows} {
		if _, err := BuildNativeProcessSpecs(NativeAction("unknown"), plan, nativeTestInstallationID, platform); err == nil {
			t.Errorf("platform %s accepted an unknown native action", platform)
		}
	}
}

func nativeCorePlanForTest(t *testing.T) InstallPlan {
	t.Helper()
	plan, err := BuildInstallPlan(InstallInput{
		Mode: "native", Profile: "core", Topology: "single", Role: "single", RuntimeDir: filepath.Join(t.TempDir(), "runtime with spaces"),
		StorageDriver: "local", ApplicationVersion: "v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

type processRunnerFunc func(context.Context, ProcessSpec) error

func (run processRunnerFunc) Run(ctx context.Context, spec ProcessSpec) error { return run(ctx, spec) }

type nativeExitError int

func (code nativeExitError) Error() string { return fmt.Sprintf("exit %d", code) }
func (code nativeExitError) ExitCode() int { return int(code) }

type nativeArchiveEntry struct {
	content  string
	mode     int64
	typeflag byte
}

func nativeReleaseArchiveForTest(t *testing.T, entries map[string]nativeArchiveEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, entry := range entries {
		typeflag := entry.typeflag
		if typeflag == 0 {
			typeflag = tar.TypeReg
		}
		size := int64(len(entry.content))
		if typeflag == tar.TypeDir {
			size = 0
		}
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: entry.mode, Size: size, Typeflag: typeflag}); err != nil {
			t.Fatal(err)
		}
		if size > 0 {
			if _, err := io.WriteString(tarWriter, entry.content); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
