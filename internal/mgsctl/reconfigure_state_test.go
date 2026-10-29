package mgsctl

import (
	"github.com/fatballfish/pic-gallery/internal/setup"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReconfigureStateCASRejectsStaleProof(t *testing.T) {
	dir, _, _ := reconfigureFixture(t)
	store := setup.NewStateStore(filepath.Join(dir, "config", "runtime.env"))
	state, _, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	old := *state.Commit
	next := old
	next.ConfigRevision++
	next.RequestDigest = strings.Repeat("c", 64)
	if _, err := store.CompareSwapCompletedCommit(old, next, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CompareSwapCompletedCommit(old, next, time.Now().UTC()); err == nil {
		t.Fatal("stale proof accepted")
	}
	if _, err := store.CompareSwapCompletedCommit(next, old, time.Now().UTC()); err != nil {
		t.Fatal("rollback CAS rejected")
	}
}
