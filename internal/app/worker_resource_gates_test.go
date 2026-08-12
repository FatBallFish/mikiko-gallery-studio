package app

import (
	"context"
	"testing"
)

type temporaryDiskObserverSpy struct {
	usedPercent int
	freeBytes   int64
}

func (spy *temporaryDiskObserverSpy) SetTemporaryDisk(usedPercent int, freeBytes int64) {
	spy.usedPercent = usedPercent
	spy.freeBytes = freeBytes
}

func TestDiskClaimGatePublishesUsageBeforeApplyingWatermark(t *testing.T) {
	observer := &temporaryDiskObserverSpy{}
	gate := &diskClaimGate{
		path:         "/media-temp",
		pausePercent: 75,
		usage: func(path string) (int, int64, error) {
			if path != "/media-temp" {
				t.Fatalf("usage path = %q", path)
			}
			return 76, 10 << 30, nil
		},
		observer: observer,
	}
	allowed, err := gate.Allowed(context.Background())
	if err != nil || allowed {
		t.Fatalf("Allowed = %v, err %v", allowed, err)
	}
	if observer.usedPercent != 76 || observer.freeBytes != 10<<30 {
		t.Fatalf("disk observation = %d%%, %d bytes", observer.usedPercent, observer.freeBytes)
	}
}
