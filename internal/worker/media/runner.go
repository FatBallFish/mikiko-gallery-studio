package media

import (
	"context"
	"errors"
	"fmt"
	"time"
)

type ClaimRequest struct {
	Owner    string
	Now      time.Time
	LeaseTTL time.Duration
}

type LeaseRef struct {
	JobID string
	Owner string
}

type WorkItem struct {
	JobID           string
	AssetID         string
	UserID          int64
	ProjectID       string
	MediaType       string
	MIMEType        string
	SizeBytes       int64
	StorageConfigID string
	StorageDriver   string
	Bucket          string
	ObjectKey       string
	AttemptCount    int
	MaxAttempts     int
}

type ProbeMetadata struct {
	Format         string
	Container      string
	VideoCodec     string
	AudioCodec     string
	Width          int
	Height         int
	DurationMS     int64
	FrameRateMilli int
	Channels       int
	SampleRate     int
}

type Derivative struct {
	Kind             string
	TransformVersion int
	StorageConfigID  string
	StorageDriver    string
	Bucket           string
	ObjectKey        string
	MIMEType         string
	SizeBytes        int64
	SHA256           string
}

type ProcessResult struct {
	Probe       ProbeMetadata
	Derivatives []Derivative
}

type CompleteRequest struct {
	JobID  string
	Owner  string
	Now    time.Time
	Result ProcessResult
}

type FailRequest struct {
	JobID        string
	Owner        string
	Now          time.Time
	RetryAt      time.Time
	Terminal     bool
	ErrorCode    string
	ErrorMessage string
}

type Store interface {
	ClaimDue(context.Context, ClaimRequest) (WorkItem, bool, error)
	Complete(context.Context, CompleteRequest) (bool, error)
	Fail(context.Context, FailRequest) error
	ReleaseLease(context.Context, LeaseRef) error
}

type Processor interface {
	Process(context.Context, WorkItem) (ProcessResult, error)
}

type Observer interface {
	RecordDerivative(kind, result string, bytes int64)
}

type Options struct {
	Owner        string
	LeaseTTL     time.Duration
	Now          func() time.Time
	ClaimAllowed func(context.Context) (bool, error)
	Observer     Observer
}

type Runner struct {
	store     Store
	processor Processor
	options   Options
}

func NewRunner(store Store, processor Processor, options Options) *Runner {
	if options.Owner == "" {
		options.Owner = "media-worker"
	}
	if options.LeaseTTL <= 0 {
		options.LeaseTTL = 2 * time.Minute
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	return &Runner{store: store, processor: processor, options: options}
}

func (runner *Runner) RunOnce(ctx context.Context) (bool, error) {
	if runner == nil || runner.store == nil || runner.processor == nil {
		return false, errors.New("media worker dependencies are unavailable")
	}
	now := runner.options.Now().UTC()
	if runner.options.ClaimAllowed != nil {
		allowed, err := runner.options.ClaimAllowed(ctx)
		if err != nil || !allowed {
			return false, err
		}
	}
	item, claimed, err := runner.store.ClaimDue(ctx, ClaimRequest{Owner: runner.options.Owner, Now: now, LeaseTTL: runner.options.LeaseTTL})
	if err != nil || !claimed {
		return claimed, err
	}
	lease := LeaseRef{JobID: item.JobID, Owner: runner.options.Owner}
	defer func() { _ = runner.store.ReleaseLease(context.WithoutCancel(ctx), lease) }()

	result, processErr := runner.processor.Process(ctx, item)
	if processErr == nil {
		completed, completeErr := runner.store.Complete(ctx, CompleteRequest{JobID: item.JobID, Owner: runner.options.Owner, Now: runner.options.Now().UTC(), Result: result})
		err = completeErr
		if err != nil {
			return true, fmt.Errorf("complete media processing job: %w", err)
		}
		if completed && runner.options.Observer != nil {
			for _, derivative := range result.Derivatives {
				runner.options.Observer.RecordDerivative(derivative.Kind, "success", derivative.SizeBytes)
			}
		}
		return true, nil
	}
	if errors.Is(processErr, context.Canceled) && ctx.Err() != nil {
		return true, ctx.Err()
	}
	maxAttempts := item.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	terminal := item.AttemptCount >= maxAttempts
	retryAt := now
	if !terminal {
		shift := item.AttemptCount - 1
		if shift < 0 {
			shift = 0
		}
		if shift > 8 {
			shift = 8
		}
		retryAt = now.Add(time.Second * time.Duration(1<<shift))
	}
	if err := runner.store.Fail(ctx, FailRequest{
		JobID: item.JobID, Owner: runner.options.Owner, Now: now, RetryAt: retryAt, Terminal: terminal,
		ErrorCode: "media_processing_failed", ErrorMessage: "media processing failed",
	}); err != nil {
		return true, fmt.Errorf("record media processing failure: %w", err)
	}
	return true, nil
}
