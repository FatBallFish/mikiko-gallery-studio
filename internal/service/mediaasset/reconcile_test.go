package mediaasset

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

type uploadExpiryStoreProbe struct {
	session    UploadSession
	claimed    bool
	completed  []uuid.UUID
	claimNow   time.Time
	claimLease time.Duration
}

type mediaReconcileStoreProbe struct {
	processed bool
	err       error
	calls     int
}

func (store *mediaReconcileStoreProbe) ReconcileMediaOnce(context.Context) (bool, error) {
	store.calls++
	return store.processed, store.err
}

func (store *uploadExpiryStoreProbe) ClaimExpiredUpload(_ context.Context, now time.Time, lease time.Duration) (UploadSession, bool, error) {
	store.claimNow, store.claimLease = now, lease
	return store.session, store.claimed, nil
}

func (store *uploadExpiryStoreProbe) CompleteExpiredUpload(_ context.Context, id uuid.UUID) (bool, error) {
	store.completed = append(store.completed, id)
	return true, nil
}

type multipartAbortProbe struct {
	err     error
	aborted []storage.MultipartUpload
}

func (*multipartAbortProbe) Driver() string                                    { return "local" }
func (*multipartAbortProbe) Put(context.Context, string, string, []byte) error { return nil }
func (*multipartAbortProbe) Get(context.Context, string) ([]byte, error)       { return nil, nil }
func (*multipartAbortProbe) Delete(context.Context, string) error              { return nil }
func (*multipartAbortProbe) CreateMultipart(context.Context, storage.MultipartCreateRequest) (storage.MultipartUpload, error) {
	return storage.MultipartUpload{}, nil
}
func (*multipartAbortProbe) SignMultipartPart(context.Context, storage.MultipartUpload, int, string, time.Duration) (storage.MultipartPartTarget, error) {
	return storage.MultipartPartTarget{}, nil
}
func (*multipartAbortProbe) PutMultipartPart(context.Context, storage.MultipartUpload, int, io.Reader, int64, string) (storage.CompletedPart, error) {
	return storage.CompletedPart{}, nil
}
func (*multipartAbortProbe) MultipartStatus(context.Context, storage.MultipartUpload) (storage.MultipartStatus, error) {
	return storage.MultipartStatus{}, nil
}
func (*multipartAbortProbe) CompleteMultipart(context.Context, storage.MultipartUpload, []storage.CompletedPart) (storage.CompletedMultipartObject, error) {
	return storage.CompletedMultipartObject{}, nil
}
func (backend *multipartAbortProbe) AbortMultipart(_ context.Context, upload storage.MultipartUpload) error {
	backend.aborted = append(backend.aborted, upload)
	return backend.err
}

func TestUploadExpiryProcessorAbortsBeforeReleasingReservation(t *testing.T) {
	now := time.Date(2026, 8, 12, 21, 0, 0, 0, time.UTC)
	session := UploadSession{ID: uuid.New(), BackendUploadID: "upload-1", ObjectKey: "media/original/1/video.mp4", StorageDriver: "local"}
	store := &uploadExpiryStoreProbe{session: session, claimed: true}
	backend := &multipartAbortProbe{}
	processor := NewUploadExpiryProcessor(store, storage.NewStaticRouter(backend), UploadExpiryProcessorOptions{LeaseTTL: time.Minute, Now: func() time.Time { return now }})

	processed, err := processor.ProcessOnce(t.Context())
	if err != nil || !processed || len(backend.aborted) != 1 || len(store.completed) != 1 || store.completed[0] != session.ID {
		t.Fatalf("ProcessOnce() = %v, %v; aborted=%v completed=%v", processed, err, backend.aborted, store.completed)
	}
	if !store.claimNow.Equal(now) || store.claimLease != time.Minute {
		t.Fatalf("claim arguments = %s, %s", store.claimNow, store.claimLease)
	}
}

func TestUploadExpiryProcessorRetainsLeaseOnAbortFailureAndAcceptsMissingUpload(t *testing.T) {
	session := UploadSession{ID: uuid.New(), BackendUploadID: "upload-2", ObjectKey: "media/original/1/image.png", StorageDriver: "local"}
	store := &uploadExpiryStoreProbe{session: session, claimed: true}
	backend := &multipartAbortProbe{err: errors.New("storage unavailable")}
	processor := NewUploadExpiryProcessor(store, storage.NewStaticRouter(backend), UploadExpiryProcessorOptions{})

	processed, err := processor.ProcessOnce(t.Context())
	if !processed || err == nil || len(store.completed) != 0 {
		t.Fatalf("failed abort = %v, %v; completed=%v", processed, err, store.completed)
	}
	backend.err = storage.ErrMultipartNotFound
	processed, err = processor.ProcessOnce(t.Context())
	if err != nil || !processed || len(store.completed) != 1 {
		t.Fatalf("missing abort = %v, %v; completed=%v", processed, err, store.completed)
	}
}

func TestUploadExpiryProcessorIsIdleWithoutClaim(t *testing.T) {
	store := &uploadExpiryStoreProbe{}
	processor := NewUploadExpiryProcessor(store, storage.NewStaticRouter(&multipartAbortProbe{}), UploadExpiryProcessorOptions{})
	processed, err := processor.ProcessOnce(t.Context())
	if err != nil || processed {
		t.Fatalf("ProcessOnce() = %v, %v", processed, err)
	}
}

func TestMediaReconcileProcessorDelegatesOneDatabaseRepair(t *testing.T) {
	store := &mediaReconcileStoreProbe{processed: true}
	processor := NewMediaReconcileProcessor(store)
	processed, err := processor.ProcessOnce(t.Context())
	if err != nil || !processed || store.calls != 1 {
		t.Fatalf("ProcessOnce() = %v, %v; calls=%d", processed, err, store.calls)
	}
}
