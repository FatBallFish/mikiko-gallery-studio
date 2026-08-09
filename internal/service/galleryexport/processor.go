package galleryexport

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/internal/storage"
	"github.com/google/uuid"
)

const DefaultAsyncTimeout = 10 * time.Minute

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
	CompletedAt      time.Time
}

type ProcessorStore interface {
	Store
	AcquireNextJob(ctx context.Context, owner string, now time.Time, leaseTTL time.Duration) (Job, bool, error)
	RenewJobLease(ctx context.Context, jobID, owner string, attempt int, now time.Time, leaseTTL time.Duration) (bool, error)
	CompleteJob(ctx context.Context, req CompleteJobRequest) (Job, error)
	FailJob(ctx context.Context, job Job, now time.Time, code, message string) error
	ExpireReady(ctx context.Context, now time.Time, limit int) (int, error)
}

type ProcessorOptions struct {
	Owner        string
	LeaseTTL     time.Duration
	ArchiveTTL   time.Duration
	AsyncTimeout time.Duration
	ExpireBatch  int
	Now          func() time.Time
	Service      Options
	AttemptToken func() string
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
	if opts.AsyncTimeout <= 0 {
		opts.AsyncTimeout = DefaultAsyncTimeout
	}
	if opts.ExpireBatch <= 0 {
		opts.ExpireBatch = 25
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	if opts.AttemptToken == nil {
		opts.AttemptToken = uuid.NewString
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
	processCtx, cancelProcess := context.WithTimeout(ctx, p.opts.AsyncTimeout)
	if job.DeadlineAt != nil {
		workerDeadline := time.Now().UTC().Add(p.opts.AsyncTimeout)
		if job.DeadlineAt.Before(workerDeadline) {
			cancelProcess()
			processCtx, cancelProcess = context.WithDeadline(ctx, job.DeadlineAt.UTC())
		}
	}
	defer cancelProcess()
	heartbeatCtx, stopHeartbeat := context.WithCancel(ctx)
	heartbeatDone := make(chan leaseHeartbeatResult, 1)
	go p.renewLease(heartbeatCtx, cancelProcess, job, heartbeatDone)

	processErr := p.processJob(processCtx, job, now)
	stopHeartbeat()
	heartbeat := <-heartbeatDone
	if heartbeat.lost {
		return true, nil
	}
	if heartbeat.err != nil {
		return true, heartbeat.err
	}
	if processErr == nil {
		return true, nil
	}
	return true, p.fail(ctx, job, p.opts.Now().UTC(), processErr.code, processErr.message)
}

type jobProcessError struct {
	code    string
	message string
}

type leaseHeartbeatResult struct {
	lost bool
	err  error
}

func (p *Processor) processJob(ctx context.Context, job Job, now time.Time) *jobProcessError {
	assets, err := p.store.AuthorizeAssets(ctx, job.UserID, job.ProjectID, job.ImageIDs)
	if err != nil {
		return &jobProcessError{code: "authorization_changed", message: "selected assets are no longer available"}
	}
	archive, err := p.service.buildArchive(ctx, assets)
	if err != nil {
		return &jobProcessError{code: "archive_build_failed", message: err.Error()}
	}
	defer archive.Close()
	writer, err := p.router.DefaultWriter(ctx)
	if err != nil {
		return &jobProcessError{code: "storage_unavailable", message: "archive storage is unavailable"}
	}
	streaming, ok := writer.Backend.(storage.StreamingBackend)
	if !ok {
		return &jobProcessError{code: "streaming_storage_unsupported", message: "archive storage does not support streaming uploads"}
	}
	file, err := os.Open(archive.Path)
	if err != nil {
		return &jobProcessError{code: "archive_build_failed", message: "temporary archive could not be opened"}
	}
	defer file.Close()
	objectKey := p.attemptObjectKey(job)
	if err := streaming.PutReader(ctx, objectKey, "application/zip", file, archive.Size); err != nil {
		_ = writer.Backend.Delete(context.WithoutCancel(ctx), objectKey)
		return &jobProcessError{code: "archive_store_failed", message: "archive could not be stored"}
	}
	completedAt := p.opts.Now().UTC()
	_, err = p.store.CompleteJob(ctx, CompleteJobRequest{
		JobID: job.ID, Owner: p.opts.Owner, AttemptCount: job.AttemptCount,
		StorageConfigID: writer.ConfigID, StorageDriver: writer.Driver, Bucket: writer.Bucket,
		ObjectKey: objectKey, ArchiveSizeBytes: archive.Size, ExpiresAt: completedAt.Add(p.opts.ArchiveTTL), CompletedAt: completedAt,
	})
	if err != nil {
		_ = writer.Backend.Delete(context.WithoutCancel(ctx), objectKey)
		if errors.Is(err, repoerr.ErrConflict) {
			return nil
		}
		return &jobProcessError{code: "archive_completion_failed", message: "archive completion could not be recorded"}
	}
	return nil
}

func (p *Processor) attemptObjectKey(job Job) string {
	token := strings.TrimSpace(p.opts.AttemptToken())
	if _, err := uuid.Parse(token); err != nil {
		token = uuid.NewString()
	}
	return fmt.Sprintf("gallery-exports/%d/%s/attempt-%d-%s.zip", job.UserID, job.ID, job.AttemptCount, token)
}

func (p *Processor) renewLease(ctx context.Context, cancelProcess context.CancelFunc, job Job, done chan<- leaseHeartbeatResult) {
	interval := p.opts.LeaseTTL / 3
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			done <- leaseHeartbeatResult{}
			return
		case <-ticker.C:
			ok, err := p.store.RenewJobLease(ctx, job.ID, p.opts.Owner, job.AttemptCount, p.opts.Now().UTC(), p.opts.LeaseTTL)
			if err != nil {
				cancelProcess()
				done <- leaseHeartbeatResult{err: fmt.Errorf("renew gallery export lease: %w", err)}
				return
			}
			if !ok {
				cancelProcess()
				done <- leaseHeartbeatResult{lost: true}
				return
			}
		}
	}
}

func (p *Processor) fail(ctx context.Context, job Job, now time.Time, code, message string) error {
	if err := p.store.FailJob(ctx, job, now, strings.TrimSpace(code), strings.TrimSpace(message)); err != nil {
		if errors.Is(err, repoerr.ErrConflict) {
			return nil
		}
		return fmt.Errorf("record gallery export failure: %w", err)
	}
	return nil
}
