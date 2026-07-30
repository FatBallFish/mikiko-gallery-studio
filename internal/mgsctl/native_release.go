package mgsctl

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
)

const (
	defaultNativeReleaseBaseURL = "https://github.com/fatballfish/mikiko-gallery-studio/releases"
	maxNativeReleaseBytes       = int64(1 << 30)
	maxNativeExtractedBytes     = int64(2 << 30)
	nativeReleaseJournalSchema  = 1
)

type NativeReleaseInstaller struct {
	Client       *http.Client
	BaseURL      string
	Architecture string
	Rename       func(string, string) error
}

type nativeReleaseJournal struct {
	SchemaVersion         int               `json:"schema_version"`
	ArchiveSHA256         string            `json:"archive_sha256"`
	Files                 map[string]string `json:"files"`
	PreviousArchiveSHA256 string            `json:"previous_archive_sha256,omitempty"`
	PreviousFiles         map[string]string `json:"previous_files,omitempty"`
}

func InstallNativeRelease(ctx context.Context, plan InstallPlan, platform NativePlatform) error {
	baseURL := strings.TrimSpace(os.Getenv("MGSCTL_RELEASE_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultNativeReleaseBaseURL
	}
	return (NativeReleaseInstaller{
		Client: &http.Client{Timeout: 5 * time.Minute}, BaseURL: baseURL, Architecture: runtime.GOARCH,
	}).Install(ctx, plan, platform)
}

func StageNativeReleaseMigration(ctx context.Context, plan InstallPlan, platform NativePlatform) (string, func() error, error) {
	baseURL := strings.TrimSpace(os.Getenv("MGSCTL_RELEASE_BASE_URL"))
	if baseURL == "" {
		baseURL = defaultNativeReleaseBaseURL
	}
	return (NativeReleaseInstaller{
		Client: &http.Client{Timeout: 5 * time.Minute}, BaseURL: baseURL, Architecture: runtime.GOARCH,
	}).StageMigration(ctx, plan, platform)
}

func (installer NativeReleaseInstaller) StageMigration(ctx context.Context, plan InstallPlan, platform NativePlatform) (string, func() error, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ValidateInstallPlan(plan); err != nil {
		return "", nil, fmt.Errorf("validate native release plan: %w", err)
	}
	if plan.Mode != config.DeploymentModeNative {
		return "", nil, fmt.Errorf("native release cannot stage deployment mode %q", plan.Mode)
	}
	if platform != NativePlatformLinux && platform != NativePlatformWindows {
		return "", nil, fmt.Errorf("unsupported native release platform %q", platform)
	}
	if installer.Client == nil {
		installer.Client = &http.Client{Timeout: 5 * time.Minute}
	}
	if installer.Architecture != "amd64" && installer.Architecture != "arm64" {
		return "", nil, fmt.Errorf("unsupported native release architecture %q", installer.Architecture)
	}
	baseURL, err := validateNativeReleaseBaseURL(installer.BaseURL)
	if err != nil {
		return "", nil, err
	}
	archive, err := installer.downloadVerifiedArchive(ctx, plan.ReleaseVersion, platform, baseURL)
	if err != nil {
		return "", nil, err
	}
	stageDirectory, err := os.MkdirTemp("", "mgsctl-native-upgrade-")
	if err != nil {
		return "", nil, fmt.Errorf("create native upgrade stage: %w", err)
	}
	cleanup := func() error { return os.RemoveAll(stageDirectory) }
	if err := extractNativeRelease(archive, stageDirectory); err != nil {
		_ = cleanup()
		return "", nil, err
	}
	if err := validateNativeReleaseFiles(stageDirectory, plan, platform); err != nil {
		_ = cleanup()
		return "", nil, err
	}
	extension := ""
	if platform == NativePlatformWindows {
		extension = ".exe"
	}
	executable := filepath.Join(stageDirectory, "bin", "mikiko-gallery-studio-db-migrate"+extension)
	return executable, cleanup, nil
}

func (installer NativeReleaseInstaller) downloadVerifiedArchive(ctx context.Context, releaseVersion string, platform NativePlatform, baseURL string) ([]byte, error) {
	artifactName := "mikiko-gallery-studio-native-" + string(platform) + "-" + installer.Architecture + ".tar.gz"
	releasePath := "download/" + url.PathEscape(releaseVersion)
	if releaseVersion == "latest" {
		releasePath = "latest/download"
	}
	artifactURL := baseURL + "/" + releasePath + "/" + artifactName
	checksumContent, err := downloadNativeRelease(ctx, installer.Client, artifactURL+".sha256", 4096)
	if err != nil {
		return nil, fmt.Errorf("download native release checksum: %w", err)
	}
	expectedDigest, err := parseNativeReleaseChecksum(checksumContent)
	if err != nil {
		return nil, err
	}
	archive, err := downloadNativeRelease(ctx, installer.Client, artifactURL, maxNativeReleaseBytes)
	if err != nil {
		return nil, fmt.Errorf("download native release: %w", err)
	}
	actualDigest := sha256.Sum256(archive)
	if !slices.Equal(actualDigest[:], expectedDigest) {
		return nil, fmt.Errorf("native release checksum mismatch")
	}
	return archive, nil
}

func (installer NativeReleaseInstaller) Install(ctx context.Context, plan InstallPlan, platform NativePlatform) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ValidateInstallPlan(plan); err != nil {
		return fmt.Errorf("validate native release plan: %w", err)
	}
	if plan.Mode != config.DeploymentModeNative {
		return fmt.Errorf("native release cannot install deployment mode %q", plan.Mode)
	}
	if platform != NativePlatformLinux && platform != NativePlatformWindows {
		return fmt.Errorf("unsupported native release platform %q", platform)
	}
	if installer.Client == nil {
		installer.Client = &http.Client{Timeout: 5 * time.Minute}
	}
	if installer.Architecture != "amd64" && installer.Architecture != "arm64" {
		return fmt.Errorf("unsupported native release architecture %q", installer.Architecture)
	}
	if installer.Rename == nil {
		installer.Rename = os.Rename
	}
	baseURL, err := validateNativeReleaseBaseURL(installer.BaseURL)
	if err != nil {
		return err
	}
	archive, err := installer.downloadVerifiedArchive(ctx, plan.ReleaseVersion, platform, baseURL)
	if err != nil {
		return err
	}
	actualDigest := sha256.Sum256(archive)
	if err := os.MkdirAll(plan.RuntimeDir, 0o700); err != nil {
		return fmt.Errorf("create native runtime directory: %w", err)
	}
	if err := secureInstallDirectory(plan.RuntimeDir); err != nil {
		return fmt.Errorf("secure native runtime directory: %w", err)
	}
	if err := cleanupNativeReleaseStages(plan.RuntimeDir); err != nil {
		return err
	}
	digestHex := hex.EncodeToString(actualDigest[:])
	markerPath := filepath.Join(plan.RuntimeDir, ".native-release.sha256")
	manifestPath := filepath.Join(plan.RuntimeDir, ".native-release.manifest.json")
	pendingPath := filepath.Join(plan.RuntimeDir, ".native-release.pending.json")
	markerDigest, markerExists, err := readNativeReleaseMarker(markerPath)
	if err != nil {
		return err
	}
	journal, journalExists, err := readNativeReleaseJournal(pendingPath)
	if err != nil {
		return err
	}
	if markerExists && markerDigest == digestHex {
		if err := validateNativeReleaseFiles(plan.RuntimeDir, plan, platform); err != nil {
			return err
		}
		manifest, exists, err := readNativeReleaseJournal(manifestPath)
		if err != nil {
			return fmt.Errorf("read installed native release manifest: %w", err)
		}
		if !exists || manifest.ArchiveSHA256 != digestHex || manifest.PreviousArchiveSHA256 != "" || len(manifest.PreviousFiles) != 0 {
			return fmt.Errorf("installed native release manifest does not match its checksum marker")
		}
		if err := validateNativeReleaseTree(plan.RuntimeDir, manifest.Files); err != nil {
			return fmt.Errorf("validate installed native release manifest: %w", err)
		}
		if journalExists {
			if journal.ArchiveSHA256 != digestHex {
				return fmt.Errorf("completed release marker does not match pending release journal")
			}
			if err := removeNativeReleaseBackups(plan.RuntimeDir, journal.PreviousFiles); err != nil {
				return err
			}
		}
		if err := os.Remove(pendingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove completed native release journal: %w", err)
		}
		return nil
	}
	var previousManifest nativeReleaseJournal
	if journalExists {
		if journal.ArchiveSHA256 != digestHex {
			return fmt.Errorf("pending release journal belongs to a different archive; refusing to remove existing release targets")
		}
		if err := recoverPendingNativeRelease(plan.RuntimeDir, journal, installer.Rename); err != nil {
			return err
		}
		previousManifest = nativeReleaseJournal{ArchiveSHA256: journal.PreviousArchiveSHA256, Files: journal.PreviousFiles}
	} else if markerExists {
		manifest, exists, err := readNativeReleaseJournal(manifestPath)
		if err != nil {
			return fmt.Errorf("read installed native release manifest: %w", err)
		}
		if !exists || manifest.ArchiveSHA256 != markerDigest || manifest.PreviousArchiveSHA256 != "" || len(manifest.PreviousFiles) != 0 {
			return fmt.Errorf("installed native release manifest does not match its checksum marker")
		}
		if err := validateNativeReleaseTree(plan.RuntimeDir, manifest.Files); err != nil {
			return fmt.Errorf("validate installed native release before update: %w", err)
		}
		if err := rejectNativeReleaseBackups(plan.RuntimeDir); err != nil {
			return err
		}
		previousManifest = manifest
	} else {
		if _, manifestExists, err := readNativeReleaseJournal(manifestPath); err != nil {
			return fmt.Errorf("read installed native release manifest: %w", err)
		} else if manifestExists {
			return fmt.Errorf("native release manifest exists without a checksum marker or recovery journal")
		}
		if err := rejectNativeReleaseBackups(plan.RuntimeDir); err != nil {
			return err
		}
		for _, name := range []string{"bin", "web", "api"} {
			if _, err := os.Lstat(filepath.Join(plan.RuntimeDir, name)); err == nil {
				return fmt.Errorf("native release target %s already exists without a matching checksum marker", name)
			} else if !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("inspect native release target %s: %w", name, err)
			}
		}
	}
	stageDirectory, err := os.MkdirTemp(plan.RuntimeDir, ".native-release-stage-")
	if err != nil {
		return fmt.Errorf("create native release stage: %w", err)
	}
	defer os.RemoveAll(stageDirectory)
	if err := extractNativeRelease(archive, stageDirectory); err != nil {
		return err
	}
	if err := validateNativeReleaseFiles(stageDirectory, plan, platform); err != nil {
		return err
	}
	stageHashes, err := hashNativeReleaseFiles(stageDirectory)
	if err != nil {
		return fmt.Errorf("hash staged native release: %w", err)
	}
	if journalExists {
		if !equalStringMaps(stageHashes, journal.Files) {
			return fmt.Errorf("staged native release does not match pending release journal")
		}
	} else {
		journal = nativeReleaseJournal{
			SchemaVersion: nativeReleaseJournalSchema, ArchiveSHA256: digestHex, Files: stageHashes,
			PreviousArchiveSHA256: previousManifest.ArchiveSHA256, PreviousFiles: previousManifest.Files,
		}
		if err := writeNativeReleaseJournal(pendingPath, journal); err != nil {
			return fmt.Errorf("write native release journal: %w", err)
		}
	}
	for _, name := range []string{"bin", "web", "api"} {
		source := filepath.Join(stageDirectory, name)
		sourceExists, err := nativeReleaseDirectoryExists(source)
		if err != nil {
			return fmt.Errorf("inspect staged native release %s: %w", name, err)
		}
		target := filepath.Join(plan.RuntimeDir, name)
		if journal.PreviousArchiveSHA256 != "" {
			targetExists, err := nativeReleaseDirectoryExists(target)
			if err != nil {
				return fmt.Errorf("inspect installed native release %s: %w", name, err)
			}
			if targetExists {
				if err := installer.Rename(target, nativeReleaseBackupPath(plan.RuntimeDir, name)); err != nil {
					return fmt.Errorf("back up installed native release %s: %w", name, err)
				}
			}
		}
		if sourceExists {
			if err := installer.Rename(source, target); err != nil {
				return fmt.Errorf("publish native release %s: %w", name, err)
			}
		}
	}
	installedManifest := nativeReleaseJournal{SchemaVersion: nativeReleaseJournalSchema, ArchiveSHA256: digestHex, Files: stageHashes}
	if err := writeNativeReleaseJournal(manifestPath, installedManifest); err != nil {
		return fmt.Errorf("write installed native release manifest: %w", err)
	}
	if err := writeNativeServiceFile(markerPath, []byte(digestHex+"\n")); err != nil {
		return fmt.Errorf("write native release marker: %w", err)
	}
	if err := removeNativeReleaseBackups(plan.RuntimeDir, journal.PreviousFiles); err != nil {
		return err
	}
	if err := os.Remove(pendingPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove completed native release journal: %w", err)
	}
	return nil
}

func readNativeReleaseJournal(journalPath string) (nativeReleaseJournal, bool, error) {
	content, err := os.ReadFile(journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nativeReleaseJournal{}, false, nil
	}
	if err != nil {
		return nativeReleaseJournal{}, false, fmt.Errorf("read pending release journal: %w", err)
	}
	var journal nativeReleaseJournal
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&journal); err != nil {
		return nativeReleaseJournal{}, false, fmt.Errorf("parse pending release journal: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nativeReleaseJournal{}, false, fmt.Errorf("pending release journal contains trailing data")
	}
	if journal.SchemaVersion != nativeReleaseJournalSchema || len(journal.Files) == 0 || !validNativeReleaseDigest(journal.ArchiveSHA256) {
		return nativeReleaseJournal{}, false, fmt.Errorf("pending release journal is invalid")
	}
	if err := validateNativeReleaseHashes(journal.Files); err != nil {
		return nativeReleaseJournal{}, false, fmt.Errorf("pending release journal is invalid")
	}
	hasPreviousDigest := journal.PreviousArchiveSHA256 != ""
	hasPreviousFiles := len(journal.PreviousFiles) != 0
	if hasPreviousDigest != hasPreviousFiles || hasPreviousDigest && (!validNativeReleaseDigest(journal.PreviousArchiveSHA256) || validateNativeReleaseHashes(journal.PreviousFiles) != nil) {
		return nativeReleaseJournal{}, false, fmt.Errorf("pending release journal has invalid previous release metadata")
	}
	return journal, true, nil
}

func writeNativeReleaseJournal(journalPath string, journal nativeReleaseJournal) error {
	content, err := json.Marshal(journal)
	if err != nil {
		return err
	}
	return writeNativeServiceFile(journalPath, append(content, '\n'))
}

func readNativeReleaseMarker(markerPath string) (string, bool, error) {
	content, err := os.ReadFile(markerPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read native release marker: %w", err)
	}
	digest := strings.TrimSpace(string(content))
	if !validNativeReleaseDigest(digest) {
		return "", false, fmt.Errorf("native release checksum marker is invalid")
	}
	return digest, true, nil
}

func validNativeReleaseDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func validateNativeReleaseHashes(hashes map[string]string) error {
	for name, digest := range hashes {
		top, _, _ := strings.Cut(name, "/")
		if !fs.ValidPath(name) || path.Clean(name) != name || (top != "bin" && top != "web" && top != "api") || !validNativeReleaseDigest(digest) {
			return fmt.Errorf("invalid native release file hash")
		}
	}
	return nil
}

func nativeReleaseBackupPath(runtimeDirectory, name string) string {
	return filepath.Join(runtimeDirectory, ".native-release-backup-"+name)
}

func nativeReleaseDirectoryExists(directory string) (bool, error) {
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, fmt.Errorf("path is not a regular directory")
	}
	return true, nil
}

func nativeReleaseHashesForTop(hashes map[string]string, top string) map[string]string {
	result := make(map[string]string)
	prefix := top + "/"
	for relativePath, digest := range hashes {
		if strings.HasPrefix(relativePath, prefix) {
			result[strings.TrimPrefix(relativePath, prefix)] = digest
		}
	}
	return result
}

func nativeReleaseDirectoryMatches(directory string, expected map[string]string) (bool, bool, error) {
	exists, err := nativeReleaseDirectoryExists(directory)
	if err != nil || !exists {
		return exists, false, err
	}
	if len(expected) == 0 {
		return true, false, nil
	}
	actual, err := hashNativeReleaseFiles(directory)
	if err != nil {
		return true, false, err
	}
	return true, equalStringMaps(actual, expected), nil
}

func validateNativeReleaseTree(runtimeDirectory string, expected map[string]string) error {
	for _, name := range []string{"bin", "web", "api"} {
		target := filepath.Join(runtimeDirectory, name)
		expectedTop := nativeReleaseHashesForTop(expected, name)
		exists, matches, err := nativeReleaseDirectoryMatches(target, expectedTop)
		if err != nil {
			return fmt.Errorf("inspect native release target %s: %w", name, err)
		}
		if len(expectedTop) == 0 && !exists {
			continue
		}
		if !exists || !matches {
			return fmt.Errorf("native release target %s does not match release manifest", name)
		}
	}
	return nil
}

func rejectNativeReleaseBackups(runtimeDirectory string) error {
	for _, name := range []string{"bin", "web", "api"} {
		if _, err := os.Lstat(nativeReleaseBackupPath(runtimeDirectory, name)); err == nil {
			return fmt.Errorf("native release backup %s exists without a recovery journal", name)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("inspect native release backup %s: %w", name, err)
		}
	}
	return nil
}

func recoverPendingNativeRelease(runtimeDirectory string, journal nativeReleaseJournal, rename func(string, string) error) error {
	if journal.PreviousArchiveSHA256 == "" {
		verified := make([]string, 0, 3)
		for _, name := range []string{"bin", "web", "api"} {
			if _, err := os.Lstat(nativeReleaseBackupPath(runtimeDirectory, name)); err == nil {
				return fmt.Errorf("native release backup %s does not belong to the pending release journal", name)
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			target := filepath.Join(runtimeDirectory, name)
			exists, matches, err := nativeReleaseDirectoryMatches(target, nativeReleaseHashesForTop(journal.Files, name))
			if err != nil {
				return fmt.Errorf("inspect pending native release target %s: %w", name, err)
			}
			if exists && !matches {
				return fmt.Errorf("native release target %s does not match pending release journal", name)
			}
			if exists {
				verified = append(verified, target)
			}
		}
		for _, target := range verified {
			if err := os.RemoveAll(target); err != nil {
				return fmt.Errorf("remove verified pending native release target: %w", err)
			}
		}
		return nil
	}

	type recoveryState struct {
		name                  string
		target, backup        string
		targetExists          bool
		targetMatchesPrevious bool
		targetMatchesNext     bool
		targetIsNext          bool
		backupExists          bool
	}
	states := make([]recoveryState, 0, 3)
	for _, name := range []string{"bin", "web", "api"} {
		state := recoveryState{name: name, target: filepath.Join(runtimeDirectory, name), backup: nativeReleaseBackupPath(runtimeDirectory, name)}
		previous := nativeReleaseHashesForTop(journal.PreviousFiles, name)
		next := nativeReleaseHashesForTop(journal.Files, name)
		var err error
		state.targetExists, state.targetMatchesPrevious, err = nativeReleaseDirectoryMatches(state.target, previous)
		if err != nil {
			return fmt.Errorf("inspect pending update target %s: %w", name, err)
		}
		if state.targetExists {
			_, state.targetMatchesNext, err = nativeReleaseDirectoryMatches(state.target, next)
			if err != nil {
				return fmt.Errorf("inspect pending update target %s: %w", name, err)
			}
		}
		var matchesPrevious bool
		state.backupExists, matchesPrevious, err = nativeReleaseDirectoryMatches(state.backup, previous)
		if err != nil {
			return fmt.Errorf("inspect pending update backup %s: %w", name, err)
		}
		if state.backupExists && (len(previous) == 0 || !matchesPrevious) {
			return fmt.Errorf("native release backup %s does not match pending release journal", name)
		}
		if len(previous) == 0 {
			if state.backupExists || state.targetExists && !state.targetMatchesNext {
				return fmt.Errorf("native release update state for %s is inconsistent with pending release journal", name)
			}
			state.targetIsNext = state.targetExists
		} else if state.backupExists {
			if state.targetExists && !state.targetMatchesNext {
				return fmt.Errorf("native release target %s does not match pending release journal", name)
			}
			state.targetIsNext = state.targetExists
		} else if !state.targetExists || !state.targetMatchesPrevious {
			return fmt.Errorf("native release update state for %s is inconsistent with pending release journal", name)
		}
		states = append(states, state)
	}
	for _, state := range states {
		if state.targetIsNext {
			if err := os.RemoveAll(state.target); err != nil {
				return fmt.Errorf("remove verified updated native release target %s: %w", state.name, err)
			}
			state.targetExists = false
		}
		if state.backupExists {
			if err := rename(state.backup, state.target); err != nil {
				return fmt.Errorf("restore native release backup %s: %w", state.name, err)
			}
		}
	}
	if err := validateNativeReleaseTree(runtimeDirectory, journal.PreviousFiles); err != nil {
		return fmt.Errorf("validate restored native release: %w", err)
	}
	return nil
}

func cleanupNativeReleaseStages(runtimeDirectory string) error {
	entries, err := os.ReadDir(runtimeDirectory)
	if err != nil {
		return fmt.Errorf("scan native release stages: %w", err)
	}
	stages := make([]string, 0)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".native-release-stage-") {
			continue
		}
		path := filepath.Join(runtimeDirectory, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("inspect stale native release stage %s: %w", entry.Name(), err)
		}
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refuse to remove non-directory native release stage %s", entry.Name())
		}
		stages = append(stages, path)
	}
	for _, stage := range stages {
		if err := os.RemoveAll(stage); err != nil {
			return fmt.Errorf("remove stale native release stage: %w", err)
		}
	}
	return nil
}

func removeNativeReleaseBackups(runtimeDirectory string, previousFiles map[string]string) error {
	verified := make([]string, 0, 3)
	for _, name := range []string{"bin", "web", "api"} {
		backup := nativeReleaseBackupPath(runtimeDirectory, name)
		exists, matches, err := nativeReleaseDirectoryMatches(backup, nativeReleaseHashesForTop(previousFiles, name))
		if err != nil {
			return fmt.Errorf("inspect native release backup %s: %w", name, err)
		}
		if exists && !matches {
			return fmt.Errorf("native release backup %s does not match pending release journal", name)
		}
		if exists {
			verified = append(verified, backup)
		}
	}
	for _, backup := range verified {
		if err := os.RemoveAll(backup); err != nil {
			return fmt.Errorf("remove verified native release backup: %w", err)
		}
	}
	return nil
}

func hashNativeReleaseFiles(directory string) (map[string]string, error) {
	hashes := make(map[string]string)
	directories := make([]string, 0)
	err := filepath.WalkDir(directory, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if filePath == directory {
			return nil
		}
		if entry.IsDir() {
			relativePath, err := filepath.Rel(directory, filePath)
			if err != nil {
				return err
			}
			directories = append(directories, filepath.ToSlash(relativePath))
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return fmt.Errorf("native release contains non-regular path %s", filePath)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		content, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		hasher := sha256.New()
		_, _ = fmt.Fprintf(hasher, "native-release-file-v1 mode=%04o\n", info.Mode().Perm())
		_, _ = hasher.Write(content)
		digest := hasher.Sum(nil)
		relativePath, err := filepath.Rel(directory, filePath)
		if err != nil {
			return err
		}
		hashes[filepath.ToSlash(relativePath)] = hex.EncodeToString(digest)
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, directory := range directories {
		prefix := directory + "/"
		hasFile := false
		for filePath := range hashes {
			if strings.HasPrefix(filePath, prefix) {
				hasFile = true
				break
			}
		}
		if !hasFile {
			return nil, fmt.Errorf("native release contains empty directory %s", directory)
		}
	}
	return hashes, nil
}

func equalStringMaps(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if right[key] != value {
			return false
		}
	}
	return true
}

func validateNativeReleaseBaseURL(value string) (string, error) {
	raw := strings.TrimSuffix(strings.TrimSpace(value), "/")
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Contains(raw, "\\") {
		return "", fmt.Errorf("native release base URL must be an absolute HTTP(S) URL without credentials, query, or fragment")
	}
	return raw, nil
}

func downloadNativeRelease(ctx context.Context, client *http.Client, sourceURL string, maximum int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected HTTP status %d", response.StatusCode)
	}
	content, err := io.ReadAll(io.LimitReader(response.Body, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(content)) > maximum {
		return nil, fmt.Errorf("download exceeds %d bytes", maximum)
	}
	return content, nil
}

func parseNativeReleaseChecksum(content []byte) ([]byte, error) {
	fields := strings.Fields(string(content))
	if len(fields) == 0 || len(fields[0]) != sha256.Size*2 {
		return nil, fmt.Errorf("native release checksum file is invalid")
	}
	digest, err := hex.DecodeString(fields[0])
	if err != nil || len(digest) != sha256.Size {
		return nil, fmt.Errorf("native release checksum file is invalid")
	}
	return digest, nil
}

func extractNativeRelease(archive []byte, destination string) error {
	gzipReader, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		return fmt.Errorf("open native release archive: %w", err)
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	seen := make(map[string]struct{})
	var extracted int64
	for {
		header, err := tarReader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("read native release archive: %w", err)
		}
		name := header.Name
		if header.Typeflag == tar.TypeDir {
			name = strings.TrimSuffix(name, "/")
		}
		if strings.ContainsAny(name, "\\\x00") || !fs.ValidPath(name) || path.Clean(name) != name {
			return fmt.Errorf("native release contains unsafe path %q", name)
		}
		top, _, _ := strings.Cut(name, "/")
		if top != "bin" && top != "web" && top != "api" {
			return fmt.Errorf("native release contains unsupported top-level path %q", name)
		}
		if _, exists := seen[name]; exists {
			return fmt.Errorf("native release contains duplicate path %q", name)
		}
		seen[name] = struct{}{}
		target := filepath.Join(destination, filepath.FromSlash(name))
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("create native release directory: %w", err)
			}
		case tar.TypeReg, tar.TypeRegA:
			if header.Size < 0 || header.Size > maxNativeExtractedBytes-extracted {
				return fmt.Errorf("native release extracted size exceeds limit")
			}
			extracted += header.Size
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fmt.Errorf("create native release parent: %w", err)
			}
			mode := fs.FileMode(0o644)
			if top == "bin" {
				mode = 0o755
			}
			file, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
			if err != nil {
				return fmt.Errorf("create native release file: %w", err)
			}
			_, copyErr := io.CopyN(file, tarReader, header.Size)
			closeErr := file.Close()
			if copyErr != nil {
				return fmt.Errorf("extract native release file: %w", copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("close native release file: %w", closeErr)
			}
		default:
			return fmt.Errorf("native release contains unsupported entry type for %q", name)
		}
	}
	return nil
}

func validateNativeReleaseFiles(directory string, plan InstallPlan, platform NativePlatform) error {
	extension := ""
	if platform == NativePlatformWindows {
		extension = ".exe"
	}
	required := make([]string, 0, 8)
	if slices.Contains(plan.Components, ComponentAPI) {
		required = append(required, filepath.Join("bin", "mikiko-gallery-studio-api"+extension), filepath.Join("api", "openapi", "openapi.yaml"))
	}
	if slices.Contains(plan.Components, ComponentWorker) {
		required = append(required, filepath.Join("bin", "mikiko-gallery-studio-worker"+extension))
	}
	if slices.Contains(plan.Components, ComponentGateway) {
		required = append(required, filepath.Join("bin", "mikiko-gallery-studio-gateway"+extension))
	}
	for component, frontend := range map[Component]string{
		ComponentUserWeb: "user", ComponentAdminWeb: "admin", ComponentDocsWeb: "docs",
	} {
		if slices.Contains(plan.Components, component) {
			required = append(required, filepath.Join("web", frontend, "index.html"))
		}
	}
	if platform == NativePlatformWindows && (slices.Contains(plan.Components, ComponentAPI) || slices.Contains(plan.Components, ComponentWorker) || slices.Contains(plan.Components, ComponentGateway)) {
		required = append(required, filepath.Join("bin", "mikiko-gallery-studio-service-host.exe"))
	}
	required = append(required, filepath.Join("bin", "mikiko-gallery-studio-db-migrate"+extension))
	for _, relativePath := range required {
		info, err := os.Stat(filepath.Join(directory, relativePath))
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("native release is missing required file %s", filepath.ToSlash(relativePath))
		}
		if platform == NativePlatformLinux && strings.HasPrefix(filepath.ToSlash(relativePath), "bin/") && info.Mode().Perm()&0o111 == 0 {
			return fmt.Errorf("native release binary %s is not executable", filepath.ToSlash(relativePath))
		}
	}
	return nil
}
