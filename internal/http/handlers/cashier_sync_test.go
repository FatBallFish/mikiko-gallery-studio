package handlers

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/fatballfish/pic-gallery/pkg/errs"
)

func TestCashierOrderSyncCoordinatorThrottlesQueryFailures(t *testing.T) {
	var coordinator cashierOrderSyncCoordinator
	var calls atomic.Int32
	wantErr := errs.New(http.StatusBadGateway, errs.CodePaymentProviderUnavailable, "provider unavailable")
	query := func(context.Context) (adminCashierOrderSyncResult, *errs.Error) {
		calls.Add(1)
		return adminCashierOrderSyncResult{}, wantErr
	}

	for range 2 {
		_, gotErr := coordinator.Do(t.Context(), 1, query)
		if gotErr != wantErr {
			t.Fatalf("Do() error = %#v, want %#v", gotErr, wantErr)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("failed provider query calls = %d, want 1 within throttle window", got)
	}
}

func TestCashierOrderSyncCoordinatorBoundsSharedQueryContext(t *testing.T) {
	var coordinator cashierOrderSyncCoordinator
	_, gotErr := coordinator.Do(t.Context(), 1, func(ctx context.Context) (adminCashierOrderSyncResult, *errs.Error) {
		if _, ok := ctx.Deadline(); !ok {
			return adminCashierOrderSyncResult{}, errs.New(http.StatusInternalServerError, "MISSING_DEADLINE", "shared query context has no deadline")
		}
		return adminCashierOrderSyncResult{QueryStatus: "pending"}, nil
	})
	if gotErr != nil {
		t.Fatalf("Do() error = %v, want bounded shared query context", gotErr)
	}
}

func TestCashierOrderSyncCoordinatorBoundsCacheSize(t *testing.T) {
	var coordinator cashierOrderSyncCoordinator
	query := func(context.Context) (adminCashierOrderSyncResult, *errs.Error) {
		return adminCashierOrderSyncResult{QueryStatus: "pending"}, nil
	}

	for orderID := int64(1); orderID <= 2048; orderID++ {
		if _, err := coordinator.Do(t.Context(), orderID, query); err != nil {
			t.Fatalf("Do(%d) error = %v", orderID, err)
		}
	}
	if got := len(coordinator.cache); got > 1024 {
		t.Fatalf("cache size = %d, want at most 1024 entries", got)
	}
}
