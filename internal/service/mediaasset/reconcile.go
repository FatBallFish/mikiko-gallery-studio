package mediaasset

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

type UploadExpiryStore interface {
	ClaimExpiredUpload(context.Context, time.Time, time.Duration) (UploadSession, bool, error)
	CompleteExpiredUpload(context.Context, uuid.UUID) (bool, error)
}

type MediaReconcileStore interface {
	ReconcileMediaOnce(context.Context) (bool, error)
}

type MediaReconcileProcessor struct{ store MediaReconcileStore }

func NewMediaReconcileProcessor(store MediaReconcileStore) *MediaReconcileProcessor {
	return &MediaReconcileProcessor{store: store}
}

func (processor *MediaReconcileProcessor) ProcessOnce(ctx context.Context) (bool, error) {
	if processor == nil || processor.store == nil {
		return false, errors.New("media reconcile processor is unavailable")
	}
	return processor.store.ReconcileMediaOnce(ctx)
}

type UploadExpiryProcessorOptions struct {
	LeaseTTL time.Duration
	Now      func() time.Time
}

type UploadExpiryProcessor struct {
	store  UploadExpiryStore
	router storage.Router
	opts   UploadExpiryProcessorOptions
}

func NewUploadExpiryProcessor(store UploadExpiryStore, router storage.Router, opts UploadExpiryProcessorOptions) *UploadExpiryProcessor {
	if opts.LeaseTTL <= 0 {
		opts.LeaseTTL = 5 * time.Minute
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &UploadExpiryProcessor{store: store, router: router, opts: opts}
}

func (processor *UploadExpiryProcessor) ProcessOnce(ctx context.Context) (bool, error) {
	if processor == nil || processor.store == nil || processor.router == nil {
		return false, errors.New("upload expiry processor dependencies are unavailable")
	}
	session, claimed, err := processor.store.ClaimExpiredUpload(ctx, processor.opts.Now().UTC(), processor.opts.LeaseTTL)
	if err != nil || !claimed {
		return false, err
	}
	ref, err := processor.router.BackendFor(ctx, session.StorageConfigID, session.StorageDriver)
	if err != nil {
		return true, fmt.Errorf("resolve expired upload storage: %w", err)
	}
	multipart, ok := ref.Backend.(storage.MultipartBackend)
	if !ok {
		return true, errors.New("expired upload storage does not support multipart cleanup")
	}
	if err := multipart.AbortMultipart(ctx, session.MultipartUpload()); err != nil && !errors.Is(err, storage.ErrMultipartNotFound) {
		return true, fmt.Errorf("abort expired multipart upload: %w", err)
	}
	if _, err := processor.store.CompleteExpiredUpload(ctx, session.ID); err != nil {
		return true, err
	}
	return true, nil
}
