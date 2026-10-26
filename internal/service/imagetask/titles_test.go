package imagetask

import (
	"context"
	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"testing"
	"time"
)

type titleFunc func(context.Context, string) string

func (f titleFunc) Generate(ctx context.Context, prompt string) string { return f(ctx, prompt) }
func TestImageTitlesOncePerTaskAndRecoveryPreservesNames(t *testing.T) {
	calls := 0
	svc := &Service{now: time.Now}
	svc.SetTitleGenerator(titleFunc(func(context.Context, string) string { calls++; return "统一标题" }))
	results := []provider.ImageResult{{}, {}}
	svc.nameImageResults(t.Context(), domainimagetask.Task{Prompt: "prompt"}, results)
	svc.nameImageResults(t.Context(), domainimagetask.Task{Prompt: "prompt"}, results)
	if calls != 1 || results[0].Name != "统一标题" || results[1].Name != results[0].Name {
		t.Fatalf("calls=%d results=%+v", calls, results)
	}
}
func TestTitleEnrichmentRunsAfterDurableUpstreamCheckpoint(t *testing.T) {
	svc, store, _, providerCalls, now := newArtifactRecoveryTestService(t, 1)
	ctx := t.Context()
	created, err := svc.CreateTask(ctx, artifactRecoveryCreateRequest())
	if err != nil {
		t.Fatal(err)
	}
	titleCalls := 0
	svc.SetTitleGenerator(titleFunc(func(ctx context.Context, _ string) string {
		titleCalls++
		loaded, err := store.GetByID(ctx, created.UserID, created.ID)
		if err != nil || loaded.UpstreamSucceededAt == nil || loaded.ArtifactRecovery.EncryptedPayload == "" {
			t.Fatalf("checkpoint missing before title: %+v %v", loaded, err)
		}
		return "已保存的标题"
	}))
	claimed, ok, err := svc.AcquireNextTask(ctx, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim %v %v", ok, err)
	}
	pending, err := svc.ExecuteLeasedTask(ctx, claimed, "worker", []string{"openrouter"})
	if err != nil {
		t.Fatal(err)
	}
	*now = pending.Task.ArtifactRecovery.NextRetryAt.Add(time.Millisecond)
	claimed, ok, err = svc.AcquireNextTask(ctx, "worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("reclaim %v %v", ok, err)
	}
	result, err := svc.ExecuteLeasedTask(ctx, claimed, "worker", []string{"openrouter"})
	if err != nil {
		t.Fatal(err)
	}
	if titleCalls != 1 || *providerCalls != 1 || len(result.Task.Results) != 1 || result.Task.Results[0].Name != "已保存的标题" {
		t.Fatalf("titleCalls=%d imageCalls=%d results=%+v", titleCalls, *providerCalls, result.Task.Results)
	}
}

func TestImageTitlesFallbackPreservesHistoricalNames(t *testing.T) {
	svc := &Service{now: time.Now}
	results := []provider.ImageResult{{Name: "historical.png"}, {}, {}}
	svc.nameImageResults(t.Context(), domainimagetask.Task{}, results)
	if results[0].Name != "historical.png" || len(results[1].Name) != 14 || results[1].Name == results[2].Name {
		t.Fatalf("%+v", results)
	}
}
