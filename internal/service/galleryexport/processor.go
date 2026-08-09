package galleryexport

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/storage"
)

type CompleteJobRequest struct {
	JobID            string
	Owner            string
	AttemptCount     int
	StorageConfigID  string
	StorageDriver    string
	Bucket           string
	ObjectKey        string
	ArchiveSizeBytes int64
	ExpiresAt        time.Time
}

type ProcessorStore interface {
	Store
	AcquireNextJob(ctx context.Context, owner string, now time.Time, leaseTTL time.Duration) (Job, bool, error)
	CompleteJob(ctx context.Context, req CompleteJobRequest) (Job, error)
	FailJob(ctx context.Context, job Job, now time.Time, code, message string) error
	ExpireReady(ctx context.Context, now time.Time, limit int) (int, error)
}

type ProcessorOptions struct {
	Owner       string
	LeaseTTL    time.Duration
	ArchiveTTL  time.Duration
	ExpireBatch int
	Now         func() time.Time
	Service     Options
}

type Processor struct {
	store   ProcessorStore
	router  storage.Router
	service *Service
	opts    ProcessorOptions
}

func NewProcessor(store ProcessorStore, router storage.Router, opts ProcessorOptions) *Processor {
	if opts.LeaseTTL <= 0 {
		opts.LeaseTTL = 2 * time.Minute
	}
	if opts.ArchiveTTL <= 0 {
		opts.ArchiveTTL = 24 * time.Hour
	}
	if opts.ExpireBatch <= 0 {
		opts.ExpireBatch = 25
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Processor{store: store, router: router, service: NewService(store, router, opts.Service), opts: opts}
}

func (p *Processor) ProcessOnce(ctx context.Context) (bool, error) {
	now := p.opts.Now().UTC()
	expired, err := p.store.ExpireReady(ctx, now, p.opts.ExpireBatch)
	if err != nil {
		return false, fmt.Errorf("expire gallery exports: %w", err)
	}
	if expired > 0 {
		return true, nil
	}
	job, ok, err := p.store.AcquireNextJob(ctx, p.opts.Owner, now, p.opts.LeaseTTL)
	if err != nil || !ok {
		return false, err
	}
	assets, err := p.store.AuthorizeAssets(ctx, job.UserID, job.ProjectID, job.ImageIDs)
	if err != nil {
		return true, p.fail(ctx, job, now, "authorization_changed", "selected assets are no longer available")
	}
	archive, _, err := p.service.buildArchive(ctx, assets)
	if err != nil {
		return true, p.fail(ctx, job, now, "archive_build_failed", err.Error())
	}
	writer, err := p.router.DefaultWriter(ctx)
	if err != nil {
		return true, p.fail(ctx, job, now, "storage_unavailable", "archive storage is unavailable")
	}
	objectKey := fmt.Sprintf("gallery-exports/%d/%s.zip", job.UserID, job.ID)
	if err := writer.Backend.Put(ctx, objectKey, "application/zip", archive); err != nil {
		return true, p.fail(ctx, job, now, "archive_store_failed", "archive could not be stored")
	}
	_, err = p.store.CompleteJob(ctx, CompleteJobRequest{
		JobID: job.ID, Owner: p.opts.Owner, AttemptCount: job.AttemptCount,
		StorageConfigID: writer.ConfigID, StorageDriver: writer.Driver, Bucket: writer.Bucket,
		ObjectKey: objectKey, ArchiveSizeBytes: int64(len(archive)), ExpiresAt: now.Add(p.opts.ArchiveTTL),
	})
	if err != nil {
		return true, fmt.Errorf("complete gallery export: %w", err)
	}
	return true, nil
}

func (p *Processor) fail(ctx context.Context, job Job, now time.Time, code, message string) error {
	if err := p.store.FailJob(ctx, job, now, strings.TrimSpace(code), strings.TrimSpace(message)); err != nil {
		return fmt.Errorf("record gallery export failure: %w", err)
	}
	return nil
}
