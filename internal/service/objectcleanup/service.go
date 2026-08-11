package objectcleanup

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	domaincleanup "github.com/fatballfish/pic-gallery/internal/domain/objectcleanup"
	"github.com/fatballfish/pic-gallery/internal/storage"
)

const (
	StatePending = domaincleanup.StatePending
	StateRunning = domaincleanup.StateRunning
	StateRetry   = domaincleanup.StateRetry
	StateDone    = domaincleanup.StateDone
	StateBlocked = domaincleanup.StateBlocked
)

type Identity = domaincleanup.Identity
type Job = domaincleanup.Job

type Store interface {
	Enqueue(context.Context, Identity) (Job, error)
	Claim(context.Context, time.Time) (Job, bool, error)
	DeleteIfUnreferenced(context.Context, Job, func(Identity) error) (bool, error)
	MarkDone(context.Context, Job) error
	MarkBlocked(context.Context, Job, string) error
	MarkRetry(context.Context, Job, time.Time, string, string) error
	HasLiveReferences(context.Context, Identity) (bool, error)
	Reconcile(context.Context, int) (int, error)
	GetReconcileCheckpoint(context.Context, string, string) (domaincleanup.ReconcileCheckpoint, bool, error)
	SaveReconcileCheckpoint(context.Context, domaincleanup.ReconcileCheckpoint, time.Time) (bool, error)
}

type ProcessorOptions struct {
	Now                func() time.Time
	Jitter             func(time.Duration) time.Duration
	OrphanGracePeriod  time.Duration
	ObjectListPageSize int
}

type Processor struct {
	store              Store
	router             storage.Router
	now                func() time.Time
	jitter             func(time.Duration) time.Duration
	orphanGracePeriod  time.Duration
	objectListPageSize int
	reconcileMu        sync.Mutex
}

var ownedObjectPrefixes = []string{
	"generated-images/",
	"reference-assets/",
	"gallery-exports/",
	"media/original/",
	"media/derivatives/",
	"media/uploads/",
	"canvas/previews/",
}

func NewProcessor(store Store, router storage.Router, options ProcessorOptions) *Processor {
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.Jitter == nil {
		options.Jitter = func(max time.Duration) time.Duration {
			if max <= 0 {
				return 0
			}
			return time.Duration(rand.Int63n(int64(max)))
		}
	}
	if options.OrphanGracePeriod <= 0 {
		options.OrphanGracePeriod = 24 * time.Hour
	}
	if options.ObjectListPageSize <= 0 {
		options.ObjectListPageSize = 100
	}
	return &Processor{
		store: store, router: router, now: options.Now, jitter: options.Jitter,
		orphanGracePeriod:  options.OrphanGracePeriod,
		objectListPageSize: options.ObjectListPageSize,
	}
}

func (p *Processor) ProcessOnce(ctx context.Context) (bool, error) {
	job, ok, err := p.store.Claim(ctx, p.now())
	if err != nil || !ok {
		return false, err
	}
	blocked, deleteErr := p.store.DeleteIfUnreferenced(ctx, job, func(identity Identity) error {
		ref, err := p.router.BackendFor(ctx, identity.StorageConfigID, identity.StorageDriver)
		if err != nil {
			return storageBackendUnavailableError{err: err}
		}
		return ref.Backend.Delete(ctx, identity.ObjectKey)
	})
	if errors.Is(deleteErr, domaincleanup.ErrStaleClaim) {
		return true, nil
	}
	if blocked {
		return true, p.store.MarkBlocked(ctx, job, "live_reference")
	}
	if deleteErr == nil || errors.Is(deleteErr, storage.ErrNotFound) {
		return true, p.store.MarkDone(ctx, job)
	}
	var unavailable storageBackendUnavailableError
	if errors.As(deleteErr, &unavailable) {
		return true, p.retry(ctx, job, "storage_config_unavailable", "storage backend unavailable")
	}
	return true, p.retry(ctx, job, "storage_delete_failed", "storage delete failed")
}

type storageBackendUnavailableError struct{ err error }

func (e storageBackendUnavailableError) Error() string { return "storage backend unavailable" }
func (e storageBackendUnavailableError) Unwrap() error { return e.err }

func (p *Processor) Reconcile(ctx context.Context, limit int) (int, error) {
	count, err := p.store.Reconcile(ctx, limit)
	if err != nil {
		return count, err
	}
	if limit <= 0 {
		limit = 100
	}
	refs, err := p.router.ReadableBackends(ctx)
	if err != nil {
		return count, fmt.Errorf("list readable storage backends: %w", err)
	}
	if len(refs) == 0 {
		slog.Debug("object cleanup storage reconciliation skipped", "error_code", "readable_storage_unavailable")
		return count, nil
	}
	p.reconcileMu.Lock()
	defer p.reconcileMu.Unlock()
	targets := make([]objectReconcileTarget, 0, len(refs)*len(ownedObjectPrefixes))
	for _, ref := range refs {
		lister, ok := ref.Backend.(storage.ObjectLister)
		if !ok {
			slog.Debug("object cleanup storage reconciliation skipped", "error_code", "listing_unsupported", "config_id", ref.ConfigID, "storage_driver", ref.Driver)
			continue
		}
		storageIdentity := reconcileStorageIdentity(ref)
		for _, prefix := range ownedObjectPrefixes {
			checkpoint, ok, err := p.store.GetReconcileCheckpoint(ctx, storageIdentity, prefix)
			if err != nil {
				return count, err
			}
			if !ok {
				checkpoint = domaincleanup.ReconcileCheckpoint{StorageIdentity: storageIdentity, Namespace: ref.Namespace, Prefix: prefix}
			} else if checkpoint.Namespace != ref.Namespace {
				checkpoint.Namespace, checkpoint.Cursor = ref.Namespace, ""
				checkpoint.Generation++
			}
			targets = append(targets, objectReconcileTarget{ref: ref, lister: lister, checkpoint: checkpoint})
		}
	}
	if len(targets) == 0 {
		return count, nil
	}
	sort.Slice(targets, func(i, j int) bool {
		left, right := targets[i].checkpoint, targets[j].checkpoint
		if left.Generation != right.Generation {
			return left.Generation < right.Generation
		}
		if !left.UpdatedAt.Equal(right.UpdatedAt) {
			return left.UpdatedAt.Before(right.UpdatedAt)
		}
		if left.StorageIdentity != right.StorageIdentity {
			return left.StorageIdentity < right.StorageIdentity
		}
		return left.Prefix < right.Prefix
	})
	base, extra := limit/len(targets), limit%len(targets)
	for index, target := range targets {
		quota := base
		if index < extra {
			quota++
		}
		if quota == 0 {
			continue
		}
		_, enqueued, err := p.reconcileOwnedPrefix(ctx, target, quota)
		count += enqueued
		if err != nil {
			return count, err
		}
	}
	return count, nil
}

type objectReconcileTarget struct {
	ref        storage.BackendRef
	lister     storage.ObjectLister
	checkpoint domaincleanup.ReconcileCheckpoint
}

func reconcileStorageIdentity(ref storage.BackendRef) string {
	if id := strings.TrimSpace(ref.ConfigID); id != "" {
		return id
	}
	return "legacy:" + strings.ToLower(strings.TrimSpace(ref.Driver)) + ":" + strings.TrimSpace(ref.Namespace)
}

func (p *Processor) reconcileOwnedPrefix(ctx context.Context, target objectReconcileTarget, limit int) (int, int, error) {
	scanned, enqueued := 0, 0
	cutoff := p.now().Add(-p.orphanGracePeriod)
	for scanned < limit {
		pageLimit := p.objectListPageSize
		if remaining := limit - scanned; pageLimit > remaining {
			pageLimit = remaining
		}
		cursor := target.checkpoint.Cursor
		page, err := target.lister.ListObjects(ctx, target.checkpoint.Prefix, cursor, pageLimit)
		if err != nil {
			return scanned, enqueued, fmt.Errorf("list cleanup objects: %w", err)
		}
		objects := page.Objects
		if remaining := limit - scanned; len(objects) > remaining {
			objects = objects[:remaining]
		}
		for _, object := range objects {
			scanned++
			if object.ModifiedAt.IsZero() || object.ModifiedAt.After(cutoff) {
				continue
			}
			identity := domaincleanup.CanonicalIdentity(Identity{
				StorageConfigID: target.ref.ConfigID,
				StorageDriver:   target.ref.Driver,
				Bucket:          target.ref.Bucket,
				ObjectKey:       strings.TrimSpace(object.ObjectKey),
			})
			if !strings.HasPrefix(identity.ObjectKey, target.checkpoint.Prefix) {
				continue
			}
			live, err := p.store.HasLiveReferences(ctx, identity)
			if err != nil {
				return scanned, enqueued, err
			}
			if live {
				continue
			}
			if _, err := p.store.Enqueue(ctx, identity); err != nil {
				return scanned, enqueued, err
			}
			enqueued++
		}
		nextCursor := strings.TrimSpace(page.NextCursor)
		if nextCursor != "" && nextCursor == cursor {
			return scanned, enqueued, errors.New("object listing cursor did not advance")
		}
		expectedUpdatedAt := target.checkpoint.UpdatedAt
		target.checkpoint.Cursor = nextCursor
		target.checkpoint.UpdatedAt = nextCheckpointUpdateTime(expectedUpdatedAt, p.now().UTC())
		if nextCursor == "" {
			target.checkpoint.Generation++
		}
		saved, err := p.store.SaveReconcileCheckpoint(ctx, target.checkpoint, expectedUpdatedAt)
		if err != nil {
			return scanned, enqueued, err
		}
		if !saved {
			return scanned, enqueued, nil
		}
		if nextCursor == "" {
			return scanned, enqueued, nil
		}
	}
	return scanned, enqueued, nil
}

func nextCheckpointUpdateTime(previous, current time.Time) time.Time {
	current = current.UTC()
	if !previous.IsZero() && !current.After(previous) {
		return previous.UTC().Add(time.Microsecond)
	}
	return current
}

func (p *Processor) retry(ctx context.Context, job Job, code, message string) error {
	shift := job.AttemptCount - 1
	if shift < 0 {
		shift = 0
	}
	if shift > 8 {
		shift = 8
	}
	delay := time.Second * time.Duration(1<<shift)
	delay += p.jitter(delay / 4)
	return p.store.MarkRetry(ctx, job, p.now().Add(delay), sanitizeCode(code), sanitizeMessage(message))
}

func sanitizeCode(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}

func sanitizeMessage(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 160 {
		value = value[:160]
	}
	return value
}

type MemoryStore struct {
	mu          sync.Mutex
	now         func() time.Time
	jobs        map[string]Job
	jobByKey    map[string]string
	liveRefs    map[string]map[string]struct{}
	candidates  map[string]Identity
	checkpoints map[string]domaincleanup.ReconcileCheckpoint
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		now:         time.Now,
		jobs:        map[string]Job{},
		jobByKey:    map[string]string{},
		liveRefs:    map[string]map[string]struct{}{},
		candidates:  map[string]Identity{},
		checkpoints: map[string]domaincleanup.ReconcileCheckpoint{},
	}
}

func checkpointKey(storageIdentity, prefix string) string {
	return strings.TrimSpace(storageIdentity) + "\x00" + strings.TrimSpace(prefix)
}

func (s *MemoryStore) GetReconcileCheckpoint(_ context.Context, storageIdentity, prefix string) (domaincleanup.ReconcileCheckpoint, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	checkpoint, ok := s.checkpoints[checkpointKey(storageIdentity, prefix)]
	return checkpoint, ok, nil
}

func (s *MemoryStore) SaveReconcileCheckpoint(_ context.Context, checkpoint domaincleanup.ReconcileCheckpoint, expectedUpdatedAt time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := checkpointKey(checkpoint.StorageIdentity, checkpoint.Prefix)
	current, ok := s.checkpoints[key]
	if expectedUpdatedAt.IsZero() {
		if ok {
			return false, nil
		}
	} else if !ok || !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return false, nil
	}
	s.checkpoints[key] = checkpoint
	return true, nil
}

func (s *MemoryStore) SetNow(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func identityKey(value Identity) string {
	return domaincleanup.CanonicalKey(value)
}

func (s *MemoryStore) Enqueue(_ context.Context, identity Identity) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity = domaincleanup.CanonicalIdentity(identity)
	key, now := identityKey(identity), s.now().UTC()
	if id := s.jobByKey[key]; id != "" {
		job := s.jobs[id]
		if job.State != StateDone {
			job.State, job.NextAttemptAt, job.CompletedAt = StatePending, nil, nil
			job.UpdatedAt = now
			s.jobs[id] = job
			return job, nil
		}
	}
	job := Job{ID: uuid.NewString(), Identity: identity, State: StatePending, CreatedAt: now, UpdatedAt: now}
	s.jobs[job.ID], s.jobByKey[key] = job, job.ID
	return job, nil
}

func (s *MemoryStore) Claim(_ context.Context, now time.Time) (Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, job := range s.jobs {
		if job.State != StatePending && job.State != StateRetry && !(job.State == StateRunning && job.UpdatedAt.Add(time.Minute).Before(now)) {
			continue
		}
		if job.NextAttemptAt != nil && job.NextAttemptAt.After(now) {
			continue
		}
		job.State, job.AttemptCount, job.UpdatedAt = StateRunning, job.AttemptCount+1, now.UTC()
		s.jobs[id] = job
		return job, true, nil
	}
	return Job{}, false, nil
}

func (s *MemoryStore) DeleteIfUnreferenced(_ context.Context, job Job, deleteFn func(Identity) error) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, ok := s.jobs[job.ID]
	if !ok || current.State != StateRunning || current.AttemptCount != job.AttemptCount || identityKey(current.Identity) != identityKey(job.Identity) {
		return false, domaincleanup.ErrStaleClaim
	}
	if len(s.liveRefs[identityKey(current.Identity)]) > 0 {
		return true, nil
	}
	return false, deleteFn(current.Identity)
}

func (s *MemoryStore) HasLiveReferences(_ context.Context, identity Identity) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.liveRefs[identityKey(identity)]) > 0, nil
}

func (s *MemoryStore) MarkDone(_ context.Context, claim Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[claim.ID]
	if job.State != StateRunning || job.AttemptCount != claim.AttemptCount {
		return nil
	}
	now := s.now().UTC()
	job.State, job.CompletedAt, job.UpdatedAt = StateDone, &now, now
	s.jobs[claim.ID] = job
	return nil
}
func (s *MemoryStore) MarkBlocked(_ context.Context, claim Job, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[claim.ID]
	if job.State != StateRunning || job.AttemptCount != claim.AttemptCount {
		return nil
	}
	job.State, job.LastErrorCode, job.UpdatedAt = StateBlocked, sanitizeCode(code), s.now().UTC()
	s.jobs[claim.ID] = job
	return nil
}
func (s *MemoryStore) MarkRetry(_ context.Context, claim Job, next time.Time, code, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[claim.ID]
	if job.State != StateRunning || job.AttemptCount != claim.AttemptCount {
		return nil
	}
	job.State, job.NextAttemptAt, job.LastErrorCode, job.LastErrorMessage, job.UpdatedAt = StateRetry, &next, sanitizeCode(code), sanitizeMessage(message), s.now().UTC()
	s.jobs[claim.ID] = job
	return nil
}

func (s *MemoryStore) AddLiveReference(identity Identity, ref string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := identityKey(identity)
	if s.liveRefs[key] == nil {
		s.liveRefs[key] = map[string]struct{}{}
	}
	s.liveRefs[key][ref] = struct{}{}
}
func (s *MemoryStore) RemoveLiveReference(identity Identity, ref string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.liveRefs[identityKey(identity)], ref)
}
func (s *MemoryStore) AddDeletedCandidate(identity Identity) {
	s.mu.Lock()
	defer s.mu.Unlock()
	identity = domaincleanup.CanonicalIdentity(identity)
	s.candidates[identityKey(identity)] = identity
}
func (s *MemoryStore) Jobs() []Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := make([]Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		result = append(result, job)
	}
	return result
}
func (s *MemoryStore) Reconcile(ctx context.Context, limit int) (int, error) {
	s.mu.Lock()
	candidates := make([]Identity, 0, len(s.candidates))
	for _, item := range s.candidates {
		if len(candidates) >= limit {
			break
		}
		candidates = append(candidates, item)
	}
	s.mu.Unlock()
	count := 0
	for _, item := range candidates {
		if _, err := s.Enqueue(ctx, item); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}
