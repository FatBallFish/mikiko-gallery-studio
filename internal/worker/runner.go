package worker

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/internal/repository/repoerr"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Config struct {
	Owner                       string
	LeaseTTL                    time.Duration
	HeartbeatInterval           time.Duration
	PollInterval                time.Duration
	PreferredProviders          []string
	RefundCompensationBatchSize int
	MaxConcurrentTasks          int
	MaxConcurrentTasksResolver  func(context.Context) (int, error)
	ConfigRefreshInterval       time.Duration
	CleanupReconcileInterval    time.Duration
	CleanupReconcileBatchSize   int
}

type Runner struct {
	tasks                taskService
	compensation         compensationService
	cleanup              cleanupService
	galleryExport        galleryExportService
	cfg                  Config
	cleanupMu            sync.Mutex
	backgroundStreak     int
	nextBackgroundExport bool
	lastCleanupReconcile time.Time
}

const maxBackgroundStreak = 1

type executeOutcome struct {
	result domainimagetask.ExecuteResult
	err    error
}

type taskService interface {
	AcquireNextTask(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error)
	HeartbeatTask(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error)
	ExecuteLeasedTask(ctx context.Context, task domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error)
}

type compensationService interface {
	ProcessRefundFinalizeFailures(ctx context.Context, limit int) (int, error)
}

type cleanupService interface {
	ProcessOnce(context.Context) (bool, error)
	Reconcile(context.Context, int) (int, error)
}

type galleryExportService interface {
	ProcessOnce(context.Context) (bool, error)
}

func NewRunner(tasks taskService, cfg Config) *Runner {
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 30 * time.Second
	}
	if cfg.HeartbeatInterval <= 0 {
		cfg.HeartbeatInterval = cfg.LeaseTTL / 3
		if cfg.HeartbeatInterval <= 0 {
			cfg.HeartbeatInterval = time.Second
		}
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 500 * time.Millisecond
	}
	if cfg.RefundCompensationBatchSize <= 0 {
		cfg.RefundCompensationBatchSize = 5
	}
	if cfg.MaxConcurrentTasks <= 0 {
		cfg.MaxConcurrentTasks = 1
	}
	if cfg.MaxConcurrentTasks > 64 {
		cfg.MaxConcurrentTasks = 64
	}
	if cfg.ConfigRefreshInterval <= 0 {
		cfg.ConfigRefreshInterval = 5 * time.Second
	}
	if cfg.CleanupReconcileInterval <= 0 {
		cfg.CleanupReconcileInterval = 6 * time.Hour
	}
	if cfg.CleanupReconcileBatchSize <= 0 {
		cfg.CleanupReconcileBatchSize = 100
	}
	return &Runner{tasks: tasks, cfg: cfg}
}

func (r *Runner) SetCleanupService(service cleanupService) { r.cleanup = service }

func (r *Runner) SetGalleryExportService(service galleryExportService) { r.galleryExport = service }

func (r *Runner) SetCompensationService(service compensationService) {
	r.compensation = service
}

func (r *Runner) Run(ctx context.Context) error {
	if r.cfg.MaxConcurrentTasksResolver == nil && r.cfg.MaxConcurrentTasks <= 1 {
		return r.runLoop(ctx, true)
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	slotCount := r.cfg.MaxConcurrentTasks
	if r.cfg.MaxConcurrentTasksResolver != nil {
		slotCount = 64
	}
	currentMax := &atomic.Int64{}
	currentMax.Store(int64(r.cfg.MaxConcurrentTasks))

	errCh := make(chan error, slotCount+1)
	done := make(chan struct{})
	var wg sync.WaitGroup
	if r.cfg.MaxConcurrentTasksResolver != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.refreshMaxConcurrentTasks(ctx, currentMax, errCh, cancel)
		}()
	}
	for slot := 0; slot < slotCount; slot++ {
		slotIndex := slot
		processCompensation := slot == 0
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.runSlotLoop(ctx, slotIndex, currentMax, processCompensation); err != nil && !errors.Is(err, context.Canceled) {
				select {
				case errCh <- err:
					cancel()
				default:
				}
			}
		}()
	}
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case err := <-errCh:
		cancel()
		<-done
		return err
	case <-ctx.Done():
		cancel()
		<-done
		return ctx.Err()
	case <-done:
		return nil
	}
}

func (r *Runner) refreshMaxConcurrentTasks(ctx context.Context, currentMax *atomic.Int64, errCh chan<- error, cancel context.CancelFunc) {
	ticker := time.NewTicker(r.cfg.ConfigRefreshInterval)
	defer ticker.Stop()
	for {
		if err := r.refreshMaxConcurrentTasksOnce(ctx, currentMax); err != nil {
			select {
			case errCh <- err:
				cancel()
			default:
			}
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *Runner) refreshMaxConcurrentTasksOnce(ctx context.Context, currentMax *atomic.Int64) error {
	value, err := r.cfg.MaxConcurrentTasksResolver(ctx)
	if err != nil {
		return err
	}
	currentMax.Store(int64(normalizeMaxConcurrentTasks(value)))
	return nil
}

func (r *Runner) runSlotLoop(ctx context.Context, slotIndex int, currentMax *atomic.Int64, processCompensation bool) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		if slotIndex >= int(currentMax.Load()) {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-ticker.C:
			}
			continue
		}

		var (
			processed bool
			err       error
		)
		if processCompensation {
			processed, err = r.ProcessOnce(ctx)
		} else {
			processed, err = r.processTaskOnce(ctx)
		}
		if err != nil {
			return err
		}
		if processed {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (r *Runner) runLoop(ctx context.Context, processCompensation bool) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var (
			processed bool
			err       error
		)
		if processCompensation {
			processed, err = r.ProcessOnce(ctx)
		} else {
			processed, err = r.processTaskOnce(ctx)
		}
		if err != nil {
			return err
		}
		if processed {
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func normalizeMaxConcurrentTasks(value int) int {
	switch {
	case value <= 0:
		return 1
	case value > 64:
		return 64
	default:
		return value
	}
}

func (r *Runner) ProcessOnce(ctx context.Context) (bool, error) {
	if r.takeBackgroundFairnessTurn() {
		processed, err := r.processTaskOnce(ctx)
		if err != nil || processed {
			return processed, err
		}
	}
	processed, err := r.processBackgroundOnce(ctx)
	if err != nil || processed {
		return processed, err
	}
	if r.compensation != nil {
		processed, err := r.compensation.ProcessRefundFinalizeFailures(ctx, r.cfg.RefundCompensationBatchSize)
		if err != nil {
			if repoerr.IsTransientContention(err) {
				return false, nil
			}
			return false, err
		}
		if processed > 0 {
			return true, nil
		}
	}
	processed, err = r.processTaskOnce(ctx)
	if processed {
		r.resetBackgroundStreak()
	}
	return processed, err
}

func (r *Runner) processBackgroundOnce(ctx context.Context) (bool, error) {
	r.cleanupMu.Lock()
	exportFirst := r.nextBackgroundExport
	r.cleanupMu.Unlock()
	if exportFirst {
		if processed, err := r.processGalleryExportOnce(ctx); err != nil || processed {
			return processed, err
		}
		return r.processCleanupOnce(ctx)
	}
	if processed, err := r.processCleanupOnce(ctx); err != nil || processed {
		return processed, err
	}
	return r.processGalleryExportOnce(ctx)
}

func (r *Runner) processCleanupOnce(ctx context.Context) (bool, error) {
	if r.cleanup == nil {
		return false, nil
	}
	r.cleanupMu.Lock()
	due := r.lastCleanupReconcile.IsZero() || time.Since(r.lastCleanupReconcile) >= r.cfg.CleanupReconcileInterval
	if due {
		r.lastCleanupReconcile = time.Now()
	}
	r.cleanupMu.Unlock()
	if due {
		if _, err := r.cleanup.Reconcile(ctx, r.cfg.CleanupReconcileBatchSize); err != nil {
			return false, err
		}
	}
	processed, err := r.cleanup.ProcessOnce(ctx)
	if processed {
		r.noteBackgroundProcessed(true)
	}
	return processed, err
}

func (r *Runner) processGalleryExportOnce(ctx context.Context) (bool, error) {
	if r.galleryExport == nil {
		return false, nil
	}
	processed, err := r.galleryExport.ProcessOnce(ctx)
	if processed {
		r.noteBackgroundProcessed(false)
	}
	return processed, err
}

func (r *Runner) takeBackgroundFairnessTurn() bool {
	if r.cleanup == nil && r.galleryExport == nil {
		return false
	}
	r.cleanupMu.Lock()
	defer r.cleanupMu.Unlock()
	if r.backgroundStreak < maxBackgroundStreak {
		return false
	}
	r.backgroundStreak = 0
	return true
}

func (r *Runner) noteBackgroundProcessed(cleanup bool) {
	r.cleanupMu.Lock()
	r.backgroundStreak++
	r.nextBackgroundExport = cleanup
	r.cleanupMu.Unlock()
}

func (r *Runner) resetBackgroundStreak() {
	r.cleanupMu.Lock()
	r.backgroundStreak = 0
	r.cleanupMu.Unlock()
}

func (r *Runner) processTaskOnce(ctx context.Context) (bool, error) {
	task, ok, err := r.tasks.AcquireNextTask(ctx, r.cfg.Owner, r.cfg.LeaseTTL)
	if err != nil {
		if repoerr.IsTransientContention(err) {
			return false, nil
		}
		return false, err
	}
	if !ok {
		return false, nil
	}

	heartbeatTicker := time.NewTicker(r.cfg.HeartbeatInterval)
	defer heartbeatTicker.Stop()
	if err := r.processClaimedTask(ctx, task, heartbeatTicker.C); err != nil {
		return true, err
	}
	return true, nil
}

func (r *Runner) processClaimedTask(ctx context.Context, task domainimagetask.Task, heartbeatC <-chan time.Time) error {
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	done := make(chan executeOutcome, 1)
	go func() {
		result, execErr := r.tasks.ExecuteLeasedTask(execCtx, task, r.cfg.Owner, r.cfg.PreferredProviders)
		done <- executeOutcome{result: result, err: execErr}
	}()

	for {
		select {
		case <-ctx.Done():
			cancel()
			return ctx.Err()
		case outcome := <-done:
			return classifyExecutionOutcome(outcome.result, outcome.err)
		case <-heartbeatC:
			select {
			case outcome := <-done:
				return classifyExecutionOutcome(outcome.result, outcome.err)
			default:
			}
			if _, hbErr := r.tasks.HeartbeatTask(ctx, task.ID, r.cfg.Owner, r.cfg.LeaseTTL); hbErr != nil {
				cancel()
				if outcome, ok := waitForOutcome(done, r.heartbeatJoinTimeout()); ok && benignHeartbeatRace(hbErr, outcome) {
					return classifyExecutionOutcome(outcome.result, outcome.err)
				}
				return hbErr
			}
		}
	}
}

func (r *Runner) heartbeatJoinTimeout() time.Duration {
	switch {
	case r.cfg.HeartbeatInterval <= 0:
		return 100 * time.Millisecond
	case r.cfg.HeartbeatInterval < 50*time.Millisecond:
		return 50 * time.Millisecond
	case r.cfg.HeartbeatInterval > 250*time.Millisecond:
		return 250 * time.Millisecond
	default:
		return r.cfg.HeartbeatInterval
	}
}

func waitForOutcome(done <-chan executeOutcome, timeout time.Duration) (executeOutcome, bool) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case outcome := <-done:
		return outcome, true
	case <-timer.C:
		return executeOutcome{}, false
	}
}

func classifyExecutionOutcome(result domainimagetask.ExecuteResult, err error) error {
	if err == nil {
		return nil
	}
	if result.Task.Status == domainimagetask.StatusFailed {
		return nil
	}
	return err
}

func benignHeartbeatRace(hbErr error, outcome executeOutcome) bool {
	appErr, ok := hbErr.(*errs.Error)
	if !ok || appErr.Code != errs.CodeConflict {
		return false
	}
	if outcome.err == nil {
		return true
	}
	if outcome.result.Task.Status != domainimagetask.StatusFailed {
		return false
	}
	return outcome.err != context.Canceled
}
