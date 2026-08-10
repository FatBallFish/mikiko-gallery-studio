package mgsctl

import (
	"fmt"
	"net/url"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/fatballfish/pic-gallery/internal/config"
)

type Component string

const (
	ComponentAPI        Component = "api"
	ComponentWorker     Component = "worker"
	ComponentUserWeb    Component = "user-web"
	ComponentAdminWeb   Component = "admin-web"
	ComponentDocsWeb    Component = "docs-web"
	ComponentGateway    Component = "gateway"
	ComponentPostgres   Component = "postgres"
	ComponentRedis      Component = "redis"
	ComponentMinIO      Component = "minio"
	ComponentMonitoring Component = "monitoring"
)

var componentOrder = []Component{
	ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb,
	ComponentGateway, ComponentPostgres, ComponentRedis, ComponentMinIO, ComponentMonitoring,
}

var applicationPreset = []Component{ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb, ComponentGateway}
var fullPreset = []Component{ComponentAPI, ComponentWorker, ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb, ComponentGateway, ComponentPostgres, ComponentRedis, ComponentMinIO}

type InstallInput struct {
	Interactive              bool
	OverwriteExisting        bool
	Mode                     config.DeploymentMode
	Profile                  config.DeploymentProfile
	Topology                 config.DeploymentTopology
	Role                     config.DeploymentRole
	Components               []Component
	RuntimeDir               string
	StorageDriver            string
	PublicAPIURL             string
	DocsURL                  string
	DocsProbeURL             string
	ExternalGatewayConfirmed bool
	InstallationInitialized  bool
	MigrationRequested       bool
	ApplicationVersion       string
	ImageRegistry            string
	ImageTag                 string
	ImageDigests             map[Component]string
	ReleaseVersion           string
	APIPort                  string
	GatewayPort              string
	UserWebPort              string
	AdminWebPort             string
	DocsWebPort              string
	MonitoringPort           string
	ModeExplicit             bool
	ProfileExplicit          bool
	TopologyExplicit         bool
	RoleExplicit             bool
	StorageDriverExplicit    bool
	RuntimeDirExplicit       bool
	ImageTagExplicit         bool
	ReleaseVersionExplicit   bool
	APIPortExplicit          bool
	GatewayPortExplicit      bool
	UserWebPortExplicit      bool
	AdminWebPortExplicit     bool
	DocsWebPortExplicit      bool
	MonitoringPortExplicit   bool
}

type InstallPlan struct {
	Mode                     config.DeploymentMode     `json:"mode"`
	Profile                  config.DeploymentProfile  `json:"profile"`
	Topology                 config.DeploymentTopology `json:"topology"`
	Role                     config.DeploymentRole     `json:"role"`
	Components               []Component               `json:"components"`
	RuntimeDir               string                    `json:"runtime_dir"`
	StorageDriver            string                    `json:"storage_driver,omitempty"`
	PublicAPIURL             string                    `json:"public_api_url,omitempty"`
	DocsURL                  string                    `json:"docs_url,omitempty"`
	DocsProbeURL             string                    `json:"docs_probe_url,omitempty"`
	ExternalGatewayConfirmed bool                      `json:"external_gateway_confirmed,omitempty"`
	MigrationRequested       bool                      `json:"migration_requested,omitempty"`
	ApplicationVersion       string                    `json:"application_version"`
	ImageRegistry            string                    `json:"image_registry,omitempty"`
	ImageTag                 string                    `json:"image_tag,omitempty"`
	ImageDigests             map[Component]string      `json:"image_digests,omitempty"`
	ReleaseVersion           string                    `json:"release_version,omitempty"`
	APIPort                  string                    `json:"api_port,omitempty"`
	GatewayPort              string                    `json:"gateway_port,omitempty"`
	UserWebPort              string                    `json:"user_web_port,omitempty"`
	AdminWebPort             string                    `json:"admin_web_port,omitempty"`
	DocsWebPort              string                    `json:"docs_web_port,omitempty"`
	MonitoringPort           string                    `json:"monitoring_port,omitempty"`
	RequiresEnrollment       bool                      `json:"requires_enrollment"`
}

func BuildInstallPlan(input InstallInput) (InstallPlan, error) {
	if input.RuntimeDir == "" {
		input.RuntimeDir = "."
	}
	input.RuntimeDir = filepath.Clean(input.RuntimeDir)
	if input.ApplicationVersion == "" {
		input.ApplicationVersion = DefaultApplicationVersion
	}
	if input.StorageDriver == "" && input.Role != config.DeploymentRoleWeb {
		if input.Topology == config.DeploymentTopologyCluster || input.Profile == config.DeploymentProfileFull || (input.Profile == config.DeploymentProfileCustom && slices.Contains(input.Components, ComponentMinIO)) {
			input.StorageDriver = "s3"
		} else {
			input.StorageDriver = "local"
		}
	}
	if err := config.ValidateApplicationVersion(input.ApplicationVersion); err != nil {
		return InstallPlan{}, fmt.Errorf("validate application version: %w", err)
	}
	if input.Profile == config.DeploymentProfileFull && input.StorageDriver != "s3" {
		return InstallPlan{}, fmt.Errorf("Docker full profile requires s3 object storage managed by MinIO")
	}
	joined := input.Role == config.DeploymentRoleAPI || input.Role == config.DeploymentRoleWorker || input.Role == config.DeploymentRoleWeb
	context := config.DeploymentContext{
		Mode: input.Mode, Profile: input.Profile, Topology: input.Topology, Role: input.Role,
		StorageDriver: input.StorageDriver, SetupCompleted: joined && input.InstallationInitialized,
	}
	if err := config.ValidateDeploymentContext(context); err != nil {
		return InstallPlan{}, fmt.Errorf("validate deployment context: %w", err)
	}
	if joined && !input.InstallationInitialized {
		return InstallPlan{}, fmt.Errorf("joined role %q requires an initialized installation and cluster enrollment", input.Role)
	}
	if input.MigrationRequested && input.Role != config.DeploymentRoleSingle && input.Role != config.DeploymentRoleControl {
		return InstallPlan{}, fmt.Errorf("deployment role %q cannot execute migrations", input.Role)
	}
	if strings.TrimSpace(input.PublicAPIURL) != "" {
		if err := validatePublicAPIURL(input.PublicAPIURL); err != nil {
			return InstallPlan{}, err
		}
	}
	input.APIPort = defaultString(input.APIPort, "8080")
	for name, value := range map[string]string{
		"API": input.APIPort, "Gateway": input.GatewayPort, "user web": input.UserWebPort,
		"admin web": input.AdminWebPort, "documentation web": input.DocsWebPort, "monitoring": input.MonitoringPort,
	} {
		if err := validateInstallPort(name, value); err != nil {
			return InstallPlan{}, err
		}
	}

	components, err := componentsForInput(input)
	if err != nil {
		return InstallPlan{}, err
	}
	if slices.Contains(components, ComponentGateway) {
		input.GatewayPort = defaultString(input.GatewayPort, "80")
	}
	if slices.Contains(components, ComponentUserWeb) {
		input.UserWebPort = defaultString(input.UserWebPort, "5173")
	}
	if slices.Contains(components, ComponentAdminWeb) {
		input.AdminWebPort = defaultString(input.AdminWebPort, "5174")
	}
	if slices.Contains(components, ComponentDocsWeb) {
		input.DocsWebPort = defaultString(input.DocsWebPort, "5175")
	}
	if slices.Contains(components, ComponentMonitoring) {
		input.MonitoringPort = defaultString(input.MonitoringPort, "9090")
	}
	if err := validateComponents(input, components); err != nil {
		return InstallPlan{}, err
	}
	docsURL, docsProbeURL, err := resolveInstallDocumentationTargets(input.DocsURL, input.DocsProbeURL, input.Mode, components, input.GatewayPort)
	if err != nil {
		return InstallPlan{}, err
	}

	return InstallPlan{
		Mode: input.Mode, Profile: input.Profile, Topology: input.Topology, Role: input.Role,
		Components: components, RuntimeDir: input.RuntimeDir, StorageDriver: input.StorageDriver,
		PublicAPIURL: strings.TrimSpace(input.PublicAPIURL), ExternalGatewayConfirmed: input.ExternalGatewayConfirmed,
		DocsURL:            docsURL,
		DocsProbeURL:       docsProbeURL,
		MigrationRequested: input.MigrationRequested, ApplicationVersion: input.ApplicationVersion,
		ImageRegistry: input.ImageRegistry, ImageTag: defaultString(input.ImageTag, input.ApplicationVersion), ImageDigests: cloneImageDigests(input.ImageDigests),
		ReleaseVersion: defaultString(input.ReleaseVersion, input.ApplicationVersion),
		APIPort:        input.APIPort, GatewayPort: input.GatewayPort,
		UserWebPort: input.UserWebPort, AdminWebPort: input.AdminWebPort, DocsWebPort: input.DocsWebPort,
		MonitoringPort:     input.MonitoringPort,
		RequiresEnrollment: joined,
	}, nil
}

func ValidateInstallPlan(plan InstallPlan) error {
	if strings.TrimSpace(plan.RuntimeDir) == "" || filepath.Clean(plan.RuntimeDir) != plan.RuntimeDir {
		return fmt.Errorf("runtime directory must be a non-empty clean path")
	}
	if err := config.ValidateApplicationVersion(plan.ApplicationVersion); err != nil {
		return fmt.Errorf("validate application version: %w", err)
	}
	joined := plan.Role == config.DeploymentRoleAPI || plan.Role == config.DeploymentRoleWorker || plan.Role == config.DeploymentRoleWeb
	if plan.RequiresEnrollment != joined {
		return fmt.Errorf("deployment role %q has inconsistent enrollment metadata", plan.Role)
	}
	if err := config.ValidateDeploymentContext(config.DeploymentContext{
		Mode: plan.Mode, Profile: plan.Profile, Topology: plan.Topology, Role: plan.Role,
		StorageDriver: plan.StorageDriver, SetupCompleted: joined,
	}); err != nil {
		return fmt.Errorf("validate deployment context: %w", err)
	}
	if plan.MigrationRequested && plan.Role != config.DeploymentRoleSingle && plan.Role != config.DeploymentRoleControl {
		return fmt.Errorf("deployment role %q cannot execute migrations", plan.Role)
	}
	if plan.Profile == config.DeploymentProfileFull && plan.StorageDriver != "s3" {
		return fmt.Errorf("Docker full profile requires s3 object storage managed by MinIO")
	}
	if strings.TrimSpace(plan.PublicAPIURL) != "" {
		if err := validatePublicAPIURL(plan.PublicAPIURL); err != nil {
			return err
		}
	}
	if err := validateInstallDocumentationTargets(plan.DocsURL, plan.DocsProbeURL); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"API": plan.APIPort, "Gateway": plan.GatewayPort, "user web": plan.UserWebPort,
		"admin web": plan.AdminWebPort, "documentation web": plan.DocsWebPort, "monitoring": plan.MonitoringPort,
	} {
		if err := validateInstallPort(name, value); err != nil {
			return err
		}
	}

	input := InstallInput{
		Mode: plan.Mode, Profile: plan.Profile, Topology: plan.Topology, Role: plan.Role,
		StorageDriver: plan.StorageDriver, PublicAPIURL: plan.PublicAPIURL, DocsURL: plan.DocsURL,
		DocsProbeURL:             plan.DocsProbeURL,
		ExternalGatewayConfirmed: plan.ExternalGatewayConfirmed,
	}
	if plan.Profile == config.DeploymentProfileCustom {
		input.Components = plan.Components
	}
	expected, err := componentsForInput(input)
	if err != nil {
		return err
	}
	if !slices.Equal(plan.Components, expected) {
		return fmt.Errorf("components %v do not match the %s/%s deployment plan", plan.Components, plan.Profile, plan.Role)
	}
	if err := validateComponents(input, plan.Components); err != nil {
		return err
	}
	if slices.Contains(plan.Components, ComponentAPI) && strings.TrimSpace(plan.APIPort) == "" {
		return fmt.Errorf("API component requires an API port")
	}
	for component, digest := range plan.ImageDigests {
		if !releaseImageComponent(component) || !slices.Contains(plan.Components, component) {
			return fmt.Errorf("image digest component %q is not deployed by this plan", component)
		}
		if !validSHA256Digest(digest) {
			return fmt.Errorf("image digest for %s must be an immutable sha256 digest", component)
		}
	}
	return nil
}

func defaultDocsProbeURL(mode config.DeploymentMode, components []Component, gatewayPort string) string {
	if !slices.Contains(components, ComponentAPI) || !slices.Contains(components, ComponentGateway) {
		return ""
	}
	if mode == config.DeploymentModeDocker {
		return "http://gateway/developer-docs/"
	}
	if mode == config.DeploymentModeNative && strings.TrimSpace(gatewayPort) != "" {
		return "http://127.0.0.1:" + strings.TrimSpace(gatewayPort) + "/developer-docs/"
	}
	return ""
}

func resolveInstallDocumentationTargets(rawDocsURL, rawProbeURL string, mode config.DeploymentMode, components []Component, gatewayPort string) (string, string, error) {
	docsURL := defaultString(rawDocsURL, "/developer-docs/")
	parsedDocs, err := validateInstallDocsURL(docsURL)
	if err != nil {
		return "", "", err
	}
	probeURL := strings.TrimSpace(rawProbeURL)
	if parsedDocs.IsAbs() {
		if probeURL != "" {
			return "", "", fmt.Errorf("documentation probe URL is allowed only with a relative documentation URL")
		}
		return docsURL, "", nil
	}
	if probeURL == "" {
		probeURL = defaultDocsProbeURL(mode, components, gatewayPort)
	}
	if err := validateInstallDocumentationTargets(docsURL, probeURL); err != nil {
		return "", "", err
	}
	return docsURL, probeURL, nil
}

func validateInstallDocumentationTargets(rawDocsURL, rawProbeURL string) error {
	docsURL := defaultString(rawDocsURL, "/developer-docs/")
	parsedDocs, err := validateInstallDocsURL(docsURL)
	if err != nil {
		return err
	}
	probeURL := strings.TrimSpace(rawProbeURL)
	if parsedDocs.IsAbs() {
		if probeURL != "" {
			return fmt.Errorf("documentation probe URL is allowed only with a relative documentation URL")
		}
		return nil
	}
	if probeURL == "" {
		return nil
	}
	if err := validateHTTPBaseURL(probeURL, "documentation probe URL"); err != nil {
		return err
	}
	parsedProbe, err := url.Parse(probeURL)
	if err != nil || parsedProbe.EscapedPath() != parsedDocs.EscapedPath() {
		return fmt.Errorf("documentation URL and probe URL path must match")
	}
	return nil
}

func validateInstallDocsURL(value string) (*url.URL, error) {
	raw := strings.TrimSpace(value)
	if raw == "" || strings.Contains(raw, "\\") {
		return nil, fmt.Errorf("documentation URL is invalid")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Opaque != "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return nil, fmt.Errorf("documentation URL must be an absolute HTTP(S) URL or a path beginning with / without credentials, query, fragment, or backslashes")
	}
	cleanedPath := pathpkg.Clean(parsed.Path)
	if parsed.Path != "" && parsed.Path != cleanedPath && !(strings.HasSuffix(parsed.Path, "/") && strings.TrimSuffix(parsed.Path, "/") == cleanedPath) {
		return nil, fmt.Errorf("documentation URL path must not contain traversal or duplicate separators")
	}
	if parsed.IsAbs() {
		if err := validateHTTPBaseURL(raw, "documentation URL"); err != nil {
			return nil, err
		}
		return parsed, nil
	}
	if parsed.Host != "" || !strings.HasPrefix(parsed.Path, "/") || strings.HasPrefix(parsed.Path, "//") {
		return nil, fmt.Errorf("documentation URL must be an absolute HTTP(S) URL or a path beginning with /")
	}
	return parsed, nil
}

func cloneImageDigests(values map[Component]string) map[Component]string {
	if len(values) == 0 {
		return nil
	}
	cloned := make(map[Component]string, len(values))
	for component, digest := range values {
		cloned[component] = digest
	}
	return cloned
}

func componentsForInput(input InstallInput) ([]Component, error) {
	if input.Profile != config.DeploymentProfileCustom && len(input.Components) > 0 {
		return nil, fmt.Errorf("component overrides require the custom profile")
	}
	if input.Profile == config.DeploymentProfileFull {
		return slices.Clone(fullPreset), nil
	}
	if input.Profile == config.DeploymentProfileCustom {
		if len(input.Components) == 0 {
			return nil, fmt.Errorf("custom profile requires at least one component")
		}
		return canonicalComponents(input.Components)
	}
	switch input.Role {
	case config.DeploymentRoleSingle, config.DeploymentRoleControl:
		return slices.Clone(applicationPreset), nil
	case config.DeploymentRoleAPI:
		return []Component{ComponentAPI}, nil
	case config.DeploymentRoleWorker:
		return []Component{ComponentWorker}, nil
	case config.DeploymentRoleWeb:
		return []Component{ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb, ComponentGateway}, nil
	default:
		return nil, fmt.Errorf("unsupported deployment role %q", input.Role)
	}
}

func canonicalComponents(components []Component) ([]Component, error) {
	seen := make(map[Component]struct{}, len(components))
	for _, component := range components {
		if !slices.Contains(componentOrder, component) {
			return nil, fmt.Errorf("unknown component %q", component)
		}
		if _, exists := seen[component]; exists {
			return nil, fmt.Errorf("duplicate component %q", component)
		}
		seen[component] = struct{}{}
	}
	ordered := make([]Component, 0, len(seen))
	for _, component := range componentOrder {
		if _, exists := seen[component]; exists {
			ordered = append(ordered, component)
		}
	}
	return ordered, nil
}

func validateComponents(input InstallInput, components []Component) error {
	managed := slices.Contains(components, ComponentPostgres) || slices.Contains(components, ComponentRedis) || slices.Contains(components, ComponentMinIO)
	if managed && input.Topology == config.DeploymentTopologyCluster {
		return fmt.Errorf("cluster deployments cannot manage node-local middleware components")
	}
	if input.Mode == config.DeploymentModeNative && (managed || slices.Contains(components, ComponentMonitoring)) {
		return fmt.Errorf("native deployment cannot manage middleware or monitoring components")
	}
	if slices.Contains(components, ComponentMonitoring) && (input.Mode != config.DeploymentModeDocker || !slices.Contains(components, ComponentAPI)) {
		return fmt.Errorf("monitoring requires a local Docker API component")
	}
	if slices.Contains(components, ComponentMinIO) && input.StorageDriver != "s3" {
		return fmt.Errorf("MinIO component requires s3 object storage")
	}
	if slices.Contains(components, ComponentGateway) {
		requiredComponents := []Component{ComponentUserWeb, ComponentAdminWeb, ComponentDocsWeb}
		if input.Role != config.DeploymentRoleWeb {
			requiredComponents = append([]Component{ComponentAPI}, requiredComponents...)
		}
		for _, required := range requiredComponents {
			if !slices.Contains(components, required) {
				return fmt.Errorf("gateway component requires local component %q", required)
			}
		}
	}
	if input.Role == config.DeploymentRoleSingle || input.Role == config.DeploymentRoleControl {
		if !slices.Contains(components, ComponentAPI) {
			return fmt.Errorf("setup authority role requires the API component")
		}
	}
	allowed := map[config.DeploymentRole]map[Component]bool{
		config.DeploymentRoleAPI:    {ComponentAPI: true},
		config.DeploymentRoleWorker: {ComponentWorker: true},
		config.DeploymentRoleWeb:    {ComponentUserWeb: true, ComponentAdminWeb: true, ComponentDocsWeb: true, ComponentGateway: true},
	}
	if roleAllowed := allowed[input.Role]; roleAllowed != nil {
		for _, component := range components {
			if !roleAllowed[component] {
				return fmt.Errorf("component %q is not allowed on role %q", component, input.Role)
			}
		}
	}
	hasWeb := slices.Contains(components, ComponentUserWeb) || slices.Contains(components, ComponentAdminWeb) || slices.Contains(components, ComponentDocsWeb)
	if hasWeb && !slices.Contains(components, ComponentGateway) && !input.ExternalGatewayConfirmed {
		return fmt.Errorf("web components without gateway require explicit external hosting confirmation")
	}
	if input.Role == config.DeploymentRoleWeb {
		if err := validatePublicAPIURL(input.PublicAPIURL); err != nil {
			return err
		}
	}
	return nil
}

func validatePublicAPIURL(value string) error {
	return validateHTTPBaseURL(value, "public API URL")
}

func validateHTTPBaseURL(value, label string) error {
	raw := strings.TrimSpace(value)
	parsed, err := url.Parse(raw)
	if err != nil || strings.ContainsAny(raw, "\\?#") || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return fmt.Errorf("%s must be an absolute HTTP(S) URL without credentials, query, fragment, or backslashes", label)
	}
	if port := parsed.Port(); port != "" {
		parsedPort, err := strconv.Atoi(port)
		if err != nil || parsedPort < 1 || parsedPort > 65535 {
			return fmt.Errorf("%s port must be between 1 and 65535", label)
		}
	}
	return nil
}

func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return strings.TrimSpace(value)
}

func validateInstallPort(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("%s port must be between 1 and 65535", name)
	}
	return nil
}
