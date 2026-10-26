package imagetask_test

import (
	"context"
	"errors"
	domainassets "github.com/fatballfish/pic-gallery/internal/domain/assets"
	image "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/provider"
	"github.com/fatballfish/pic-gallery/internal/service/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
	"testing"
)

type deniedPreflightAsset struct{ err error }

func (a deniedPreflightAsset) LoadInput(int64, string) (provider.ImageInput, error) {
	return provider.ImageInput{}, a.err
}
func TestOpenPlatformPreflightCreateReferenceParity(t *testing.T) {
	denied := errors.New("reference belongs to another user")
	store := imagetask.NewMemoryStore()
	svc := imagetask.NewServiceWithStoreAndAssets(taskTestConfig(), store, deniedPreflightAsset{denied})
	req := image.CreateRequest{UserID: 1, AbstractModel: "plus", TaskType: "image_edit", Prompt: "edit this", SizeMode: "auto", ReferenceAssetIDs: []string{"foreign"}, ReferenceImageCount: 999, OutputImageCount: 1}
	if _, err := svc.Preflight(context.Background(), req); !errors.Is(err, denied) {
		t.Fatalf("preflight error %v", err)
	}
	if tasks, err := svc.ListByUser(context.Background(), 1); err != nil || len(tasks) != 0 {
		t.Fatalf("preflight persisted task: %v %v", tasks, err)
	}
	if _, err := svc.CreateTask(context.Background(), req); !errors.Is(err, denied) {
		t.Fatalf("create parity error %v", err)
	}
}

type metadataPreflightAsset struct {
	binaryLoads, metadataLoads int
	denied                     bool
}

func (a *metadataPreflightAsset) LoadInput(int64, string) (provider.ImageInput, error) {
	a.binaryLoads++
	return provider.ImageInput{}, errors.New("binary loading forbidden in estimate")
}
func (a *metadataPreflightAsset) GetMetadata(context.Context, int64, string) (domainassets.ReferenceAsset, error) {
	a.metadataLoads++
	if a.denied {
		return domainassets.ReferenceAsset{}, errs.New(404, errs.CodeNotFound, "reference not found")
	}
	return domainassets.ReferenceAsset{ID: "ref", Status: "ready"}, nil
}
func TestCanvasMetadataPreflightNeverLoadsBinary(t *testing.T) {
	loader := &metadataPreflightAsset{}
	svc := imagetask.NewServiceWithStoreAndAssets(taskTestConfig(), imagetask.NewMemoryStore(), loader)
	req := image.CreateRequest{UserID: 1, AbstractModel: "plus", TaskType: "image_edit", Prompt: "edit this", SizeMode: "auto", ReferenceAssetIDs: []string{"ref"}, ReferenceImageCount: 999, OutputImageCount: 1}
	// A quote may fail downstream if the test has no billable route; metadata
	// validation still must never invoke the binary loader.
	_, _ = svc.PreflightMetadata(context.Background(), req)
	if loader.binaryLoads != 0 || loader.metadataLoads != 1 {
		t.Fatalf("loads binary=%d metadata=%d", loader.binaryLoads, loader.metadataLoads)
	}
	loader.denied = true
	_, err := svc.PreflightMetadata(context.Background(), req)
	var typed *errs.Error
	if !errors.As(err, &typed) || typed.StatusCode != 404 || typed.Details["field"] != "reference_asset_ids" {
		t.Fatalf("metadata ownership error: %#v", err)
	}
	loader.denied = false
	if _, err := svc.CreateTask(context.Background(), req); err == nil || loader.binaryLoads != 1 {
		t.Fatalf("create must retain binary validation: loads=%d err=%v", loader.binaryLoads, err)
	}
}
