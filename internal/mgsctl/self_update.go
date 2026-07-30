package mgsctl

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultMGSCTLReleaseBaseURL = "https://github.com/fatballfish/pic-gallery/releases"
	maxMGSCTLBinarySize         = 128 << 20
	maxMGSCTLChecksumSize       = 4 << 10
)

var errMGSCTLReleaseUnavailable = errors.New("mgsctl release artifact is unavailable")

type SelfUpdateOptions struct {
	Version        string
	ReleaseBaseURL string
	DownloadURL    string
	ExpectedSHA256 string
	CurrentVersion string
	Yes            bool
}

type SelfUpdateDependencies struct {
	HTTPClient     *http.Client
	ExecutablePath func() (string, error)
	GOOS           string
	GOARCH         string
	Replace        func(current, staged string) (deferred bool, err error)
}

type SelfUpdateResult struct {
	PreviousVersion string
	CurrentVersion  string
	Executable      string
	StagedPath      string
	Deferred        bool
}

func ProductionSelfUpdateDependencies() SelfUpdateDependencies {
	return SelfUpdateDependencies{
		HTTPClient: &http.Client{Timeout: 2 * time.Minute}, ExecutablePath: os.Executable,
		GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Replace: replaceMGSCTLExecutable,
	}
}

func ResolveMGSCTLArtifact(options SelfUpdateOptions, goos, goarch string) (string, string, error) {
	if err := validateSelfUpdateOptions(options); err != nil {
		return "", "", err
	}
	if !supportedMGSCTLPlatform(goos, goarch) {
		return "", "", fmt.Errorf("mgsctl self-update does not support %s/%s", goos, goarch)
	}
	artifact := "mgsctl-" + goos + "-" + goarch
	if goos == "windows" {
		artifact += ".exe"
	}
	if options.DownloadURL != "" {
		return artifact, options.DownloadURL, nil
	}
	releasePath := "latest/download"
	if options.Version != "latest" {
		releasePath = "download/" + options.Version
	}
	return artifact, strings.TrimRight(options.ReleaseBaseURL, "/") + "/" + releasePath + "/" + artifact, nil
}

func SelfUpdate(ctx context.Context, options SelfUpdateOptions, dependencies SelfUpdateDependencies) (SelfUpdateResult, error) {
	dependencies = normalizeSelfUpdateDependencies(dependencies)
	_, downloadURL, err := ResolveMGSCTLArtifact(options, dependencies.GOOS, dependencies.GOARCH)
	if err != nil {
		return SelfUpdateResult{}, err
	}
	executable, err := dependencies.ExecutablePath()
	if err != nil {
		return SelfUpdateResult{}, fmt.Errorf("locate current mgsctl executable: %w", err)
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return SelfUpdateResult{}, fmt.Errorf("resolve current mgsctl executable path: %w", err)
	}
	info, err := os.Stat(executable)
	if err != nil {
		return SelfUpdateResult{}, fmt.Errorf("inspect current mgsctl executable: %w", err)
	}
	if !info.Mode().IsRegular() {
		return SelfUpdateResult{}, fmt.Errorf("current mgsctl executable is not a regular file: %s", executable)
	}

	expected := strings.ToLower(strings.TrimSpace(options.ExpectedSHA256))
	if expected == "" {
		checksum, _, downloadErr := downloadBytes(ctx, dependencies.HTTPClient, downloadURL+".sha256", maxMGSCTLChecksumSize)
		if downloadErr != nil {
			return SelfUpdateResult{}, releaseUnavailableError(downloadErr)
		}
		expected, err = parseMGSCTLChecksum(checksum)
		if err != nil {
			return SelfUpdateResult{}, fmt.Errorf("parse mgsctl release checksum: %w", err)
		}
	}

	staged, err := os.CreateTemp(filepath.Dir(executable), "."+filepath.Base(executable)+".update-*")
	if err != nil {
		return SelfUpdateResult{}, fmt.Errorf("stage mgsctl update beside current executable: %w", err)
	}
	stagedPath := staged.Name()
	keepStaged := false
	defer func() {
		_ = staged.Close()
		if !keepStaged {
			_ = os.Remove(stagedPath)
		}
	}()

	actual, finalURL, err := downloadFile(ctx, dependencies.HTTPClient, downloadURL, staged, maxMGSCTLBinarySize)
	if err != nil {
		return SelfUpdateResult{}, releaseUnavailableError(err)
	}
	if actual != expected {
		return SelfUpdateResult{}, fmt.Errorf("mgsctl checksum verification failed: expected %s, got %s", expected, actual)
	}
	if err := staged.Chmod(info.Mode().Perm()); err != nil {
		return SelfUpdateResult{}, fmt.Errorf("apply mgsctl executable permissions: %w", err)
	}
	if err := staged.Sync(); err != nil {
		return SelfUpdateResult{}, fmt.Errorf("sync staged mgsctl executable: %w", err)
	}
	if err := staged.Close(); err != nil {
		return SelfUpdateResult{}, fmt.Errorf("close staged mgsctl executable: %w", err)
	}

	deferred, err := dependencies.Replace(executable, stagedPath)
	if err != nil {
		keepStaged = true
		return SelfUpdateResult{}, fmt.Errorf("replace mgsctl executable; verified update retained at %s: %w", stagedPath, err)
	}
	keepStaged = deferred
	targetVersion := options.Version
	if targetVersion == "latest" {
		targetVersion = releaseVersionFromURL(finalURL)
	}
	return SelfUpdateResult{
		PreviousVersion: defaultBuildValue(options.CurrentVersion, "unknown"),
		CurrentVersion:  defaultBuildValue(targetVersion, "latest"),
		Executable:      executable,
		StagedPath:      stagedPath,
		Deferred:        deferred,
	}, nil
}

func normalizeSelfUpdateDependencies(dependencies SelfUpdateDependencies) SelfUpdateDependencies {
	production := ProductionSelfUpdateDependencies()
	if dependencies.HTTPClient == nil {
		dependencies.HTTPClient = production.HTTPClient
	}
	if dependencies.ExecutablePath == nil {
		dependencies.ExecutablePath = production.ExecutablePath
	}
	if dependencies.GOOS == "" {
		dependencies.GOOS = production.GOOS
	}
	if dependencies.GOARCH == "" {
		dependencies.GOARCH = production.GOARCH
	}
	if dependencies.Replace == nil {
		dependencies.Replace = production.Replace
	}
	return dependencies
}

func validateSelfUpdateOptions(options SelfUpdateOptions) error {
	version := strings.TrimSpace(options.Version)
	if version == "" {
		return fmt.Errorf("self-update version is required")
	}
	for _, character := range version {
		if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') || character == '.' || character == '_' || character == '-' {
			continue
		}
		return fmt.Errorf("self-update version contains an unsupported character")
	}
	if err := validateMGSCTLDownloadURL(options.ReleaseBaseURL, true); err != nil {
		return fmt.Errorf("self-update release base URL: %w", err)
	}
	if options.DownloadURL != "" {
		if err := validateMGSCTLDownloadURL(options.DownloadURL, false); err != nil {
			return fmt.Errorf("self-update download URL: %w", err)
		}
	}
	if options.ExpectedSHA256 != "" {
		if _, err := hex.DecodeString(options.ExpectedSHA256); err != nil || len(options.ExpectedSHA256) != sha256.Size*2 {
			return fmt.Errorf("self-update SHA-256 must contain exactly 64 hexadecimal characters")
		}
	}
	return nil
}

func validateMGSCTLDownloadURL(value string, base bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return fmt.Errorf("must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil {
		return fmt.Errorf("must not contain credentials")
	}
	if parsed.Fragment != "" || (base && parsed.RawQuery != "") {
		return fmt.Errorf("must not contain %s", map[bool]string{true: "a query or fragment", false: "a fragment"}[base])
	}
	return nil
}

func supportedMGSCTLPlatform(goos, goarch string) bool {
	return (goos == "linux" || goos == "darwin" || goos == "windows") && (goarch == "amd64" || goarch == "arm64")
}

func parseMGSCTLChecksum(content []byte) (string, error) {
	fields := strings.Fields(string(content))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return "", fmt.Errorf("checksum file does not start with a SHA-256 digest")
	}
	digest := strings.ToLower(fields[0])
	if _, err := hex.DecodeString(digest); err != nil {
		return "", fmt.Errorf("checksum file contains an invalid SHA-256 digest")
	}
	return digest, nil
}

func downloadBytes(ctx context.Context, client *http.Client, downloadURL string, limit int64) ([]byte, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	limited := &io.LimitedReader{R: response.Body, N: limit + 1}
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", err
	}
	if int64(len(content)) > limit {
		return nil, "", fmt.Errorf("download exceeds %d bytes", limit)
	}
	return content, response.Request.URL.String(), nil
}

func downloadFile(ctx context.Context, client *http.Client, downloadURL string, destination io.Writer, limit int64) (string, string, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return "", "", err
	}
	response, err := client.Do(request)
	if err != nil {
		return "", "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", "", fmt.Errorf("HTTP %d", response.StatusCode)
	}
	hash := sha256.New()
	limited := &io.LimitedReader{R: response.Body, N: limit + 1}
	written, err := io.Copy(io.MultiWriter(destination, hash), limited)
	if err != nil {
		return "", "", err
	}
	if written > limit {
		return "", "", fmt.Errorf("download exceeds %d bytes", limit)
	}
	return hex.EncodeToString(hash.Sum(nil)), response.Request.URL.String(), nil
}

func releaseUnavailableError(err error) error {
	guidance := "rerun scripts/install.sh or scripts/install.ps1 from a complete source checkout to use the local build fallback"
	if errors.Is(err, context.Canceled) {
		return fmt.Errorf("%w: %w; %s", errMGSCTLReleaseUnavailable, context.Canceled, guidance)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w; %s", errMGSCTLReleaseUnavailable, context.DeadlineExceeded, guidance)
	}
	return fmt.Errorf("%w: download failed; %s", errMGSCTLReleaseUnavailable, guidance)
}

func releaseVersionFromURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return "latest"
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for index := 0; index+1 < len(parts); index++ {
		if parts[index] == "download" {
			return defaultBuildValue(parts[index+1], "latest")
		}
	}
	return "latest"
}

func windowsSelfUpdateScript(current, staged string, pid int) string {
	errorLog := staged + ".update-error.log"
	return "$ErrorActionPreference = 'Stop'; try { " +
		"Wait-Process -Id " + strconv.Itoa(pid) + " -ErrorAction SilentlyContinue; " +
		"Move-Item -LiteralPath " + powershellSingleQuoted(staged) + " -Destination " + powershellSingleQuoted(current) + " -Force " +
		"} catch { ($_ | Out-String) | Set-Content -LiteralPath " + powershellSingleQuoted(errorLog) + "; exit 1 }"
}

func powershellSingleQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}
