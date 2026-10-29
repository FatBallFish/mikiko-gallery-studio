package mgsctl

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReconfigureRecoversPublishedCrashWithoutDowngrade(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "before commit marker", true: "after commit marker"}[committed], func(t *testing.T) {
			dir, s, deps := reconfigureFixture(t)
			path := filepath.Join(dir, "config", ".mgsctl-reconfigure.json")
			var journal []byte
			deps.ApplyDeployment = func(context.Context, InstallPlan) error { journal, _ = os.ReadFile(path); return nil }
			if _, err := Reconfigure(t.Context(), ReconfigureOptions{dir, "https://mikiko.studio"}, deps); err != nil {
				t.Fatal(err)
			}
			var j configurationJournal
			if json.Unmarshal(journal, &j) != nil {
				t.Fatal("missing journal")
			}
			j.Committed = committed
			journal, _ = json.Marshal(j)
			if err := os.WriteFile(path, journal, 0600); err != nil {
				t.Fatal(err)
			}
			stopped := false
			deps.StopDeployment = func(context.Context, InstallPlan) error { stopped = true; return nil }
			deps.ApplyDeployment = func(context.Context, InstallPlan) error { return nil }
			result, err := Reconfigure(t.Context(), ReconfigureOptions{dir, "https://mikiko.studio"}, deps)
			if err != nil {
				t.Fatal(err)
			}
			if result.ConfigRevision != 2 || s.binding.ConfigRevision != 2 || stopped {
				t.Fatal("published commit downgraded")
			}
		})
	}
}
func TestReconfigureRecordsStopIntentAndRefreshesSameURL(t *testing.T) {
	dir, s, deps := reconfigureFixture(t)
	opts := ReconfigureOptions{dir, "https://mikiko.studio"}
	if _, err := Reconfigure(t.Context(), opts, deps); err != nil {
		t.Fatal(err)
	}
	refreshed := false
	restarted := false
	deps.StopDeployment = func(context.Context, InstallPlan) error {
		if _, err := os.Stat(filepath.Join(dir, "config", ".mgsctl-reconfigure-stop.json")); err != nil {
			t.Fatal("stop intent absent")
		}
		return nil
	}
	deps.RefreshAssets = func(context.Context, InstallPlan) error { refreshed = true; return nil }
	deps.ApplyDeployment = func(context.Context, InstallPlan) error { restarted = true; return nil }
	if _, err := Reconfigure(t.Context(), opts, deps); err != nil {
		t.Fatal(err)
	}
	if !refreshed || !restarted || s.binding.ConfigRevision != 2 {
		t.Fatal("same canonical did not refresh safely")
	}
	deps.StopDeployment = func(context.Context, InstallPlan) error { return errors.New("partial stop") }
	restarted = false
	if _, err := Reconfigure(t.Context(), ReconfigureOptions{dir, "https://other.example"}, deps); err == nil || !restarted {
		t.Fatal("partial stop not recovered")
	}
}
