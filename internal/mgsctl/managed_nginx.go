package mgsctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"time"

	deploymentassets "github.com/fatballfish/pic-gallery/deployments"
	"github.com/fatballfish/pic-gallery/internal/config"
)

const managedNginxPath = "assets/nginx-default.conf"
const managedNginxJournalPath = "config/.mgsctl-nginx-refresh.json"

// Historical official templates are immutable provenance fixtures, extracted from
// deployments/nginx/default.conf at the full commits below. Compatibility is an
// exact rendered-byte allowlist, never a general hash-check bypass.
//
//go:embed managed_nginx_fixtures/88114a0.conf
var nginx88114a0 []byte // 88114a0b35b43900217477fdc997f27754c222cd

//go:embed managed_nginx_fixtures/971e03c.conf
var nginx971e03c []byte // 971e03c187706acd4fb8428eb2eb8aac0b547cb3

func managedNginxOwnership(plan InstallPlan, asset []byte, recordedHash string) bool {
	if recordedHash == fmt.Sprintf("%x", sha256.Sum256(asset)) {
		return true
	}
	actualOfficial, recordedOfficial := false, false
	for _, template := range [][]byte{nginx88114a0, nginx971e03c} {
		rendered, err := materializeNginxConfig(plan, template)
		if err != nil {
			continue
		} // Unsupported historical topologies fail closed.
		actualOfficial = actualOfficial || bytes.Equal(asset, rendered)
		recordedOfficial = recordedOfficial || recordedHash == fmt.Sprintf("%x", sha256.Sum256(rendered))
	}
	return actualOfficial && recordedOfficial
}

type managedNginxJournal struct {
	Version                                      int    `json:"version"`
	Phase                                        string `json:"phase"`
	OldAsset, NewAsset, OldManifest, NewManifest []byte
}

func decodeNginxManifest(content []byte) (deploymentManifest, error) {
	var m deploymentManifest
	d := json.NewDecoder(bytes.NewReader(content))
	d.DisallowUnknownFields()
	if err := d.Decode(&m); err != nil {
		return m, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return m, fmt.Errorf("manifest has trailing data")
	}
	return m, nil
}

func validateNginxJournal(j managedNginxJournal, installationID string) error {
	if j.Version != 1 || (j.Phase != "prepared" && j.Phase != "rollback" && j.Phase != "completed") {
		return fmt.Errorf("invalid managed nginx journal phase")
	}
	old, err := decodeNginxManifest(j.OldManifest)
	if err != nil {
		return err
	}
	next, err := decodeNginxManifest(j.NewManifest)
	if err != nil {
		return err
	}
	if old.SchemaVersion != 1 || old.InstallationID != installationID || next.InstallationID != installationID || !managedNginxOwnership(old.Plan, j.OldAsset, old.Files[managedNginxPath]) || next.Files[managedNginxPath] != fmt.Sprintf("%x", sha256.Sum256(j.NewAsset)) {
		return fmt.Errorf("invalid managed nginx journal ownership evidence")
	}
	candidate, err := materializeNginxConfig(old.Plan, deploymentassets.NginxDefault())
	if err != nil || !bytes.Equal(candidate, j.NewAsset) {
		return fmt.Errorf("managed nginx journal candidate is not trusted embedded template")
	}
	next.Files[managedNginxPath] = old.Files[managedNginxPath]
	a, _ := json.Marshal(old)
	b, _ := json.Marshal(next)
	if !bytes.Equal(a, b) {
		return fmt.Errorf("managed nginx journal changes unrelated manifest metadata")
	}
	return nil
}

// ManagedNginxDependencies supplies runtime operations separately from asset
// ownership checks. Validate must test the candidate with a fresh mount (e.g.
// compose run nginx -t), because an existing container may retain the old bind
// mount inode. Apply must recreate the gateway, not merely reload that container.
// Apply may be nil when the caller has stopped services and owns their restart.
type ManagedNginxDependencies struct {
	Validate func(context.Context, InstallPlan) error
	Apply    func(context.Context, InstallPlan) error
	// RemoveJournal defaults to os.Remove; injected for cleanup failure testing.
	RemoveJournal func(string) error
}

// RefreshManagedNginx replaces only an unmodified, manifest-owned Docker gateway
// asset with this tool's embedded template. It does not reconfigure canonical URLs
// or change other deployment assets. Callers must serialize other runtime edits
// with the same install lock.
func RefreshManagedNginx(ctx context.Context, runtimeDir string, dependencies ManagedNginxDependencies) (returnErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if dependencies.Validate == nil {
		return fmt.Errorf("managed nginx validation dependency is required")
	}
	runtimeDir = filepath.Clean(defaultString(runtimeDir, "."))
	release, err := acquireInstallLock(ctx, filepath.Join(runtimeDir, "config", ".mgsctl-install.lock"))
	if err != nil {
		return fmt.Errorf("lock managed nginx refresh: %w", err)
	}
	defer func() { returnErr = errors.Join(returnErr, release()) }()
	return refreshManagedNginxLocked(ctx, runtimeDir, dependencies)
}

// refreshManagedNginxLocked requires the caller to hold the runtime install lock.
// It is suitable for reconfiguration's stopped-services phase with Apply=nil.
func refreshManagedNginxLocked(ctx context.Context, runtimeDir string, dependencies ManagedNginxDependencies) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if dependencies.Validate == nil {
		return fmt.Errorf("managed nginx validation dependency is required")
	}
	runtimeDir = filepath.Clean(defaultString(runtimeDir, "."))
	plan, document, err := loadInstallation(runtimeDir)
	if err != nil {
		return fmt.Errorf("load installation for managed nginx refresh: %w", err)
	}
	if plan.Mode != config.DeploymentModeDocker || !slices.Contains(plan.Components, ComponentGateway) {
		return fmt.Errorf("managed nginx refresh requires a Docker gateway installation")
	}
	// Refuse symlinked asset directories as well as symlinked leaf files.
	directory, err := os.Lstat(filepath.Join(runtimeDir, "assets"))
	if err != nil {
		return fmt.Errorf("inspect managed nginx directory: %w", err)
	}
	if !directory.IsDir() {
		return fmt.Errorf("managed nginx assets directory must be a directory, not a symlink")
	}
	assetPath := filepath.Join(runtimeDir, filepath.FromSlash(managedNginxPath))
	manifestPath := filepath.Join(runtimeDir, "deployment.json")
	for _, path := range []string{assetPath, manifestPath} {
		exists, err := regularFileExists(path)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("managed nginx refresh requires existing asset and manifest")
		}
	}
	journalPath := filepath.Join(runtimeDir, filepath.FromSlash(managedNginxJournalPath))
	if err := inspectInstallPath(journalPath); err != nil {
		return err
	}
	if dependencies.RemoveJournal == nil {
		dependencies.RemoveJournal = os.Remove
	}
	if raw, err := os.ReadFile(journalPath); err == nil {
		var j managedNginxJournal
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		if err := d.Decode(&j); err != nil {
			return fmt.Errorf("decode managed nginx recovery journal: %w", err)
		}
		if d.Decode(new(any)) != io.EOF {
			return fmt.Errorf("managed nginx journal has trailing data")
		}
		if err := validateNginxJournal(j, document.Values["INSTALLATION_ID"]); err != nil {
			return err
		}
		if err := checkNginxJournalFiles(assetPath, manifestPath, j); err != nil {
			return err
		}
		if j.Phase == "completed" {
			asset, _ := os.ReadFile(assetPath)
			manifest, _ := os.ReadFile(manifestPath)
			if !bytes.Equal(asset, j.NewAsset) || !bytes.Equal(manifest, j.NewManifest) {
				return fmt.Errorf("completed managed nginx refresh differs from recorded result")
			}
			if err := dependencies.RemoveJournal(journalPath); err != nil {
				return fmt.Errorf("managed nginx refresh completed; journal cleanup failed: %w", err)
			}
			return nil
		}
		return executeNginxJournal(ctx, plan, assetPath, manifestPath, journalPath, j, dependencies)
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("read managed nginx recovery journal: %w", err)
	}
	originalManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("read managed nginx manifest: %w", err)
	}
	var manifest deploymentManifest
	decoder := json.NewDecoder(bytes.NewReader(originalManifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return fmt.Errorf("decode managed nginx manifest: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("managed nginx manifest has trailing data")
	}
	if manifest.SchemaVersion != 1 || manifest.InstallationID != document.Values["INSTALLATION_ID"] {
		return fmt.Errorf("managed nginx manifest installation identity is invalid")
	}
	originalAsset, err := os.ReadFile(assetPath)
	if err != nil {
		return fmt.Errorf("read managed nginx asset: %w", err)
	}
	if !managedNginxOwnership(plan, originalAsset, manifest.Files[managedNginxPath]) {
		return fmt.Errorf("refuse to replace customized or untracked managed nginx asset %s", managedNginxPath)
	}
	candidate, err := materializeNginxConfig(plan, deploymentassets.NginxDefault())
	if err != nil {
		return fmt.Errorf("render managed nginx asset: %w", err)
	}
	if bytes.Equal(originalAsset, candidate) {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Keep all identities, plan fields, and unrelated file hashes unchanged.
	manifest.Files[managedNginxPath] = fmt.Sprintf("%x", sha256.Sum256(candidate))
	updatedManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("render managed nginx manifest: %w", err)
	}
	j := managedNginxJournal{Version: 1, Phase: "prepared", OldAsset: originalAsset, NewAsset: candidate, OldManifest: originalManifest, NewManifest: append(updatedManifest, '\n')}
	raw, err := json.Marshal(j)
	if err != nil {
		return err
	}
	if err := writePrivateFileAtomicNoReplace(journalPath, raw); err != nil {
		return fmt.Errorf("persist managed nginx recovery journal: %w", err)
	}
	return executeNginxJournal(ctx, plan, assetPath, manifestPath, journalPath, j, dependencies)
}

func checkNginxJournalFiles(assetPath, manifestPath string, j managedNginxJournal) error {
	for _, f := range []struct {
		path      string
		old, next []byte
	}{{assetPath, j.OldAsset, j.NewAsset}, {manifestPath, j.OldManifest, j.NewManifest}} {
		exists, err := regularFileExists(f.path)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("managed nginx recovery file is missing")
		}
		actual, err := os.ReadFile(f.path)
		if err != nil || (!bytes.Equal(actual, f.old) && !bytes.Equal(actual, f.next)) {
			return fmt.Errorf("managed nginx recovery refused external file modification")
		}
	}
	return nil
}

func executeNginxJournal(ctx context.Context, plan InstallPlan, assetPath, manifestPath, journalPath string, j managedNginxJournal, deps ManagedNginxDependencies) error {
	persist := func(phase string) error {
		j.Phase = phase
		raw, err := json.Marshal(j)
		if err != nil {
			return err
		}
		return writePrivateFileAtomicReplace(journalPath, raw)
	}
	restore := func() error {
		if err := checkNginxJournalFiles(assetPath, manifestPath, j); err != nil {
			return err
		}
		if err := writeDeploymentFileAtomicReplace(assetPath, j.OldAsset); err != nil {
			return err
		}
		if err := writePrivateFileAtomicReplace(manifestPath, j.OldManifest); err != nil {
			return err
		}
		if deps.Apply != nil {
			recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
			defer cancel()
			if err := deps.Apply(recovery, plan); err != nil {
				return err
			}
		}
		return deps.RemoveJournal(journalPath)
	}
	if j.Phase == "rollback" {
		if err := restore(); err != nil {
			return fmt.Errorf("restore previous managed nginx deployment: %w", err)
		}
		return fmt.Errorf("interrupted managed nginx refresh restored; rerun refresh")
	}
	rollback := func(cause error, applied bool) error {
		if err := persist("rollback"); err != nil {
			return errors.Join(cause, fmt.Errorf("record managed nginx rollback: %w", err))
		}
		// Before Apply, the existing container still holds the original configuration.
		savedApply := deps.Apply
		if !applied {
			deps.Apply = nil
		}
		err := restore()
		deps.Apply = savedApply
		if err != nil {
			return errors.Join(cause, fmt.Errorf("restore previous managed nginx deployment: %w", err))
		}
		return cause
	}
	if err := checkNginxJournalFiles(assetPath, manifestPath, j); err != nil {
		return err
	}
	if err := writeDeploymentFileAtomicReplace(assetPath, j.NewAsset); err != nil {
		return rollback(fmt.Errorf("write managed nginx candidate: %w", err), false)
	}
	if err := deps.Validate(ctx, plan); err != nil {
		return rollback(fmt.Errorf("validate managed nginx candidate: %w", err), false)
	}
	if err := ctx.Err(); err != nil {
		return rollback(err, false)
	}
	if err := checkNginxJournalFiles(assetPath, manifestPath, j); err != nil {
		return err
	}
	if err := writePrivateFileAtomicReplace(manifestPath, j.NewManifest); err != nil {
		return rollback(fmt.Errorf("write managed nginx manifest: %w", err), false)
	}
	if deps.Apply != nil {
		if err := deps.Apply(ctx, plan); err != nil {
			return rollback(fmt.Errorf("apply managed nginx candidate: %w", err), true)
		}
	}
	// Once applied, never roll back because phase persistence or cleanup failed.
	// A prepared journal resumes forward, validating and reapplying the candidate.
	if err := persist("completed"); err != nil {
		return fmt.Errorf("managed nginx applied; completion journal write failed: %w", err)
	}
	if err := deps.RemoveJournal(journalPath); err != nil {
		return fmt.Errorf("managed nginx refresh completed; journal cleanup failed: %w", err)
	}
	return nil
}
