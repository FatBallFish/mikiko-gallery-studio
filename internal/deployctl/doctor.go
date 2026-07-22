package deployctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

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
	ReadFile        func(string) ([]byte, error)
	Stat            func(string) (os.FileInfo, error)
	ProbeMiddleware func(context.Context, map[string]string) error
	CheckSchema     func(context.Context, map[string]string) error
}

func ProductionDoctorDependencies() DoctorDependencies {
	return DoctorDependencies{
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

	if dependencies.ProbeMiddleware == nil {
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
	if dependencies.CheckSchema != nil {
		if schemaErr := dependencies.CheckSchema(ctx, cloneRuntimeValues(values)); schemaErr != nil {
			schemaOK = false
			schemaMessage = sanitizeDiagnostic(schemaErr.Error(), values)
		}
	}
	report.add("SCHEMA_DRIFT", schemaOK, schemaMessage)
	return report
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
