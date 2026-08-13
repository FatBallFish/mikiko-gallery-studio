package mgsctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
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
	DefaultMGSCTLReleaseBaseURL = "https://github.com/fatballfish/mikiko-gallery-studio/releases"
	maxMGSCTLBinarySize         = 128 << 20
	maxMGSCTLChecksumSize       = 4 << 10
	selfUpdateMaxAttempts       = 3
	selfUpdateBodyIdleTimeout   = 60 * time.Second
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
	Progress       func(SelfUpdateProgress)
	Retry          func(SelfUpdateRetry)
}

type SelfUpdateProgress struct {
	Stage      string
	Attempt    int
	Downloaded int64
	Total      int64
	Elapsed    time.Duration
	Done       bool
}

type SelfUpdateRetry struct {
	Stage       string
	Attempt     int
	MaxAttempts int
	Err         error
}

type SelfUpdateResult struct {
	PreviousVersion string
	CurrentVersion  string
	Executable      string
	StagedPath      string
	Deferred        bool
}

func ProductionSelfUpdateDependencies() SelfUpdateDependencies {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	transport.TLSHandshakeTimeout = 15 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second
	transport.ExpectContinueTimeout = time.Second
	return SelfUpdateDependencies{
		HTTPClient: &http.Client{Transport: transport}, ExecutablePath: os.Executable,
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
		checksum, _, downloadErr := downloadBytesWithRetry(
			ctx, dependencies.HTTPClient, downloadURL+".sha256", maxMGSCTLChecksumSize,
			"checksum", dependencies.Progress, dependencies.Retry,
		)
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

	actual, finalURL, err := downloadFileWithRetry(
		ctx, dependencies.HTTPClient, downloadURL, staged, maxMGSCTLBinarySize,
		"binary", dependencies.Progress, dependencies.Retry,
	)
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
	return downloadBytesAttempt(ctx, client, downloadURL, limit, nil)
}

func downloadBytesWithRetry(
	ctx context.Context,
	client *http.Client,
	downloadURL string,
	limit int64,
	stage string,
	progress func(SelfUpdateProgress),
	retry func(SelfUpdateRetry),
) ([]byte, string, error) {
	var lastErr error
	for attempt := 1; attempt <= selfUpdateMaxAttempts; attempt++ {
		started := time.Now()
		content, finalURL, err := downloadBytesAttempt(ctx, client, downloadURL, limit, func(downloaded, total int64, done bool) {
			reportSelfUpdateProgress(progress, stage, attempt, downloaded, total, time.Since(started), done)
		})
		if err == nil {
			return content, finalURL, nil
		}
		lastErr = err
		if !shouldRetryDownload(ctx, err) || attempt == selfUpdateMaxAttempts {
			return nil, "", newDownloadStageError(stage, attempt, err)
		}
		reportSelfUpdateRetry(retry, stage, attempt, err)
		if err := waitForSelfUpdateRetry(ctx, attempt); err != nil {
			return nil, "", newDownloadStageError(stage, attempt, err)
		}
	}
	return nil, "", newDownloadStageError(stage, selfUpdateMaxAttempts, lastErr)
}

func downloadBytesAttempt(
	ctx context.Context,
	client *http.Client,
	downloadURL string,
	limit int64,
	progress func(downloaded, total int64, done bool),
) ([]byte, string, error) {
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
		return nil, "", &downloadHTTPError{StatusCode: response.StatusCode}
	}
	content, err := readAllWithIdleTimeout(response.Body, selfUpdateBodyIdleTimeout, limit, response.ContentLength, progress)
	if err != nil {
		return nil, "", err
	}
	if int64(len(content)) > limit {
		return nil, "", fmt.Errorf("download exceeds %d bytes", limit)
	}
	return content, response.Request.URL.String(), nil
}

func downloadFile(ctx context.Context, client *http.Client, downloadURL string, destination io.Writer, limit int64) (string, string, error) {
	return downloadFileAttempt(ctx, client, downloadURL, destination, limit, nil)
}

func downloadFileWithRetry(
	ctx context.Context,
	client *http.Client,
	downloadURL string,
	destination *os.File,
	limit int64,
	stage string,
	progress func(SelfUpdateProgress),
	retry func(SelfUpdateRetry),
) (string, string, error) {
	var lastErr error
	for attempt := 1; attempt <= selfUpdateMaxAttempts; attempt++ {
		if err := resetStagedDownload(destination); err != nil {
			return "", "", fmt.Errorf("reset staged download: %w", err)
		}
		started := time.Now()
		digest, finalURL, err := downloadFileAttempt(ctx, client, downloadURL, destination, limit, func(downloaded, total int64, done bool) {
			reportSelfUpdateProgress(progress, stage, attempt, downloaded, total, time.Since(started), done)
		})
		if err == nil {
			return digest, finalURL, nil
		}
		lastErr = err
		if !shouldRetryDownload(ctx, err) || attempt == selfUpdateMaxAttempts {
			return "", "", newDownloadStageError(stage, attempt, err)
		}
		reportSelfUpdateRetry(retry, stage, attempt, err)
		if err := waitForSelfUpdateRetry(ctx, attempt); err != nil {
			return "", "", newDownloadStageError(stage, attempt, err)
		}
	}
	return "", "", newDownloadStageError(stage, selfUpdateMaxAttempts, lastErr)
}

func downloadFileAttempt(
	ctx context.Context,
	client *http.Client,
	downloadURL string,
	destination io.Writer,
	limit int64,
	progress func(downloaded, total int64, done bool),
) (string, string, error) {
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
		return "", "", &downloadHTTPError{StatusCode: response.StatusCode}
	}
	hash := sha256.New()
	written, err := copyWithIdleTimeout(
		response.Body, io.MultiWriter(destination, hash), selfUpdateBodyIdleTimeout,
		limit, response.ContentLength, progress,
	)
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
		return fmt.Errorf("%w: %w: %v; %s", errMGSCTLReleaseUnavailable, context.Canceled, err, guidance)
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Errorf("%w: %w: %v; %s", errMGSCTLReleaseUnavailable, context.DeadlineExceeded, err, guidance)
	}
	return fmt.Errorf("%w: %v; %s", errMGSCTLReleaseUnavailable, err, guidance)
}

type downloadHTTPError struct{ StatusCode int }

func (e *downloadHTTPError) Error() string { return fmt.Sprintf("HTTP %d", e.StatusCode) }

type downloadStageError struct {
	stage   string
	attempt int
	cause   error
}

func (e *downloadStageError) Error() string {
	return fmt.Sprintf("%s download failed on attempt %d/%d: %s", e.stage, e.attempt, selfUpdateMaxAttempts, sanitizeDownloadError(e.cause))
}

func (e *downloadStageError) Unwrap() error { return e.cause }

func newDownloadStageError(stage string, attempt int, cause error) error {
	return &downloadStageError{stage: stage, attempt: attempt, cause: cause}
}

func shouldRetryDownload(ctx context.Context, err error) bool {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) {
		return false
	}
	var httpErr *downloadHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
			return true
		default:
			return false
		}
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}

func waitForSelfUpdateRetry(ctx context.Context, attempt int) error {
	timer := time.NewTimer(time.Duration(attempt) * 100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func resetStagedDownload(file *os.File) error {
	if err := file.Truncate(0); err != nil {
		return err
	}
	_, err := file.Seek(0, io.SeekStart)
	return err
}

func reportSelfUpdateProgress(callback func(SelfUpdateProgress), stage string, attempt int, downloaded, total int64, elapsed time.Duration, done bool) {
	if callback != nil {
		callback(SelfUpdateProgress{Stage: stage, Attempt: attempt, Downloaded: downloaded, Total: total, Elapsed: elapsed, Done: done})
	}
}

func reportSelfUpdateRetry(callback func(SelfUpdateRetry), stage string, attempt int, err error) {
	if callback != nil {
		callback(SelfUpdateRetry{Stage: stage, Attempt: attempt, MaxAttempts: selfUpdateMaxAttempts, Err: errors.New(sanitizeDownloadError(err))})
	}
}

func readAllWithIdleTimeout(
	body io.ReadCloser,
	idleTimeout time.Duration,
	limit int64,
	total int64,
	progress func(downloaded, total int64, done bool),
) ([]byte, error) {
	var destination bytes.Buffer
	if _, err := copyWithIdleTimeout(body, &destination, idleTimeout, limit, total, progress); err != nil {
		return nil, err
	}
	return destination.Bytes(), nil
}

func copyWithIdleTimeout(
	body io.ReadCloser,
	destination io.Writer,
	idleTimeout time.Duration,
	limit int64,
	total int64,
	progress func(downloaded, total int64, done bool),
) (int64, error) {
	if total < 0 {
		total = -1
	}
	type copyResult struct {
		written int64
		err     error
	}
	activity := make(chan struct{}, 1)
	resultChannel := make(chan copyResult, 1)
	limited := &io.LimitedReader{R: body, N: limit + 1}
	reader := &downloadProgressReader{
		reader: limited,
		total:  total,
		callback: func(downloaded, total int64, done bool) {
			select {
			case activity <- struct{}{}:
			default:
			}
			if progress != nil {
				progress(downloaded, total, done)
			}
		},
	}
	go func() {
		written, err := io.Copy(destination, reader)
		resultChannel <- copyResult{written: written, err: err}
	}()

	timer := time.NewTimer(idleTimeout)
	defer timer.Stop()
	for {
		select {
		case result := <-resultChannel:
			if result.err == nil {
				reader.finish()
			}
			return result.written, result.err
		case <-activity:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(idleTimeout)
		case <-timer.C:
			_ = body.Close()
			result := <-resultChannel
			return result.written, &downloadIdleTimeoutError{timeout: idleTimeout}
		}
	}
}

type downloadProgressReader struct {
	reader     io.Reader
	total      int64
	downloaded int64
	callback   func(downloaded, total int64, done bool)
}

func (r *downloadProgressReader) Read(buffer []byte) (int, error) {
	count, err := r.reader.Read(buffer)
	if count > 0 {
		r.downloaded += int64(count)
		if r.callback != nil {
			r.callback(r.downloaded, r.total, false)
		}
	}
	return count, err
}

func (r *downloadProgressReader) finish() {
	if r.callback != nil {
		r.callback(r.downloaded, r.total, true)
	}
}

type downloadIdleTimeoutError struct{ timeout time.Duration }

func (e *downloadIdleTimeoutError) Error() string {
	return fmt.Sprintf("download body made no progress for %s", e.timeout)
}

func (e *downloadIdleTimeoutError) Timeout() bool   { return true }
func (e *downloadIdleTimeoutError) Temporary() bool { return true }

func sanitizeDownloadError(err error) string {
	if err == nil {
		return "unknown error"
	}
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		operation := strings.TrimSpace(urlErr.Op)
		if operation == "" {
			operation = "request"
		}
		return operation + ": " + sanitizeDownloadError(urlErr.Err)
	}
	return err.Error()
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
