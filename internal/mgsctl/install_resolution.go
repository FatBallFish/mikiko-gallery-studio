package mgsctl

import (
	"context"
	"fmt"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type InstallReleaseResolver func(context.Context, ReleaseManifestOptions) (ResolvedRelease, error)

func ResolveInstallInput(ctx context.Context, input InstallInput, resolve InstallReleaseResolver) (InstallInput, error) {
	if resolve == nil {
		return InstallInput{}, fmt.Errorf("release resolver is required")
	}
	components, err := componentsForInput(input)
	if err != nil {
		return InstallInput{}, err
	}
	selector := input.ImageTag
	if input.Mode == config.DeploymentModeNative {
		selector = input.ReleaseVersion
	}
	selector = defaultString(strings.TrimSpace(selector), "latest")
	resolved, err := resolve(ctx, ReleaseManifestOptions{Version: selector, Components: components})
	if err != nil {
		return InstallInput{}, err
	}
	input.ApplicationVersion = resolved.ApplicationVersion
	if input.Mode == config.DeploymentModeNative {
		input.ReleaseVersion = resolved.ApplicationVersion
		input.ImageTag = ""
		input.ImageDigests = nil
		return input, nil
	}
	if input.Mode != config.DeploymentModeDocker {
		return InstallInput{}, fmt.Errorf("unsupported install mode %q", input.Mode)
	}
	input.ImageTag = resolved.ApplicationVersion
	input.ReleaseVersion = ""
	input.ImageDigests = make(map[Component]string, len(resolved.Images))
	registry := strings.TrimRight(strings.TrimSpace(input.ImageRegistry), "/")
	deriveRegistry := registry == ""
	for component, image := range resolved.Images {
		input.ImageDigests[component] = image.Digest
		if !deriveRegistry {
			continue
		}
		suffix := "/mikiko-gallery-studio-" + string(component)
		if !strings.HasSuffix(image.Repository, suffix) {
			return InstallInput{}, fmt.Errorf("release image repository %q does not match component %s", image.Repository, component)
		}
		candidate := strings.TrimSuffix(image.Repository, suffix)
		if registry == "" {
			registry = candidate
		} else if registry != candidate {
			return InstallInput{}, fmt.Errorf("release images do not share one registry")
		}
	}
	input.ImageRegistry = registry
	return input, nil
}
