package mgsctl

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type ReconfigureOptions struct{ RuntimeDir, PublicAPIURL string }
type ReconfigureDependencies struct {
	OpenStore             setup.SetupStoreOpener
	OpenInstallationStore func(context.Context, InstallPlan, string) (setup.SetupStoreSession, error)
	StopDeployment        func(context.Context, InstallPlan) error
	ApplyDeployment       func(context.Context, InstallPlan) error
	RefreshAssets         func(context.Context, InstallPlan) error
}
type ReconfigureResult struct {
	ConfigRevision int
	PublicAPIURL   string
}

type configurationJournal struct {
	Committed                                        bool               `json:"committed"`
	Version                                          int                `json:"version"`
	OldBinding                                       setup.SetupBinding `json:"old_binding"`
	OldProof                                         setup.CommitProof  `json:"old_proof"`
	NewProof                                         setup.CommitProof  `json:"new_proof"`
	OldRuntime, NewRuntime, OldManifest, NewManifest []byte
}

func validateCanonicalReconfigureURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Path != "" || u.Opaque != "" {
		return fmt.Errorf("public API URL must be an HTTPS origin without path, credentials, query or fragment")
	}
	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("public API URL port is invalid")
		}
	}
	return nil
}

// Reconfigure changes only the canonical public origin. Its private journal
// contains runtime backups and must remain protected like runtime.env itself.
// A retry first restores an interrupted operation, then verifies the old proof.
func Reconfigure(ctx context.Context, options ReconfigureOptions, deps ReconfigureDependencies) (ReconfigureResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := validateCanonicalReconfigureURL(options.PublicAPIURL); err != nil {
		return ReconfigureResult{}, err
	}
	if (deps.OpenStore == nil && deps.OpenInstallationStore == nil) || deps.StopDeployment == nil || deps.ApplyDeployment == nil {
		return ReconfigureResult{}, fmt.Errorf("reconfigure store and deployment dependencies are required")
	}
	dir := filepath.Clean(defaultString(options.RuntimeDir, "."))
	unlock, err := acquireInstallLock(ctx, filepath.Join(dir, "config", ".mgsctl-install.lock"))
	if err != nil {
		return ReconfigureResult{}, fmt.Errorf("acquire configuration lock: %w", err)
	}
	defer unlock()
	runtimePath := filepath.Join(dir, "config", "runtime.env")
	manifestPath := filepath.Join(dir, "deployment.json")
	journalPath := filepath.Join(dir, "config", ".mgsctl-reconfigure.json")
	stopPath := filepath.Join(dir, "config", ".mgsctl-reconfigure-stop.json")
	writeStopIntent := func() error { return writePrivateFileAtomicNoReplace(stopPath, []byte("{\"version\":1}\n")) }
	clearStopIntent := func() error {
		err := os.Remove(stopPath)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	stateStore := setup.NewStateStore(runtimePath)
	for _, path := range []string{runtimePath, manifestPath, journalPath, stopPath} {
		if err := inspectInstallPath(path); err != nil {
			return ReconfigureResult{}, fmt.Errorf("configuration target is not a regular file")
		}
	}
	initialRuntime, initialErr := os.ReadFile(runtimePath)
	if initialErr != nil {
		return ReconfigureResult{}, fmt.Errorf("read configuration snapshot failed")
	}
	plan, doc, err := loadInstallation(dir)
	if err != nil {
		return ReconfigureResult{}, fmt.Errorf("load configuration for reconfigure failed")
	}
	if plan.Topology != config.DeploymentTopologySingle || plan.Role != config.DeploymentRoleSingle {
		return ReconfigureResult{}, fmt.Errorf("reconfigure currently requires a single-node installation")
	}
	var store setup.SetupStoreSession
	if deps.OpenInstallationStore != nil {
		store, err = deps.OpenInstallationStore(ctx, plan, doc.Values["DATABASE_URL"])
	} else {
		store, err = deps.OpenStore(ctx, doc.Values["DATABASE_URL"])
	}
	if err != nil {
		return ReconfigureResult{}, fmt.Errorf("open configuration binding store failed")
	}
	defer store.Close()
	updater, ok := store.(setup.SetupBindingConfigUpdater)
	if !ok {
		return ReconfigureResult{}, fmt.Errorf("configuration binding store does not support revision CAS")
	}
	rollback := func(j configurationJournal) error {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		// Refuse to overwrite external edits, including ambiguous atomic-write errors.
		for _, f := range []struct {
			path      string
			old, next []byte
		}{{runtimePath, j.OldRuntime, j.NewRuntime}, {manifestPath, j.OldManifest, j.NewManifest}} {
			actual, err := os.ReadFile(f.path)
			if err != nil || (!bytes.Equal(actual, f.old) && !bytes.Equal(actual, f.next)) {
				return fmt.Errorf("configuration changed outside the recorded operation; recovery stopped")
			}
		}
		current, err := store.GetBinding(recovery, j.OldBinding.InstallationID)
		if err != nil {
			return fmt.Errorf("read recovery binding failed")
		}
		next := j.OldBinding
		next.ConfigRevision = j.NewProof.ConfigRevision
		next.RequestDigest = j.NewProof.RequestDigest
		if current == next {
			if _, err := updater.CompareSwapConfiguration(recovery, setup.SetupBindingConfigUpdate{Expected: next, ConfigRevision: j.OldProof.ConfigRevision, RequestDigest: j.OldProof.RequestDigest}); err != nil {
				return fmt.Errorf("restore database binding failed")
			}
		} else if current != j.OldBinding {
			return fmt.Errorf("recovery database binding conflict")
		}
		state, exists, err := stateStore.Load()
		if err != nil || !exists || state.Commit == nil {
			return fmt.Errorf("read recovery state failed")
		}
		if *state.Commit == j.NewProof {
			if _, err := stateStore.CompareSwapCompletedCommit(j.NewProof, j.OldProof, time.Now().UTC()); err != nil {
				return fmt.Errorf("restore configuration proof failed")
			}
		} else if *state.Commit != j.OldProof {
			return fmt.Errorf("recovery configuration proof conflict")
		}
		if err := config.WriteRuntimeEnvAtomic(runtimePath, j.OldRuntime); err != nil {
			return fmt.Errorf("restore runtime failed")
		}
		if err := config.WriteRuntimeEnvAtomic(manifestPath, j.OldManifest); err != nil {
			return fmt.Errorf("restore deployment manifest failed")
		}
		var m deploymentManifest
		if json.Unmarshal(j.OldManifest, &m) != nil {
			return fmt.Errorf("recovery manifest invalid")
		}
		m.Plan.RuntimeDir = dir
		if err := deps.ApplyDeployment(recovery, m.Plan); err != nil {
			return fmt.Errorf("restart restored deployment failed")
		}
		if err := clearStopIntent(); err != nil {
			return err
		}
		return os.Remove(journalPath)
	}
	if err := inspectInstallPath(journalPath); err != nil {
		return ReconfigureResult{}, err
	}
	if data, err := os.ReadFile(journalPath); err == nil {
		var j configurationJournal
		if json.Unmarshal(data, &j) != nil || j.Version != 1 || validateConfigurationJournal(j, plan, doc) != nil {
			return ReconfigureResult{}, fmt.Errorf("configuration recovery journal is invalid")
		}
		// A fully published new proof is resumable even if the process died
		// immediately after a successful rollout and before its commit marker.
		rb, re := store.GetBinding(ctx, j.OldProof.InstallationID)
		rs, rx, rse := stateStore.Load()
		rr, rre := os.ReadFile(runtimePath)
		rm, rme := os.ReadFile(manifestPath)
		fullyPublished := re == nil && rse == nil && rx && rs.Commit != nil && *rs.Commit == j.NewProof && rb.ConfigRevision == j.NewProof.ConfigRevision && rb.RequestDigest == j.NewProof.RequestDigest && rb.OperationID == j.OldBinding.OperationID && rb.AdminID == j.OldBinding.AdminID && rre == nil && rme == nil && bytes.Equal(rr, j.NewRuntime) && bytes.Equal(rm, j.NewManifest)
		if fullyPublished && !j.Committed {
			if err := deps.ApplyDeployment(ctx, plan); err != nil {
				return ReconfigureResult{}, fmt.Errorf("resume published configuration failed; journal retained")
			}
			j.Committed = true
			data, _ := json.Marshal(j)
			if err := config.WriteRuntimeEnvAtomic(journalPath, data); err != nil {
				return ReconfigureResult{}, fmt.Errorf("persist resumed commit marker failed")
			}
		}
		if j.Committed {
			binding, e := store.GetBinding(ctx, j.OldProof.InstallationID)
			state, exists, se := stateStore.Load()
			actual, readErr := os.ReadFile(runtimePath)
			manifest, me := os.ReadFile(manifestPath)
			if e != nil || se != nil || !exists || state.Commit == nil || *state.Commit != j.NewProof || binding.ConfigRevision != j.NewProof.ConfigRevision || binding.RequestDigest != j.NewProof.RequestDigest || binding.OperationID != j.OldBinding.OperationID || binding.AdminID != j.OldBinding.AdminID || readErr != nil || me != nil || !bytes.Equal(actual, j.NewRuntime) || !bytes.Equal(manifest, j.NewManifest) {
				return ReconfigureResult{}, fmt.Errorf("committed configuration recovery mismatch")
			}
			if err := clearStopIntent(); err != nil {
				return ReconfigureResult{}, fmt.Errorf("committed stop-intent cleanup failed")
			}
			if err := os.Remove(journalPath); err != nil {
				return ReconfigureResult{}, fmt.Errorf("committed journal cleanup failed")
			}
			return ReconfigureResult{j.NewProof.ConfigRevision, doc.Values["PUBLIC_API_URL"]}, nil
		}
		if deps.RefreshAssets != nil {
			if err := deps.RefreshAssets(ctx, plan); err != nil {
				return ReconfigureResult{}, fmt.Errorf("recover managed assets failed")
			}
		}
		if err := deps.StopDeployment(ctx, plan); err != nil {
			return ReconfigureResult{}, fmt.Errorf("stop deployment for recovery failed")
		}
		if err := rollback(j); err != nil {
			return ReconfigureResult{}, err
		}
		return ReconfigureResult{}, fmt.Errorf("interrupted configuration restored; rerun reconfigure")
	} else if !errors.Is(err, os.ErrNotExist) {
		return ReconfigureResult{}, fmt.Errorf("read configuration journal failed")
	}
	state, exists, err := stateStore.Load()
	if err != nil || !exists || state.Validate() != nil || state.Phase != setup.InstallPhaseCompleted || state.Commit == nil || doc.Values["SETUP_COMPLETED"] != "true" {
		return ReconfigureResult{}, fmt.Errorf("completed configuration proof required")
	}
	binding, err := store.GetBinding(ctx, state.InstallationID)
	if err != nil {
		return ReconfigureResult{}, fmt.Errorf("read configuration binding failed")
	}
	digest, err := setup.CanonicalRequestDigest(doc.Values, binding.AdminEmail)
	revision, revErr := strconv.Atoi(doc.Values["CONFIG_REVISION"])
	if err != nil || revErr != nil || revision != state.Commit.ConfigRevision || revision != binding.ConfigRevision || state.Commit.RequestDigest != digest || binding.RequestDigest != digest || binding.OperationID != state.Commit.OperationID || binding.InstallationID != state.InstallationID || doc.Values["INSTALLATION_ID"] != state.InstallationID {
		return ReconfigureResult{}, fmt.Errorf("existing configuration binding does not match")
	}
	if err := inspectInstallPath(stopPath); err != nil {
		return ReconfigureResult{}, err
	}
	if _, err := os.Stat(stopPath); err == nil {
		if deps.RefreshAssets != nil {
			if err := deps.RefreshAssets(ctx, plan); err != nil {
				return ReconfigureResult{}, fmt.Errorf("recover stopped managed assets failed")
			}
		}
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if err := deps.ApplyDeployment(recovery, plan); err != nil {
			return ReconfigureResult{}, fmt.Errorf("recover interrupted stop failed")
		}
		if err := clearStopIntent(); err != nil {
			return ReconfigureResult{}, fmt.Errorf("clear stop intent failed")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ReconfigureResult{}, fmt.Errorf("read stop intent failed")
	}
	if doc.Values["PUBLIC_API_URL"] == options.PublicAPIURL {
		if plan.PublicAPIURL != options.PublicAPIURL {
			return ReconfigureResult{}, fmt.Errorf("deployment public URL does not match runtime")
		}
		if err := writeStopIntent(); err != nil {
			return ReconfigureResult{}, fmt.Errorf("persist stop intent failed")
		}
		stopErr := deps.StopDeployment(ctx, plan)
		var refreshErr error
		if stopErr == nil && deps.RefreshAssets != nil {
			refreshErr = deps.RefreshAssets(ctx, plan)
		}
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if err := deps.ApplyDeployment(recovery, plan); err != nil {
			return ReconfigureResult{}, fmt.Errorf("restart canonical deployment failed; stop intent retained")
		}
		if err := clearStopIntent(); err != nil {
			return ReconfigureResult{}, fmt.Errorf("clear stop intent failed")
		}
		if stopErr != nil || refreshErr != nil {
			return ReconfigureResult{}, fmt.Errorf("canonical asset refresh failed; deployment restarted")
		}
		return ReconfigureResult{revision, options.PublicAPIURL}, nil
	}
	if revision == math.MaxInt {
		return ReconfigureResult{}, fmt.Errorf("configuration revision exhausted")
	}
	if err := writeStopIntent(); err != nil {
		return ReconfigureResult{}, fmt.Errorf("persist stop intent failed")
	}
	if err := deps.StopDeployment(ctx, plan); err != nil {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if err := deps.ApplyDeployment(recovery, plan); err != nil {
			return ReconfigureResult{}, fmt.Errorf("stop application services failed; restart failed; stop intent retained")
		}
		_ = clearStopIntent()
		return ReconfigureResult{}, fmt.Errorf("stop application services failed; previous deployment restarted")
	}
	restartOld := func() {
		recovery, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Minute)
		defer cancel()
		if deps.ApplyDeployment(recovery, plan) == nil {
			_ = clearStopIntent()
		}
	}
	if deps.RefreshAssets != nil {
		if err := deps.RefreshAssets(ctx, plan); err != nil {
			restartOld()
			return ReconfigureResult{}, fmt.Errorf("refresh managed assets failed")
		}
	}
	oldRuntime, err := os.ReadFile(runtimePath)
	if err != nil {
		restartOld()
		return ReconfigureResult{}, fmt.Errorf("read runtime backup failed")
	}
	// Recheck after stopping so a concurrent unexpected writer cannot be adopted.
	actual, err := config.ParseRuntimeEnv(oldRuntime)
	if err != nil {
		restartOld()
		return ReconfigureResult{}, fmt.Errorf("runtime changed during reconfigure")
	}
	actualDigest, err := setup.CanonicalRequestDigest(actual.Values, binding.AdminEmail)
	if err != nil || actualDigest != digest || !bytes.Equal(oldRuntime, initialRuntime) {
		restartOld()
		return ReconfigureResult{}, fmt.Errorf("runtime changed during reconfigure")
	}
	oldManifest, err := os.ReadFile(manifestPath)
	if err != nil {
		restartOld()
		return ReconfigureResult{}, fmt.Errorf("read manifest backup failed")
	}
	var manifest deploymentManifest
	if json.Unmarshal(oldManifest, &manifest) != nil {
		restartOld()
		return ReconfigureResult{}, fmt.Errorf("deployment manifest invalid")
	}
	values := cloneRuntimeValues(doc.Values)
	values["PUBLIC_API_URL"] = options.PublicAPIURL
	values["CONFIG_REVISION"] = strconv.Itoa(revision + 1)
	newDigest, err := setup.CanonicalRequestDigest(values, binding.AdminEmail)
	if err != nil {
		restartOld()
		return ReconfigureResult{}, fmt.Errorf("derive configuration proof failed")
	}
	rendered, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), values, doc.Extensions)
	if err != nil {
		restartOld()
		return ReconfigureResult{}, fmt.Errorf("render configuration failed")
	}
	manifest.Plan.PublicAPIURL = options.PublicAPIURL
	newManifest, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		restartOld()
		return ReconfigureResult{}, fmt.Errorf("render manifest failed")
	}
	newManifest = append(newManifest, '\n')
	nextProof := *state.Commit
	nextProof.ConfigRevision++
	nextProof.RequestDigest = newDigest
	j := configurationJournal{Version: 1, OldBinding: binding, OldProof: *state.Commit, NewProof: nextProof, OldRuntime: oldRuntime, NewRuntime: rendered, OldManifest: oldManifest, NewManifest: newManifest}
	journalBytes, _ := json.Marshal(j)
	if err := writePrivateFileAtomicNoReplace(journalPath, journalBytes); err != nil {
		restartOld()
		return ReconfigureResult{}, fmt.Errorf("persist configuration recovery journal failed")
	}
	fail := func() (ReconfigureResult, error) {
		if err := rollback(j); err != nil {
			return ReconfigureResult{}, fmt.Errorf("configuration update failed; recovery journal retained: %w", err)
		}
		return ReconfigureResult{}, fmt.Errorf("configuration update failed; previous configuration restored")
	}
	if _, err := updater.CompareSwapConfiguration(ctx, setup.SetupBindingConfigUpdate{Expected: binding, ConfigRevision: revision + 1, RequestDigest: newDigest}); err != nil {
		return fail()
	}
	if err := config.WriteRuntimeEnvAtomic(runtimePath, rendered); err != nil {
		return fail()
	}
	if _, err := stateStore.CompareSwapCompletedCommit(*state.Commit, nextProof, time.Now().UTC()); err != nil {
		return fail()
	}
	if err := config.WriteRuntimeEnvAtomic(manifestPath, newManifest); err != nil {
		return fail()
	}
	manifest.Plan.RuntimeDir = dir
	if err := deps.ApplyDeployment(ctx, manifest.Plan); err != nil {
		if stopErr := deps.StopDeployment(context.WithoutCancel(ctx), manifest.Plan); stopErr != nil {
			return ReconfigureResult{}, fmt.Errorf("configuration rollout failed; stop failed; recovery journal retained")
		}
		return fail()
	}
	j.Committed = true
	committedBytes, _ := json.Marshal(j)
	if err := config.WriteRuntimeEnvAtomic(journalPath, committedBytes); err != nil {
		return ReconfigureResult{}, fmt.Errorf("configuration active; commit marker write failed; recovery required")
	}
	if err := clearStopIntent(); err != nil {
		return ReconfigureResult{}, fmt.Errorf("configuration committed; stop intent cleanup failed")
	}
	if err := os.Remove(journalPath); err != nil {
		return ReconfigureResult{}, fmt.Errorf("configuration applied; journal cleanup failed, rerun for recovery")
	}
	return ReconfigureResult{revision + 1, options.PublicAPIURL}, nil
}

func validateConfigurationJournal(j configurationJournal, plan InstallPlan, doc config.RuntimeEnvDocument) error {
	if j.OldProof.Validate() != nil || j.NewProof.Validate() != nil || j.OldProof.OperationID != j.NewProof.OperationID || j.OldProof.InstallationID != doc.Values["INSTALLATION_ID"] || j.NewProof.InstallationID != j.OldProof.InstallationID || j.NewProof.ConfigRevision != j.OldProof.ConfigRevision+1 {
		return fmt.Errorf("invalid journal proofs")
	}
	old, err := config.ParseRuntimeEnv(j.OldRuntime)
	if err != nil {
		return err
	}
	next, err := config.ParseRuntimeEnv(j.NewRuntime)
	if err != nil {
		return err
	}
	for _, pair := range []struct {
		v map[string]string
		p setup.CommitProof
	}{{old.Values, j.OldProof}, {next.Values, j.NewProof}} {
		d, e := setup.CanonicalRequestDigest(pair.v, j.OldBinding.AdminEmail)
		if e != nil || d != pair.p.RequestDigest || pair.v["INSTALLATION_ID"] != pair.p.InstallationID || pair.v["CONFIG_REVISION"] != strconv.Itoa(pair.p.ConfigRevision) {
			return fmt.Errorf("journal digest mismatch")
		}
	}
	if j.OldBinding.OperationID != j.OldProof.OperationID || j.OldBinding.InstallationID != j.OldProof.InstallationID || j.OldBinding.RequestDigest != j.OldProof.RequestDigest || j.OldBinding.ConfigRevision != j.OldProof.ConfigRevision {
		return fmt.Errorf("journal binding mismatch")
	}
	if len(old.Values) != len(next.Values) {
		return fmt.Errorf("journal runtime fields differ")
	}
	var before, after deploymentManifest
	if json.Unmarshal(j.OldManifest, &before) != nil || json.Unmarshal(j.NewManifest, &after) != nil || before.InstallationID != j.OldProof.InstallationID || after.InstallationID != before.InstallationID || ValidateInstallPlan(before.Plan) != nil || ValidateInstallPlan(after.Plan) != nil || after.Plan.PublicAPIURL != next.Values["PUBLIC_API_URL"] {
		return fmt.Errorf("journal manifest invalid")
	}
	after.Plan.PublicAPIURL = before.Plan.PublicAPIURL
	beforeBytes, _ := json.Marshal(before)
	afterBytes, _ := json.Marshal(after)
	if !bytes.Equal(beforeBytes, afterBytes) {
		return fmt.Errorf("journal changed unrelated deployment fields")
	}
	for k, v := range old.Values {
		if k != "PUBLIC_API_URL" && k != "CONFIG_REVISION" && next.Values[k] != v {
			return fmt.Errorf("journal changed unrelated configuration")
		}
	}
	return nil
}
