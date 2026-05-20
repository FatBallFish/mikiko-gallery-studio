package worker

import (
	"context"
	"time"

	domainimagetask "github.com/fatballfish/pic-gallery/internal/domain/imagetask"
	"github.com/fatballfish/pic-gallery/pkg/errs"
)

type Config struct {
	Owner              string
	LeaseTTL           time.Duration
	HeartbeatInterval  time.Duration
	PollInterval       time.Duration
	PreferredProviders []string
}

type Runner struct {
	tasks taskService
	cfg   Config
}

type executeOutcome struct {
	result domainimagetask.ExecuteResult
	err    error
}

type taskService interface {
	AcquireNextTask(ctx context.Context, owner string, leaseTTL time.Duration) (domainimagetask.Task, bool, error)
	HeartbeatTask(ctx context.Context, taskID, owner string, leaseTTL time.Duration) (domainimagetask.Task, error)
	ExecuteLeasedTask(ctx context.Context, task domainimagetask.Task, owner string, preferredProviders []string) (domainimagetask.ExecuteResult, error)
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
	return &Runner{tasks: tasks, cfg: cfg}
}

func (r *Runner) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		processed, err := r.ProcessOnce(ctx)
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

func (r *Runner) ProcessOnce(ctx context.Context) (bool, error) {
	task, ok, err := r.tasks.AcquireNextTask(ctx, r.cfg.Owner, r.cfg.LeaseTTL)
	if err != nil {
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
