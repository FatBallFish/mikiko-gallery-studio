package imagetask

import (
	"context"
	"testing"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
)

type forbiddenMaskLoader struct{ t *testing.T }

func (l forbiddenMaskLoader) LoadInput(_ int64, _ string) (provider.ImageInput, error) {
	l.t.Fatal("checkpoint recovery must not load mask or call upstream again")
	return provider.ImageInput{}, nil
}

func TestDurableMaskRecoverySkipsInputReload(t *testing.T) {
	svc, store, _, providerCalls, now := newArtifactRecoveryTestService(t, 1)
	ctx := context.Background()
	_, err := svc.CreateTask(ctx, artifactRecoveryCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	claimed, ok, err := svc.AcquireNextTask(ctx, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	pending, err := svc.ExecuteLeasedTask(ctx, claimed, "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	// A now unavailable mask must not prevent recovery of already paid outputs.
	pending.Task.MaskAssetID = "expired-mask"
	if err := store.Save(ctx, pending.Task); err != nil {
		t.Fatal(err)
	}
	svc.assets = forbiddenMaskLoader{t: t}
	*now = pending.Task.ArtifactRecovery.NextRetryAt.Add(time.Millisecond)
	reclaimed, ok, err := svc.AcquireNextTask(ctx, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaim: %v %v", ok, err)
	}
	result, err := svc.ExecuteLeasedTask(ctx, reclaimed, "worker", nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Task.Status != domainimagetask.StatusSucceeded || *providerCalls != 1 {
		t.Fatalf("recovery regenerated outputs: status=%s provider calls=%d", result.Task.Status, *providerCalls)
	}
}
