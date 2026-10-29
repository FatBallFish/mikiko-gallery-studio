package mgsctl

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func managedNginxFixture(t *testing.T) (string, []byte, []byte) {
	t.Helper()
	dir := setupOperationsFixture(t, true)
	path := filepath.Join(dir, filepath.FromSlash(managedNginxPath))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	old := []byte("# historical generated gateway without protocol routes\n")
	if err := os.WriteFile(path, old, 0o644); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dir, "deployment.json")
	raw, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest deploymentManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	manifest.Plan.APIPort = "8888"
	manifest.Files[managedNginxPath] = fmtNginxHash(old)
	raw, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	return dir, old, raw
}

func fmtNginxHash(content []byte) string {
	const hex = "0123456789abcdef"
	digest := sha256.Sum256(content)
	encoded := make([]byte, 64)
	for i, b := range digest {
		encoded[2*i], encoded[2*i+1] = hex[b>>4], hex[b&15]
	}
	return string(encoded)
}

func TestRefreshManagedNginxUpdatesOwnedAssetAndOnlyItsHash(t *testing.T) {
	dir, _, oldManifest := managedNginxFixture(t)
	validated, applied := 0, 0
	deps := ManagedNginxDependencies{
		Validate: func(ctx context.Context, plan InstallPlan) error {
			validated++
			if plan.RuntimeDir != dir {
				t.Fatalf("runtime directory = %q", plan.RuntimeDir)
			}
			candidate, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)))
			if err != nil {
				t.Fatal(err)
			}
			for _, route := range []string{"server api:8888;", "location = /mcp {", "location = /oauth/token {", "location = /.well-known/oauth-protected-resource/mcp {", "proxy_buffering off;"} {
				if !bytes.Contains(candidate, []byte(route)) {
					t.Errorf("missing route/config %q", route)
				}
			}
			current, _ := os.ReadFile(filepath.Join(dir, "deployment.json"))
			if !bytes.Equal(current, oldManifest) {
				t.Fatal("manifest changed before validation")
			}
			return nil
		},
		Apply: func(context.Context, InstallPlan) error { applied++; return nil },
	}
	if err := RefreshManagedNginx(context.Background(), dir, deps); err != nil {
		t.Fatal(err)
	}
	if validated != 1 || applied != 1 {
		t.Fatalf("operations = %d/%d", validated, applied)
	}
	raw, _ := os.ReadFile(filepath.Join(dir, "deployment.json"))
	var before, after deploymentManifest
	_ = json.Unmarshal(oldManifest, &before)
	_ = json.Unmarshal(raw, &after)
	asset, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)))
	if after.Files[managedNginxPath] != fmtNginxHash(asset) {
		t.Fatal("manifest hash differs from deployed asset")
	}
	after.Files[managedNginxPath] = before.Files[managedNginxPath]
	a, _ := json.Marshal(after)
	b, _ := json.Marshal(before)
	if !bytes.Equal(a, b) {
		t.Fatal("unrelated manifest metadata changed")
	}
	if err := RefreshManagedNginx(context.Background(), dir, deps); err != nil {
		t.Fatal(err)
	}
	if validated != 1 || applied != 1 {
		t.Fatal("identical asset was unnecessarily redeployed")
	}
}

func TestRefreshManagedNginxRefusesCustomizationAndMissingProof(t *testing.T) {
	for _, variant := range []string{"customized", "untracked", "symlink", "asset directory symlink", "trailing manifest"} {
		t.Run(variant, func(t *testing.T) {
			dir, _, _ := managedNginxFixture(t)
			path := filepath.Join(dir, filepath.FromSlash(managedNginxPath))
			switch variant {
			case "customized":
				_ = os.WriteFile(path, []byte("# custom routing"), 0o644)
			case "untracked":
				raw, _ := os.ReadFile(filepath.Join(dir, "deployment.json"))
				var m deploymentManifest
				_ = json.Unmarshal(raw, &m)
				delete(m.Files, managedNginxPath)
				raw, _ = json.Marshal(m)
				_ = os.WriteFile(filepath.Join(dir, "deployment.json"), raw, 0o600)
			case "symlink":
				target := filepath.Join(t.TempDir(), "custom.conf")
				_ = os.WriteFile(target, []byte("custom"), 0o600)
				_ = os.Remove(path)
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			case "asset directory symlink":
				target := filepath.Join(dir, "moved-assets")
				if err := os.Rename(filepath.Dir(path), target); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, filepath.Dir(path)); err != nil {
					t.Fatal(err)
				}
			case "trailing manifest":
				f, err := os.OpenFile(filepath.Join(dir, "deployment.json"), os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = f.WriteString(" {}")
				_ = f.Close()
			}
			called := false
			err := RefreshManagedNginx(context.Background(), dir, ManagedNginxDependencies{Validate: func(context.Context, InstallPlan) error { called = true; return nil }})
			if err == nil || called {
				t.Fatalf("expected refusal before validation, err=%v called=%v", err, called)
			}
		})
	}
}

func TestRefreshManagedNginxRestoresOnValidationApplyAndCancellation(t *testing.T) {
	for _, phase := range []string{"validate", "apply", "cancel"} {
		t.Run(phase, func(t *testing.T) {
			dir, old, manifest := managedNginxFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			failure := errors.New("injected gateway failure")
			applies := 0
			deps := ManagedNginxDependencies{
				Validate: func(context.Context, InstallPlan) error {
					if phase == "validate" {
						return failure
					}
					if phase == "cancel" {
						cancel()
					}
					return nil
				},
				Apply: func(recovery context.Context, _ InstallPlan) error {
					applies++
					if applies == 1 {
						cancel()
						return failure
					}
					if recovery.Err() != nil {
						t.Fatal("recovery used cancelled context")
					}
					current, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)))
					if !bytes.Equal(current, old) {
						t.Fatal("gateway recovery ran before old asset restoration")
					}
					return nil
				},
			}
			err := RefreshManagedNginx(ctx, dir, deps)
			if phase == "cancel" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if !errors.Is(err, failure) {
				t.Fatal(err)
			}
			asset, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)))
			current, _ := os.ReadFile(filepath.Join(dir, "deployment.json"))
			if !bytes.Equal(asset, old) || !bytes.Equal(current, manifest) {
				t.Fatal("old asset/manifest not restored exactly")
			}
			if phase == "apply" && applies != 2 {
				t.Fatalf("apply calls=%d", applies)
			}
			if phase != "apply" && applies != 0 {
				t.Fatal("gateway touched on validation failure")
			}
		})
	}
}

func TestRefreshManagedNginxLockedSupportsStoppedServices(t *testing.T) {
	dir, _, _ := managedNginxFixture(t)
	release, err := acquireInstallLock(context.Background(), filepath.Join(dir, "config", ".mgsctl-install.lock"))
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := refreshManagedNginxLocked(context.Background(), dir, ManagedNginxDependencies{Validate: func(context.Context, InstallPlan) error { return nil }}); err != nil {
		t.Fatal(err)
	}
}

func TestRefreshManagedNginxRequiresValidationAndPropagatesRecoveryFailure(t *testing.T) {
	if err := RefreshManagedNginx(context.Background(), t.TempDir(), ManagedNginxDependencies{}); err == nil {
		t.Fatal("missing validation accepted")
	}
	dir, _, _ := managedNginxFixture(t)
	err := RefreshManagedNginx(context.Background(), dir, ManagedNginxDependencies{
		Validate: func(context.Context, InstallPlan) error { return nil },
		Apply:    func(context.Context, InstallPlan) error { return errors.New("gateway unavailable") },
	})
	if err == nil || !strings.Contains(err.Error(), "restore previous managed nginx deployment") {
		t.Fatalf("recovery error lost: %v", err)
	}
}

// Panic intentionally bypasses ordinary error rollback, leaving precisely the
// durable state a process exit would leave while releasing the test's lock.
func interruptNginxRefresh(t *testing.T, dir, phase string) {
	t.Helper()
	defer func() {
		if recover() != "simulated process exit" {
			t.Fatal("refresh did not reach crash point")
		}
	}()
	deps := ManagedNginxDependencies{
		Validate: func(context.Context, InstallPlan) error {
			if phase == "candidate" {
				panic("simulated process exit")
			}
			return nil
		},
		Apply: func(context.Context, InstallPlan) error { panic("simulated process exit") },
	}
	_ = RefreshManagedNginx(context.Background(), dir, deps)
}

func TestRefreshManagedNginxRecoversCandidateAndManifestCrash(t *testing.T) {
	for _, phase := range []string{"candidate", "manifest"} {
		t.Run(phase, func(t *testing.T) {
			dir, _, oldManifest := managedNginxFixture(t)
			interruptNginxRefresh(t, dir, phase)
			raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxJournalPath)))
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(filepath.Join(dir, filepath.FromSlash(managedNginxJournalPath)))
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatal("journal is not private")
			}
			var j managedNginxJournal
			if err := json.Unmarshal(raw, &j); err != nil {
				t.Fatal(err)
			}
			manifest, _ := os.ReadFile(filepath.Join(dir, "deployment.json"))
			if phase == "candidate" && !bytes.Equal(manifest, oldManifest) {
				t.Fatal("manifest changed before candidate validation")
			}
			if phase == "manifest" && !bytes.Equal(manifest, j.NewManifest) {
				t.Fatal("manifest crash point not reached")
			}
			validates, applies := 0, 0
			err = RefreshManagedNginx(context.Background(), dir, ManagedNginxDependencies{
				Validate: func(context.Context, InstallPlan) error { validates++; return nil },
				Apply:    func(context.Context, InstallPlan) error { applies++; return nil },
			})
			if err != nil {
				t.Fatal(err)
			}
			if validates != 1 || applies != 1 {
				t.Fatal("interrupted refresh was not validated and reapplied")
			}
			asset, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)))
			manifest, _ = os.ReadFile(filepath.Join(dir, "deployment.json"))
			if !bytes.Equal(asset, j.NewAsset) || !bytes.Equal(manifest, j.NewManifest) {
				t.Fatal("recovery did not complete known new state")
			}
			if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(managedNginxJournalPath))); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("journal retained after successful recovery")
			}
		})
	}
}

func TestRefreshManagedNginxCleanupFailureNeverReversesCompletedRefresh(t *testing.T) {
	dir, old, _ := managedNginxFixture(t)
	failure := errors.New("cleanup blocked")
	validates, applies := 0, 0
	deps := ManagedNginxDependencies{
		Validate:      func(context.Context, InstallPlan) error { validates++; return nil },
		Apply:         func(context.Context, InstallPlan) error { applies++; return nil },
		RemoveJournal: func(string) error { return failure },
	}
	err := RefreshManagedNginx(context.Background(), dir, deps)
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	asset, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)))
	if bytes.Equal(asset, old) {
		t.Fatal("successful refresh reversed on cleanup failure")
	}
	raw, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxJournalPath)))
	var j managedNginxJournal
	_ = json.Unmarshal(raw, &j)
	if j.Phase != "completed" {
		t.Fatal("completed refresh phase not durable")
	}
	deps.RemoveJournal = nil
	if err := RefreshManagedNginx(context.Background(), dir, deps); err != nil {
		t.Fatal(err)
	}
	if validates != 1 || applies != 1 {
		t.Fatal("cleanup retry reran or reversed completed deployment")
	}
}

func TestRefreshManagedNginxRecoveryRejectsExternalTamper(t *testing.T) {
	for _, target := range []string{"asset", "manifest", "journal candidate", "journal old hash"} {
		t.Run(target, func(t *testing.T) {
			dir, _, _ := managedNginxFixture(t)
			interruptNginxRefresh(t, dir, "candidate")
			path := filepath.Join(dir, filepath.FromSlash(managedNginxPath))
			if target == "manifest" {
				path = filepath.Join(dir, "deployment.json")
			}
			if strings.HasPrefix(target, "journal") {
				path = filepath.Join(dir, filepath.FromSlash(managedNginxJournalPath))
				raw, _ := os.ReadFile(path)
				var j managedNginxJournal
				_ = json.Unmarshal(raw, &j)
				if target == "journal candidate" {
					j.NewAsset = []byte("# untrusted candidate")
				} else {
					j.OldAsset = []byte("# false ownership")
				}
				raw, _ = json.Marshal(j)
				if err := os.WriteFile(path, raw, 0o600); err != nil {
					t.Fatal(err)
				}
			} else {
				f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = f.WriteString("# external edit")
				_ = f.Close()
			}
			before, _ := os.ReadFile(path)
			called := false
			err := RefreshManagedNginx(context.Background(), dir, ManagedNginxDependencies{Validate: func(context.Context, InstallPlan) error { called = true; return nil }})
			if err == nil || called {
				t.Fatalf("tamper accepted err=%v called=%v", err, called)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("external edit overwritten")
			}
		})
	}
}

func TestRefreshManagedNginxResumesInterruptedRollback(t *testing.T) {
	dir, old, oldManifest := managedNginxFixture(t)
	interruptNginxRefresh(t, dir, "manifest")
	journalPath := filepath.Join(dir, filepath.FromSlash(managedNginxJournalPath))
	raw, _ := os.ReadFile(journalPath)
	var j managedNginxJournal
	_ = json.Unmarshal(raw, &j)
	j.Phase = "rollback"
	raw, _ = json.Marshal(j)
	if err := os.WriteFile(journalPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash halfway through restoring the pair.
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)), old, 0o644); err != nil {
		t.Fatal(err)
	}
	applied := 0
	err := RefreshManagedNginx(context.Background(), dir, ManagedNginxDependencies{
		Validate: func(context.Context, InstallPlan) error { t.Fatal("rollback must not validate candidate"); return nil },
		Apply:    func(context.Context, InstallPlan) error { applied++; return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "restored; rerun") {
		t.Fatal(err)
	}
	asset, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)))
	manifest, _ := os.ReadFile(filepath.Join(dir, "deployment.json"))
	if !bytes.Equal(asset, old) || !bytes.Equal(manifest, oldManifest) || applied != 1 {
		t.Fatal("interrupted rollback did not restore exact old state")
	}
}

func legacyNginxFixture(t *testing.T) (string, []byte, []byte) {
	t.Helper()
	dir, _, raw := managedNginxFixture(t)
	var m deploymentManifest
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	actual, err := materializeNginxConfig(m.Plan, nginx88114a0)
	if err != nil {
		t.Fatal(err)
	}
	recorded, err := materializeNginxConfig(m.Plan, nginx971e03c)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmtNginxHash(actual); got != "ed7e9a7b3a0cf43d8cea8beccfb8ec85b71d5b6826e553ee2c88e05ed5ffa562" {
		t.Fatalf("historical actual provenance hash=%s", got)
	}
	if got := fmtNginxHash(recorded); got != "1d7f8f26fadd35385efefedd747b8db90c5b4aff2ad9d53f67dd1d1fd26c5900" {
		t.Fatalf("historical manifest provenance hash=%s", got)
	}
	m.Files[managedNginxPath] = fmtNginxHash(recorded)
	raw, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "deployment.json"), raw, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)), actual, 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, actual, raw
}

func TestRefreshManagedNginxVerifiedLegacyPairAndRecovery(t *testing.T) {
	for _, phase := range []string{"normal", "candidate", "manifest"} {
		t.Run(phase, func(t *testing.T) {
			dir, _, _ := legacyNginxFixture(t)
			if phase != "normal" {
				interruptNginxRefresh(t, dir, phase)
			}
			if err := RefreshManagedNginx(context.Background(), dir, ManagedNginxDependencies{Validate: func(context.Context, InstallPlan) error { return nil }}); err != nil {
				t.Fatal(err)
			}
			asset, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)))
			raw, _ := os.ReadFile(filepath.Join(dir, "deployment.json"))
			var m deploymentManifest
			_ = json.Unmarshal(raw, &m)
			if m.Files[managedNginxPath] != fmtNginxHash(asset) || !bytes.Contains(asset, []byte("location = /mcp {")) {
				t.Fatal("legacy refresh did not publish current template and hash")
			}
		})
	}
}

func TestRefreshManagedNginxLegacyCompatibilityRejectsTamper(t *testing.T) {
	for _, variant := range []string{"asset byte", "unknown manifest hash", "different plan port", "journal old asset", "journal old hash"} {
		t.Run(variant, func(t *testing.T) {
			dir, asset, raw := legacyNginxFixture(t)
			manifestPath := filepath.Join(dir, "deployment.json")
			if strings.HasPrefix(variant, "journal") {
				interruptNginxRefresh(t, dir, "candidate")
				path := filepath.Join(dir, filepath.FromSlash(managedNginxJournalPath))
				data, _ := os.ReadFile(path)
				var j managedNginxJournal
				_ = json.Unmarshal(data, &j)
				if variant == "journal old asset" {
					j.OldAsset = append(j.OldAsset, ' ')
				} else {
					var m deploymentManifest
					_ = json.Unmarshal(j.OldManifest, &m)
					m.Files[managedNginxPath] = strings.Repeat("a", 64)
					j.OldManifest, _ = json.Marshal(m)
				}
				data, _ = json.Marshal(j)
				if err := os.WriteFile(path, data, 0o600); err != nil {
					t.Fatal(err)
				}
			} else if variant == "asset byte" {
				if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)), append(asset, ' '), 0o644); err != nil {
					t.Fatal(err)
				}
			} else {
				var m deploymentManifest
				_ = json.Unmarshal(raw, &m)
				if variant == "unknown manifest hash" {
					m.Files[managedNginxPath] = strings.Repeat("a", 64)
				} else {
					m.Plan.APIPort = "8889"
				}
				raw, _ = json.Marshal(m)
				if err := os.WriteFile(manifestPath, raw, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			validated := false
			err := RefreshManagedNginx(context.Background(), dir, ManagedNginxDependencies{Validate: func(context.Context, InstallPlan) error { validated = true; return nil }})
			if err == nil || validated {
				t.Fatalf("tampered legacy pair accepted err=%v validated=%v", err, validated)
			}
		})
	}
}

func TestRefreshManagedNginxLegacyRollbackRetainsOriginalDriftEvidence(t *testing.T) {
	dir, old, manifest := legacyNginxFixture(t)
	failure := errors.New("nginx validation failure")
	err := RefreshManagedNginx(context.Background(), dir, ManagedNginxDependencies{Validate: func(context.Context, InstallPlan) error { return failure }})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
	asset, _ := os.ReadFile(filepath.Join(dir, filepath.FromSlash(managedNginxPath)))
	current, _ := os.ReadFile(filepath.Join(dir, "deployment.json"))
	if !bytes.Equal(asset, old) || !bytes.Equal(current, manifest) {
		t.Fatal("legacy rollback rewrote original evidence")
	}
}
