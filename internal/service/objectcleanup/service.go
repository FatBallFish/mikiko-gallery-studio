package objectcleanup

import (
	"context"
	"errors"
	"math/rand"
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
	DeleteIfUnreferenced(context.Context, Job, func() error) (bool, error)
	MarkDone(context.Context, string) error
	MarkBlocked(context.Context, string, string) error
	MarkRetry(context.Context, string, time.Time, string, string) error
	Reconcile(context.Context, int) (int, error)
}

type ProcessorOptions struct {
	Now    func() time.Time
	Jitter func(time.Duration) time.Duration
}

type Processor struct {
	store  Store
	router storage.Router
	now    func() time.Time
	jitter func(time.Duration) time.Duration
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
	return &Processor{store: store, router: router, now: options.Now, jitter: options.Jitter}
}

func (p *Processor) ProcessOnce(ctx context.Context) (bool, error) {
	job, ok, err := p.store.Claim(ctx, p.now())
	if err != nil || !ok {
		return false, err
	}
	ref, err := p.router.BackendFor(ctx, job.Identity.StorageConfigID, job.Identity.StorageDriver)
	if err != nil {
		return true, p.retry(ctx, job, "storage_config_unavailable", "storage backend unavailable")
	}
	blocked, deleteErr := p.store.DeleteIfUnreferenced(ctx, job, func() error {
		return ref.Backend.Delete(ctx, job.Identity.ObjectKey)
	})
	if blocked {
		return true, p.store.MarkBlocked(ctx, job.ID, "live_reference")
	}
	if deleteErr == nil || errors.Is(deleteErr, storage.ErrNotFound) {
		return true, p.store.MarkDone(ctx, job.ID)
	}
	return true, p.retry(ctx, job, "storage_delete_failed", "storage delete failed")
}

func (p *Processor) Reconcile(ctx context.Context, limit int) (int, error) {
	return p.store.Reconcile(ctx, limit)
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
	return p.store.MarkRetry(ctx, job.ID, p.now().Add(delay), sanitizeCode(code), sanitizeMessage(message))
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
	mu         sync.Mutex
	now        func() time.Time
	jobs       map[string]Job
	jobByKey   map[string]string
	liveRefs   map[string]map[string]struct{}
	candidates map[string]Identity
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		now:        time.Now,
		jobs:       map[string]Job{},
		jobByKey:   map[string]string{},
		liveRefs:   map[string]map[string]struct{}{},
		candidates: map[string]Identity{},
	}
}

func (s *MemoryStore) SetNow(now func() time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.now = now
}

func identityKey(value Identity) string {
	return strings.Join([]string{
		strings.TrimSpace(value.StorageConfigID),
		strings.ToLower(strings.TrimSpace(value.StorageDriver)),
		strings.TrimSpace(value.Bucket),
		strings.TrimSpace(value.ObjectKey),
	}, "\x00")
}

func (s *MemoryStore) Enqueue(_ context.Context, identity Identity) (Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key, now := identityKey(identity), s.now().UTC()
	if id := s.jobByKey[key]; id != "" {
		job := s.jobs[id]
		if job.State == StateDone {
			return job, nil
		}
		job.State, job.NextAttemptAt, job.CompletedAt = StatePending, nil, nil
		job.UpdatedAt = now
		s.jobs[id] = job
		return job, nil
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

func (s *MemoryStore) DeleteIfUnreferenced(_ context.Context, job Job, deleteFn func() error) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.liveRefs[identityKey(job.Identity)]) > 0 {
		return true, nil
	}
	return false, deleteFn()
}

func (s *MemoryStore) MarkDone(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	if job.State != StateRunning {
		return nil
	}
	now := s.now().UTC()
	job.State, job.CompletedAt, job.UpdatedAt = StateDone, &now, now
	s.jobs[id] = job
	return nil
}
func (s *MemoryStore) MarkBlocked(_ context.Context, id, code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	if job.State != StateRunning {
		return nil
	}
	job.State, job.LastErrorCode, job.UpdatedAt = StateBlocked, sanitizeCode(code), s.now().UTC()
	s.jobs[id] = job
	return nil
}
func (s *MemoryStore) MarkRetry(_ context.Context, id string, next time.Time, code, message string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	job := s.jobs[id]
	if job.State != StateRunning {
		return nil
	}
	job.State, job.NextAttemptAt, job.LastErrorCode, job.LastErrorMessage, job.UpdatedAt = StateRetry, &next, sanitizeCode(code), sanitizeMessage(message), s.now().UTC()
	s.jobs[id] = job
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
