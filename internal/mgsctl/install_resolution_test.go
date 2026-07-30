package mgsctl

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/fatballfish/pic-gallery/internal/config"
)

func TestResolveInstallInputDerivesDockerApplicationVersionAndDigests(t *testing.T) {
	input := InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileFull,
		Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle,
		RuntimeDir: ".", ImageTag: "latest",
	}
	called := false
	resolved, err := ResolveInstallInput(context.Background(), input, func(_ context.Context, options ReleaseManifestOptions) (ResolvedRelease, error) {
		called = options.Version == "latest" && len(options.Components) == 9
		return resolvedReleaseForInstallTest(), nil
	})
	if err != nil {
		t.Fatalf("ResolveInstallInput: %v", err)
	}
	if !called || resolved.ApplicationVersion != "v1.2.3" || resolved.ImageTag != "v1.2.3" {
		t.Fatalf("resolved Docker input = %#v, called=%t", resolved, called)
	}
	if resolved.ImageRegistry != "docker.io/fatballfish" || len(resolved.ImageDigests) != 5 || resolved.ImageDigests[ComponentAPI] != "sha256:"+strings.Repeat("1", 64) {
		t.Fatalf("resolved Docker artifacts = registry %q digests %#v", resolved.ImageRegistry, resolved.ImageDigests)
	}
	plan, err := BuildInstallPlan(resolved)
	if err != nil {
		t.Fatalf("BuildInstallPlan: %v", err)
	}
	if plan.ApplicationVersion != "v1.2.3" || plan.ImageTag != "v1.2.3" || plan.ImageDigests[ComponentDocsWeb] == "" {
		t.Fatalf("resolved install plan = %#v", plan)
	}
}

func TestResolveInstallInputRejectsReleaseImagesFromDifferentRegistries(t *testing.T) {
	input := InstallInput{
		Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileFull,
		Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle,
		RuntimeDir: ".", ImageTag: "latest",
	}
	_, err := ResolveInstallInput(context.Background(), input, func(context.Context, ReleaseManifestOptions) (ResolvedRelease, error) {
		resolved := resolvedReleaseForInstallTest()
		worker := resolved.Images[ComponentWorker]
		worker.Repository = "registry.example.test/team/mikiko-gallery-studio-worker"
		resolved.Images[ComponentWorker] = worker
		return resolved, nil
	})
	if err == nil || !strings.Contains(err.Error(), "one registry") {
		t.Fatalf("mixed release registries error = %v", err)
	}
}

func TestResolveInstallInputDerivesNativeReleaseVersion(t *testing.T) {
	input := InstallInput{
		Mode: config.DeploymentModeNative, Profile: config.DeploymentProfileCore,
		Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle,
		RuntimeDir: ".", ReleaseVersion: "latest",
	}
	resolved, err := ResolveInstallInput(context.Background(), input, func(_ context.Context, options ReleaseManifestOptions) (ResolvedRelease, error) {
		if options.Version != "latest" {
			t.Fatalf("native selector = %q", options.Version)
		}
		return resolvedReleaseForInstallTest(), nil
	})
	if err != nil {
		t.Fatalf("ResolveInstallInput: %v", err)
	}
	if resolved.ApplicationVersion != "v1.2.3" || resolved.ReleaseVersion != "v1.2.3" || resolved.ImageTag != "" || len(resolved.ImageDigests) != 0 {
		t.Fatalf("resolved Native input = %#v", resolved)
	}
}

func resolvedReleaseForInstallTest() ResolvedRelease {
	images := make(map[Component]ReleaseImage, 5)
	for index, component := range []Component{ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb} {
		images[component] = ReleaseImage{
			Repository: "docker.io/fatballfish/mikiko-gallery-studio-" + string(component),
			Tag:        "v1.2.3", Digest: "sha256:" + strings.Repeat(string(rune('1'+index)), 64),
			Version: "v1.2.3", Revision: strings.Repeat("a", 40),
		}
	}
	return ResolvedRelease{ApplicationVersion: "v1.2.3", Commit: strings.Repeat("a", 40), Images: images, MigrationImage: images[ComponentAPI]}
}

func resolvedReleaseForInstallSelector(_ context.Context, options ReleaseManifestOptions) (ResolvedRelease, error) {
	version := options.Version
	if version == "" || version == "latest" {
		version = "v1.2.3"
	}
	resolved := resolvedReleaseForInstallTest()
	resolved.ApplicationVersion = version
	resolved.Images = make(map[Component]ReleaseImage)
	for _, component := range options.Components {
		if !releaseImageComponent(component) {
			continue
		}
		image := ReleaseImage{
			Repository: "docker.io/fatballfish/mikiko-gallery-studio-" + string(component),
			Tag:        version, Digest: "sha256:" + strings.Repeat("1", 64), Version: version, Revision: strings.Repeat("a", 40),
		}
		resolved.Images[component] = image
		if component == ComponentAPI {
			resolved.MigrationImage = image
		}
	}
	if len(resolved.Images) == 0 && len(options.Components) > 0 {
		return ResolvedRelease{}, fmt.Errorf("no releasable components selected")
	}
	return resolved, nil
}
