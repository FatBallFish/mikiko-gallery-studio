package mgsctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type DoctorCheck struct {
	Code    string
	OK      bool
	Message string
}

type DoctorReport struct {
	Checks []DoctorCheck
}

func (report DoctorReport) Healthy() bool {
	for _, check := range report.Checks {
		if !check.OK {
			return false
		}
	}
	return true
}

func (report DoctorReport) String() string {
	var output strings.Builder
	for _, check := range report.Checks {
		status := "PASS"
		if !check.OK {
			status = "FAIL"
		}
		fmt.Fprintf(&output, "%s %s: %s\n", status, check.Code, check.Message)
	}
	return output.String()
}

type DoctorDependencies struct {
	ReadFile                    func(string) ([]byte, error)
	Stat                        func(string) (os.FileInfo, error)
	CheckRuntimeReadiness       func(context.Context, map[string]string) error
	ProbeMiddleware             func(context.Context, map[string]string) error
	CheckSchema                 func(context.Context, map[string]string) error
	LookPath                    func(string) (string, error)
	CheckDockerWorkerMediaTools func(context.Context, string, map[string]string, string, string) error
	CheckWorkerTempDir          func(string) error
}

func ProductionDoctorDependencies() DoctorDependencies {
	return DoctorDependencies{
		LookPath:                    exec.LookPath,
		CheckDockerWorkerMediaTools: checkDockerWorkerMediaTools,
		CheckWorkerTempDir: func(path string) error {
			if err := os.MkdirAll(path, 0o700); err != nil {
				return err
			}
			file, err := os.CreateTemp(path, ".doctor-*")
			if err != nil {
				return err
			}
			name := file.Name()
			if err := file.Close(); err != nil {
				_ = os.Remove(name)
				return err
			}
			return os.Remove(name)
		},
		CheckRuntimeReadiness: probeDockerAPIReadiness,
		ProbeMiddleware: func(ctx context.Context, values map[string]string) error {
			prober := setup.NewProbeService()
			results := []setup.ProbeResult{
				prober.ProbePostgres(ctx, setup.PostgresProbeRequest{DatabaseURL: values["DATABASE_URL"]}),
				prober.ProbeRedis(ctx, setup.RedisProbeRequest{RedisURL: values["REDIS_URL"], KeyPrefix: values["REDIS_KEY_PREFIX"]}),
				prober.ProbeStorage(ctx, setup.StorageProbeRequest{Config: storageConfigFromValues(values)}),
			}
			failures := make([]string, 0, len(results))
			for _, result := range results {
				if !result.Success {
					failures = append(failures, result.Kind+"="+result.Code)
				}
			}
			if len(failures) > 0 {
				return fmt.Errorf("middleware probes failed: %s", strings.Join(failures, ", "))
			}
			return nil
		},
		CheckSchema: func(ctx context.Context, values map[string]string) error {
			client, err := db.OpenContext(ctx, values["DATABASE_URL"])
			if err != nil {
				return err
			}
			defer client.Close()
			version, err := strconv.Atoi(values["RUNTIME_SCHEMA_VERSION"])
			if err != nil {
				return fmt.Errorf("runtime schema version is invalid")
			}
			return db.CheckSchemaCompatibility(ctx, client, db.SchemaVersion{
				InstallationID: values["INSTALLATION_ID"], AppVersion: values["APPLICATION_VERSION"],
				ConfigVersion: version, DatabaseSchemaVersion: db.CurrentDatabaseSchemaVersion,
			})
		},
	}
}

func probeDockerAPIReadiness(ctx context.Context, values map[string]string) error {
	port, err := strconv.Atoi(strings.TrimSpace(values["API_PORT"]))
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("Docker API port is invalid")
	}
	endpoint := (&url.URL{Scheme: "http", Host: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), Path: "/readyz"}).String()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fmt.Errorf("build Docker API readiness request: %w", err)
	}
	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("Docker API readiness request failed: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("Docker API readiness returned HTTP %d", response.StatusCode)
	}
	return nil
}

const dockerWorkerMediaToolsCheckScript = `command -v -- "$1" >/dev/null && command -v -- "$2" >/dev/null`

func BuildDockerWorkerMediaToolsCheckSpec(runtimeDir, installationID, nodeID, ffmpeg, ffprobe string, baseEnvironment []string) (ProcessSpec, error) {
	absoluteRuntime, err := filepath.Abs(runtimeDir)
	if err != nil {
		return ProcessSpec{}, fmt.Errorf("resolve Docker runtime directory: %w", err)
	}
	projectName, err := dockerProjectName(installationID, nodeID)
	if err != nil {
		return ProcessSpec{}, err
	}
	return ProcessSpec{
		Executable: "docker",
		Arguments: []string{
			"compose", "--project-directory", absoluteRuntime,
			"--env-file", filepath.Join(absoluteRuntime, "config", "runtime.env"),
			"--file", filepath.Join(absoluteRuntime, "compose.yml"),
			"--project-name", projectName,
			"exec", "--no-TTY", "worker", "sh", "-c",
			dockerWorkerMediaToolsCheckScript, "mgsctl-media-tools", ffmpeg, ffprobe,
		},
		Directory:   absoluteRuntime,
		Environment: sanitizeDockerEnvironment(baseEnvironment, absoluteRuntime, dockerRuntimeUser()),
	}, nil
}

func checkDockerWorkerMediaTools(ctx context.Context, runtimeDir string, values map[string]string, ffmpeg, ffprobe string) error {
	spec, err := BuildDockerWorkerMediaToolsCheckSpec(
		runtimeDir, values["INSTALLATION_ID"], values["CLUSTER_NODE_ID"], ffmpeg, ffprobe, os.Environ(),
	)
	if err != nil {
		return fmt.Errorf("build Docker Worker media tools check: %w", err)
	}
	if err := (OSProcessRunner{Stdout: io.Discard, Stderr: io.Discard}).Run(ctx, spec); err != nil {
		return fmt.Errorf("check Docker Worker media tools: %w", err)
	}
	return nil
}

func storageConfigFromValues(values map[string]string) config.StorageConfig {
	forcePathStyle, _ := strconv.ParseBool(values["STORAGE_S3_FORCE_PATH_STYLE"])
	sharedVolume, _ := strconv.ParseBool(values["STORAGE_SHARED_VOLUME"])
	return config.StorageConfig{
		Driver: values["STORAGE_DRIVER"], LocalRoot: values["STORAGE_LOCAL_ROOT"],
		PublicBaseURL: values["STORAGE_PUBLIC_BASE_URL"], SharedVolume: sharedVolume,
		S3: config.StorageS3Config{
			Endpoint: values["STORAGE_S3_ENDPOINT"], Region: values["STORAGE_S3_REGION"], Bucket: values["STORAGE_S3_BUCKET"],
			AccessKeyID: values["STORAGE_S3_ACCESS_KEY_ID"], SecretAccessKey: values["STORAGE_S3_SECRET_ACCESS_KEY"],
			ForcePathStyle: forcePathStyle, Prefix: values["STORAGE_S3_PREFIX"],
		},
	}
}

func Doctor(ctx context.Context, runtimeDir string, dependencies DoctorDependencies) DoctorReport {
	if ctx == nil {
		ctx = context.Background()
	}
	if dependencies.ReadFile == nil {
		dependencies.ReadFile = os.ReadFile
	}
	if dependencies.Stat == nil {
		dependencies.Stat = os.Stat
	}
	if dependencies.LookPath == nil {
		dependencies.LookPath = exec.LookPath
	}
	if dependencies.CheckWorkerTempDir == nil {
		dependencies.CheckWorkerTempDir = ProductionDoctorDependencies().CheckWorkerTempDir
	}
	runtimeDir = filepath.Clean(defaultString(runtimeDir, "."))
	envPath := filepath.Join(runtimeDir, "config", "runtime.env")
	manifestPath := filepath.Join(runtimeDir, "deployment.json")
	statePath := filepath.Join(runtimeDir, "config", "install-state.json")
	report := DoctorReport{Checks: make([]DoctorCheck, 0, 8)}

	content, err := dependencies.ReadFile(envPath)
	if err != nil {
		report.add("CONFIG_READ", false, sanitizeDiagnostic(err.Error(), nil))
		return report
	}
	document, err := config.ParseRuntimeEnv(content)
	if err != nil {
		report.add("CONFIG_PARSE", false, sanitizeDiagnostic(err.Error(), nil))
		return report
	}
	values := document.Values
	report.add("CONFIG_READ", true, "runtime configuration is readable")
	if info, statErr := dependencies.Stat(envPath); statErr != nil {
		report.add("CONFIG_PERMISSIONS", false, sanitizeDiagnostic(statErr.Error(), values))
	} else if info.Mode().Perm()&0o077 != 0 {
		report.add("CONFIG_PERMISSIONS", false, "runtime configuration is accessible by group or other users")
	} else {
		report.add("CONFIG_PERMISSIONS", true, "runtime configuration permissions are private")
	}

	deploymentContext, contextErr := deploymentContextFromValues(values)
	missing := requiredRuntimeFieldNames(values, deploymentContext, contextErr)
	if len(missing) > 0 {
		report.add("CONFIG_REQUIRED_FIELD", false, "missing or invalid required fields: "+strings.Join(missing, ", "))
	} else {
		report.add("CONFIG_REQUIRED_FIELD", true, "all required runtime fields are configured")
	}

	manifestID, manifestErr := readManifestInstallationID(dependencies.ReadFile, manifestPath)
	stateID, stateErr := readStateInstallationID(dependencies.ReadFile, statePath)
	runtimeID := strings.TrimSpace(values["INSTALLATION_ID"])
	identityOK := manifestErr == nil && stateErr == nil && runtimeID != "" && manifestID == runtimeID && stateID == runtimeID
	if identityOK {
		report.add("INSTALLATION_MISMATCH", true, "runtime, manifest, and install state identities match")
	} else {
		report.add("INSTALLATION_MISMATCH", false, "runtime, manifest, or install state identity does not match")
	}

	useDockerReadiness := values["DEPLOYMENT_MODE"] == string(config.DeploymentModeDocker) && runtimeHasModule(values, "api") && dependencies.CheckRuntimeReadiness != nil
	var runtimeReadinessErr error
	if useDockerReadiness {
		runtimeReadinessErr = dependencies.CheckRuntimeReadiness(ctx, cloneRuntimeValues(values))
	}
	if useDockerReadiness && runtimeReadinessErr != nil {
		report.add("MIDDLEWARE", false, sanitizeDiagnostic(runtimeReadinessErr.Error(), values))
	} else if useDockerReadiness {
		report.add("MIDDLEWARE", true, "Docker API readiness confirms middleware connectivity")
	} else if dependencies.ProbeMiddleware == nil {
		report.add("MIDDLEWARE", true, "middleware probe was not requested")
	} else if probeErr := dependencies.ProbeMiddleware(ctx, runtimeProbeValues(values, runtimeDir)); probeErr != nil {
		report.add("MIDDLEWARE", false, sanitizeDiagnostic(probeErr.Error(), values))
	} else {
		report.add("MIDDLEWARE", true, "middleware connectivity checks passed")
	}

	schemaVersion, versionErr := strconv.Atoi(values["RUNTIME_SCHEMA_VERSION"])
	schemaMessage := "runtime and database schemas are compatible"
	schemaOK := versionErr == nil && schemaVersion == config.CurrentRuntimeSchemaVersion
	if !schemaOK {
		schemaMessage = fmt.Sprintf("runtime schema version must be %d", config.CurrentRuntimeSchemaVersion)
	}
	if useDockerReadiness && runtimeReadinessErr != nil {
		schemaOK = false
		schemaMessage = sanitizeDiagnostic(runtimeReadinessErr.Error(), values)
	} else if useDockerReadiness {
		schemaMessage = "Docker API readiness confirms schema compatibility"
	} else if dependencies.CheckSchema != nil {
		if schemaErr := dependencies.CheckSchema(ctx, cloneRuntimeValues(values)); schemaErr != nil {
			schemaOK = false
			schemaMessage = sanitizeDiagnostic(schemaErr.Error(), values)
		}
	}
	report.add("SCHEMA_DRIFT", schemaOK, schemaMessage)

	workerEnabled := runtimeHasModule(values, "worker")
	mediaEnabled := workerEnabled && workerRoleEnabled(values["WORKER_ROLES"], "media")
	toolsOK := true
	toolsMessage := "media role is not enabled on this node"
	if mediaEnabled {
		missing := make([]string, 0, 2)
		ffmpeg := defaultString(strings.TrimSpace(values["MEDIA_FFMPEG_PATH"]), "ffmpeg")
		ffprobe := defaultString(strings.TrimSpace(values["MEDIA_FFPROBE_PATH"]), "ffprobe")
		if values["DEPLOYMENT_MODE"] == string(config.DeploymentModeDocker) {
			if dependencies.CheckDockerWorkerMediaTools == nil || dependencies.CheckDockerWorkerMediaTools(ctx, runtimeDir, cloneRuntimeValues(values), ffmpeg, ffprobe) != nil {
				missing = append(missing, "FFmpeg", "ffprobe")
			}
		} else {
			if _, err := dependencies.LookPath(ffmpeg); err != nil {
				missing = append(missing, "FFmpeg")
			}
			if _, err := dependencies.LookPath(ffprobe); err != nil {
				missing = append(missing, "ffprobe")
			}
		}
		toolsOK = len(missing) == 0
		if toolsOK {
			toolsMessage = "FFmpeg and ffprobe are available"
		} else {
			toolsMessage = "missing required media tools: " + strings.Join(missing, ", ")
		}
	}
	report.add("WORKER_MEDIA_TOOLS", toolsOK, toolsMessage)

	tempOK := true
	tempMessage := "media role is not enabled on this node"
	if mediaEnabled {
		tempDir := defaultString(strings.TrimSpace(values["MEDIA_TEMP_DIR"]), "./data/tmp")
		if !filepath.IsAbs(tempDir) {
			tempDir = filepath.Join(runtimeDir, tempDir)
		}
		if err := dependencies.CheckWorkerTempDir(tempDir); err != nil {
			tempOK = false
			tempMessage = "media temporary directory is unavailable or not writable"
		} else {
			tempMessage = "media temporary directory is writable"
		}
	}
	report.add("WORKER_TEMP_DIR", tempOK, tempMessage)
	return report
}

func workerRoleEnabled(raw, role string) bool {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return true
	}
	for _, configured := range strings.Split(raw, ",") {
		if strings.EqualFold(strings.TrimSpace(configured), role) {
			return true
		}
	}
	return false
}

func runtimeHasModule(values map[string]string, module string) bool {
	for _, configured := range strings.Split(values["DEPLOYMENT_MODULES"], ",") {
		if strings.TrimSpace(configured) == module {
			return true
		}
	}
	return false
}

func runtimeProbeValues(values map[string]string, runtimeDir string) map[string]string {
	cloned := cloneRuntimeValues(values)
	if cloned["STORAGE_DRIVER"] == "local" && cloned["STORAGE_LOCAL_ROOT"] != "" && !filepath.IsAbs(cloned["STORAGE_LOCAL_ROOT"]) {
		cloned["STORAGE_LOCAL_ROOT"] = filepath.Join(runtimeDir, cloned["STORAGE_LOCAL_ROOT"])
	}
	return cloned
}

func (report *DoctorReport) add(code string, ok bool, message string) {
	report.Checks = append(report.Checks, DoctorCheck{Code: code, OK: ok, Message: message})
}

func deploymentContextFromValues(values map[string]string) (config.DeploymentContext, error) {
	completed, err := strconv.ParseBool(values["SETUP_COMPLETED"])
	if err != nil {
		return config.DeploymentContext{}, fmt.Errorf("SETUP_COMPLETED is invalid")
	}
	deployment := config.DeploymentContext{
		Mode: config.DeploymentMode(values["DEPLOYMENT_MODE"]), Profile: config.DeploymentProfile(values["DEPLOYMENT_PROFILE"]),
		Topology: config.DeploymentTopology(values["DEPLOYMENT_TOPOLOGY"]), Role: config.DeploymentRole(values["DEPLOYMENT_ROLE"]),
		StorageDriver: values["STORAGE_DRIVER"], SetupCompleted: completed,
	}
	return deployment, config.ValidateDeploymentContext(deployment)
}

func requiredRuntimeFieldNames(values map[string]string, deployment config.DeploymentContext, contextErr error) []string {
	if contextErr != nil {
		return []string{"deployment metadata"}
	}
	required, err := config.RequiredRuntimeFields(config.DefaultRuntimeSchema(), deployment)
	if err != nil {
		return []string{"deployment metadata"}
	}
	missing := make([]string, 0)
	for _, field := range required {
		value := strings.TrimSpace(values[field.Key])
		if value == "" || field.Validate(value) != nil {
			missing = append(missing, field.Key)
		}
	}
	sort.Strings(missing)
	return missing
}

func readManifestInstallationID(readFile func(string) ([]byte, error), path string) (string, error) {
	content, err := readFile(path)
	if err != nil {
		return "", err
	}
	var manifest struct {
		SchemaVersion  int    `json:"schema_version"`
		InstallationID string `json:"installation_id"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil || manifest.SchemaVersion != 1 || strings.TrimSpace(manifest.InstallationID) == "" {
		return "", fmt.Errorf("deployment manifest is invalid")
	}
	return manifest.InstallationID, nil
}

func readStateInstallationID(readFile func(string) ([]byte, error), path string) (string, error) {
	content, err := readFile(path)
	if err != nil {
		return "", err
	}
	var state setup.InstallState
	if err := json.Unmarshal(content, &state); err != nil || state.Validate() != nil {
		return "", fmt.Errorf("install state is invalid")
	}
	return state.InstallationID, nil
}

func redactRuntimeError(err error, values map[string]string) error {
	if err == nil {
		return nil
	}
	return errors.New(sanitizeDiagnostic(err.Error(), values))
}

func sanitizeDiagnostic(message string, values map[string]string) string {
	replacements := make([]string, 0)
	secretKeys := make(map[string]struct{})
	for _, field := range config.DefaultRuntimeSchema().Fields {
		if field.Secret {
			secretKeys[field.Key] = struct{}{}
		}
	}
	for key, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, secret := secretKeys[key]; secret {
			replacements = append(replacements, value)
		}
		if strings.Contains(key, "URL") {
			replacements = append(replacements, value)
			if parsed, err := url.Parse(value); err == nil && parsed.User != nil {
				if password, exists := parsed.User.Password(); exists && password != "" {
					replacements = append(replacements, password)
				}
			}
		}
	}
	sort.Slice(replacements, func(i, j int) bool { return len(replacements[i]) > len(replacements[j]) })
	for _, secret := range replacements {
		message = strings.ReplaceAll(message, secret, "<redacted>")
	}
	message = redactURLLikeText(message)
	return message
}

func redactURLLikeText(message string) string {
	words := strings.Fields(message)
	for _, word := range words {
		trimmed := strings.Trim(word, "\"'(),;[]{}")
		parsed, err := url.Parse(trimmed)
		if err == nil && parsed.Scheme != "" && parsed.Host != "" && parsed.User != nil {
			message = strings.ReplaceAll(message, trimmed, "<redacted-url>")
		}
	}
	return message
}
