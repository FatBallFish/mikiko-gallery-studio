package imagetask_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
)

type maskOwnerLoader struct {
	calls       []string
	unavailable bool
}

func (l *maskOwnerLoader) LoadInput(userID int64, id string) (provider.ImageInput, error) {
	l.calls = append(l.calls, id)
	if userID != 72 || l.unavailable {
		return provider.ImageInput{}, errors.New("asset is not owned or ready")
	}
	return provider.ImageInput{Filename: id + ".png", MIMEType: "image/png", Data: []byte("private-mask-binary https://private.example/mask?token=secret")}, nil
}

func TestDurableMaskWorkerAndRetry(t *testing.T) {
	loader := &maskOwnerLoader{}
	calls := 0
	providers := map[string]provider.ImageProvider{"openai": fakeProvider{editFunc: func(_ context.Context, req provider.ImageRequest) (provider.ImageResponse, error) {
		calls++
		if req.Mask == nil || string(req.Mask.Data) != "private-mask-binary https://private.example/mask?token=secret" || req.Mask.Filename != "mask.png" {
			t.Fatalf("missing mask: %#v", req.Mask)
		}
		return provider.ImageResponse{}, errors.New("upstream test failure")
	}}}
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithProvidersStoreAndAssets(taskTestConfig(), providers, store, loader)
	task, err := svc.CreateTask(context.Background(), domainimagetask.CreateRequest{UserID: 72, AbstractModel: "plus", TaskType: string(provider.TaskTypeImageEdit), Prompt: "edit", SizeMode: "auto", OutputImageCount: 1, ReferenceAssetIDs: []string{"ref"}, MaskAssetID: " mask ", RequestFingerprint: "original-fingerprint"})
	if err != nil {
		t.Fatal(err)
	}
	if task.MaskAssetID != "mask" || task.RequestFingerprint != "original-fingerprint" {
		t.Fatalf("bad input snapshot: %#v", task)
	}
	payload, _ := json.Marshal(task)
	for _, secret := range []string{"private-mask-binary", "private.example", "token=secret", "original-fingerprint"} {
		if strings.Contains(string(payload), secret) {
			t.Fatalf("task persisted secret %q", secret)
		}
	}
	claimed, ok, err := svc.AcquireNextTask(context.Background(), "mask-worker", time.Minute)
	if err != nil || !ok {
		t.Fatalf("claim: %v %v", ok, err)
	}
	_, _ = svc.ExecuteLeasedTask(context.Background(), claimed, "mask-worker", nil)
	if calls != 1 {
		t.Fatalf("expected one masked edit, got %d", calls)
	}
	retry, err := svc.RetryTask(context.Background(), 72, task.ID, domainimagetask.RetryRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if retry.MaskAssetID != "mask" || retry.ID == task.ID || retry.RequestFingerprint != "" {
		t.Fatalf("retry snapshot: %#v", retry)
	}
}

func TestDurableMaskPreflightRejectsUnownedOrUnready(t *testing.T) {
	for _, userID := range []int64{71, 72} {
		loader := &maskOwnerLoader{unavailable: userID == 72}
		svc := imagetask.NewServiceWithStoreAndAssets(taskTestConfig(), imagetask.NewMemoryStore(), loader)
		_, err := svc.Preflight(context.Background(), domainimagetask.CreateRequest{UserID: userID, AbstractModel: "plus", TaskType: string(provider.TaskTypeImageEdit), Prompt: "edit", SizeMode: "auto", OutputImageCount: 1, MaskAssetID: "mask"})
		if err == nil || !strings.Contains(err.Error(), "not owned or ready") {
			t.Fatalf("expected mask validation error, got %v", err)
		}
		if len(loader.calls) != 1 || loader.calls[0] != "mask" {
			t.Fatalf("mask not validated: %#v", loader.calls)
		}
	}
}
