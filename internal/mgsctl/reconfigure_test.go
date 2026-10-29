package mgsctl

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	"github.com/fatballfish/pic-gallery/internal/repository/db"
	"github.com/fatballfish/pic-gallery/internal/setup"
)

type reconfigureTestStore struct {
	binding  setup.SetupBinding
	calls    int
	conflict bool
}

func (s *reconfigureTestStore) Close() error { return nil }
func (s *reconfigureTestStore) Initialize(context.Context, setup.SetupInitializationRequest) (setup.SetupBinding, error) {
	panic("must not initialize")
}
func (s *reconfigureTestStore) MigrationCompleted(context.Context, db.SchemaVersion) (bool, error) {
	panic("must not migrate")
}
func (s *reconfigureTestStore) GetBinding(context.Context, string) (setup.SetupBinding, error) {
	return s.binding, nil
}
func (s *reconfigureTestStore) CompareSwapConfiguration(_ context.Context, u setup.SetupBindingConfigUpdate) (setup.SetupBinding, error) {
	s.calls++
	if s.conflict || s.binding != u.Expected {
		return setup.SetupBinding{}, setup.ErrSetupBindingMismatch
	}
	s.binding.ConfigRevision = u.ConfigRevision
	s.binding.RequestDigest = u.RequestDigest
	return s.binding, nil
}

func reconfigureFixture(t *testing.T) (string, *reconfigureTestStore, ReconfigureDependencies) {
	t.Helper()
	dir := t.TempDir()
	plan, err := BuildInstallPlan(InstallInput{Mode: config.DeploymentModeDocker, Profile: config.DeploymentProfileFull, Topology: config.DeploymentTopologySingle, Role: config.DeploymentRoleSingle, RuntimeDir: dir, ApplicationVersion: "v1.0.0", ImageTag: "v1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := BuildRuntimeArtifacts(plan, strings.NewReader(strings.Repeat("a", 4096)), time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	doc, err := config.ParseRuntimeEnv(artifacts.RuntimeEnv)
	if err != nil {
		t.Fatal(err)
	}
	doc.Values["SETUP_COMPLETED"] = "true"
	doc.Values["SETUP_TOKEN"] = ""
	doc.Values["CONFIG_REVISION"] = "1"
	digest, err := setup.CanonicalRequestDigest(doc.Values, "root@example.test")
	if err != nil {
		t.Fatal(err)
	}
	proof := setup.CommitProof{OperationID: "a6246539-68f3-4b91-a827-52ece3667425", InstallationID: artifacts.InstallState.InstallationID, RuntimeSchemaVersion: config.CurrentRuntimeSchemaVersion, ConfigRevision: 1, RequestDigest: digest}
	state := artifacts.InstallState
	state.Phase = setup.InstallPhaseCompleted
	state.EverCompleted = true
	state.Commit = &proof
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0700); err != nil {
		t.Fatal(err)
	}
	// Initialize accepts only pending; finalize through the guarded setup transitions.
	pending := artifacts.InstallState
	ss := setup.NewStateStore(filepath.Join(dir, "config", "runtime.env"))
	if err := ss.Initialize(pending); err != nil {
		t.Fatal(err)
	}
	if _, err := ss.BeginAttempt(setup.SetupAttempt{OperationID: proof.OperationID, ConfigRevision: 1, RequestDigest: digest, AdminCredentialVerifier: strings.Repeat("a", 64)}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := ss.BeginCommit(proof, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := ss.FinalizeCommit(proof, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	rendered, err := config.RenderRuntimeEnv(config.DefaultRuntimeSchema(), doc.Values, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"config/runtime.env": rendered, "deployment.json": artifacts.Manifest} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	s := &reconfigureTestStore{binding: setup.SetupBinding{OperationID: proof.OperationID, InstallationID: proof.InstallationID, ConfigRevision: 1, RequestDigest: digest, AdminID: 1, AdminEmail: "root@example.test"}}
	deps := ReconfigureDependencies{OpenStore: func(context.Context, string) (setup.SetupStoreSession, error) { return s, nil }, StopDeployment: func(context.Context, InstallPlan) error { return nil }, ApplyDeployment: func(context.Context, InstallPlan) error { return nil }}
	return dir, s, deps
}
func TestReconfigureAdvancesProofAndIsIdempotent(t *testing.T) {
	dir, s, deps := reconfigureFixture(t)
	opts := ReconfigureOptions{dir, "https://mikiko.studio"}
	result, err := Reconfigure(t.Context(), opts, deps)
	if err != nil {
		t.Fatal(err)
	}
	if result.ConfigRevision != 2 || s.binding.ConfigRevision != 2 {
		t.Fatal("revision not advanced")
	}
	_, doc, err := loadInstallation(dir)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := setup.CanonicalRequestDigest(doc.Values, s.binding.AdminEmail)
	state, _, _ := setup.NewStateStore(filepath.Join(dir, "config", "runtime.env")).Load()
	if state.Commit.RequestDigest != d || s.binding.RequestDigest != d {
		t.Fatal("inconsistent proof")
	}
	if _, err := Reconfigure(t.Context(), opts, deps); err != nil {
		t.Fatal(err)
	}
	if s.calls != 1 {
		t.Fatal("idempotent operation wrote binding")
	}
}
func TestReconfigureRollsBackFailedRollAndRejectsTampering(t *testing.T) {
	t.Run("roll failure", func(t *testing.T) {
		dir, s, deps := reconfigureFixture(t)
		old := s.binding
		calls := 0
		deps.ApplyDeployment = func(context.Context, InstallPlan) error {
			calls++
			if calls == 1 {
				return errors.New("do not print secret")
			}
			return nil
		}
		_, err := Reconfigure(t.Context(), ReconfigureOptions{dir, "https://mikiko.studio"}, deps)
		if err == nil || strings.Contains(err.Error(), "secret") {
			t.Fatal("unsafe error")
		}
		if s.binding != old || calls != 2 {
			t.Fatal("rollback failed")
		}
		_, doc, _ := loadInstallation(dir)
		if doc.Values["PUBLIC_API_URL"] != "" {
			t.Fatal("runtime not restored")
		}
	})
	t.Run("tamper", func(t *testing.T) {
		dir, s, deps := reconfigureFixture(t)
		s.binding.RequestDigest = strings.Repeat("b", 64)
		stopped := false
		deps.StopDeployment = func(context.Context, InstallPlan) error { stopped = true; return nil }
		if _, err := Reconfigure(t.Context(), ReconfigureOptions{dir, "https://mikiko.studio"}, deps); err == nil || stopped || s.calls != 0 {
			t.Fatal("tampered binding accepted")
		}
	})
	t.Run("CAS conflict", func(t *testing.T) {
		dir, s, deps := reconfigureFixture(t)
		s.conflict = true
		if _, err := Reconfigure(t.Context(), ReconfigureOptions{dir, "https://mikiko.studio"}, deps); err == nil {
			t.Fatal("CAS conflict accepted")
		}
		_, doc, _ := loadInstallation(dir)
		if doc.Values["CONFIG_REVISION"] != "1" {
			t.Fatal("conflict changed runtime")
		}
	})
}
func TestReconfigureURLValidation(t *testing.T) {
	for _, raw := range []string{"", "http://mikiko.studio", "https://user:password@mikiko.studio", "https://mikiko.studio/api", "https://mikiko.studio?", "https://mikiko.studio#bad", "https://mikiko.studio:70000"} {
		if validateCanonicalReconfigureURL(raw) == nil {
			t.Fatalf("accepted invalid origin %q", raw)
		}
	}
}
