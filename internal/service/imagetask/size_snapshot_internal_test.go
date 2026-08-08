package imagetask

import (
	"errors"
	"testing"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/domain/modelhub"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestValidateResolvedSizeSnapshotUsesNominalRatioForStaticAbstractModel(t *testing.T) {
	resolved := modelhub.ResolvedRequest{BaseResolution: "1k"}
	valid := domainimagetask.Task{
		AbstractModel: "plus", SizeMode: modelhub.SizeModeRatio, BaseResolution: "1k", AspectRatio: "1:1",
		RequestedSize: "1024x1024", ResolvedWidth: 1024, ResolvedHeight: 1024,
	}
	if err := validateResolvedSizeSnapshot(valid, resolved); err != nil {
		t.Fatalf("valid static ratio snapshot rejected: %v", err)
	}

	forged := valid
	forged.RequestedSize, forged.ResolvedWidth, forged.ResolvedHeight = "1008x1008", 1008, 1008
	err := validateResolvedSizeSnapshot(forged, resolved)
	var appErr *errs.Error
	if !errors.As(err, &appErr) || appErr.Code != modelhub.CodeInvalidSizeMode {
		t.Fatalf("forged static ratio snapshot error = %#v, want %s", err, modelhub.CodeInvalidSizeMode)
	}
}
