package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/fatballfish/pic-gallery/internal/config"
	domainstorageconfig "github.com/fatballfish/pic-gallery/internal/domain/storageconfig"
)

var (
	ErrNoDefaultStorage  = errors.New("default storage config is unavailable")
	ErrStorageUnreadable = errors.New("storage config is not readable")
)

const (
	defaultRegistryProbeTimeout = 10 * time.Second
	defaultRegistryProbeWorkers = 8
)

type BackendRef struct {
	ConfigID  string
	Driver    string
	Bucket    string
	Namespace string
	Version   int64
	Backend   Backend
}

type Router interface {
	DefaultWriter(ctx context.Context) (BackendRef, error)
	BackendFor(ctx context.Context, configID string, legacyDriver string) (BackendRef, error)
	ReadableBackends(ctx context.Context) ([]BackendRef, error)
}

type ConfigSource interface {
	ResolveDefaultWritable(ctx context.Context) (domainstorageconfig.ResolvedConfig, error)
	ResolveByID(ctx context.Context, id string) (domainstorageconfig.ResolvedConfig, error)
	ResolveLegacyByDriver(ctx context.Context, driver string) (domainstorageconfig.ResolvedConfig, error)
	ListReadableConfigIDs(ctx context.Context) ([]string, error)
}

type StaticRouter struct{ ref BackendRef }

func NewStaticRouter(backend Backend) *StaticRouter {
	if backend == nil {
		backend = NewLocalBackend("")
	}
	driver := strings.ToLower(strings.TrimSpace(backend.Driver()))
	return &StaticRouter{ref: BackendRef{Driver: driver, Namespace: "static:" + driver, Backend: backend}}
}

func (r *StaticRouter) DefaultWriter(context.Context) (BackendRef, error) { return r.ref, nil }

func (r *StaticRouter) BackendFor(_ context.Context, configID string, legacyDriver string) (BackendRef, error) {
	ref := r.ref
	if strings.TrimSpace(configID) != "" {
		ref.ConfigID = strings.TrimSpace(configID)
	}
	if strings.TrimSpace(legacyDriver) != "" {
		ref.Driver = strings.TrimSpace(legacyDriver)
	}
	return ref, nil
}

func (r *StaticRouter) ReadableBackends(context.Context) ([]BackendRef, error) {
	return []BackendRef{r.ref}, nil
}

type Registry struct {
	source              ConfigSource
	ttl                 time.Duration
	now                 func() time.Time
	newBackend          func(config.StorageConfig) (Backend, error)
	probeTimeout        time.Duration
	probeCleanupTimeout time.Duration
	probeSlots          chan struct{}

	mu       sync.Mutex
	resolved map[string]cachedResolved
	backends map[string]BackendRef
}

type cachedResolved struct {
	config    domainstorageconfig.ResolvedConfig
	expiresAt time.Time
}

func NewRegistry(source ConfigSource, ttl time.Duration) *Registry {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	return &Registry{
		source: source, ttl: ttl, now: time.Now, newBackend: NewBackend,
		probeTimeout: defaultRegistryProbeTimeout, probeCleanupTimeout: time.Second,
		probeSlots: make(chan struct{}, defaultRegistryProbeWorkers),
		resolved:   map[string]cachedResolved{}, backends: map[string]BackendRef{},
	}
}

func (r *Registry) DefaultWriter(ctx context.Context) (BackendRef, error) {
	if r == nil || r.source == nil {
		return BackendRef{}, ErrNoDefaultStorage
	}
	resolved, err := r.resolveCached(ctx, "default", r.source.ResolveDefaultWritable)
	if err != nil {
		return BackendRef{}, err
	}
	if resolved.Status != domainstorageconfig.StatusEnabled || !resolved.ReadEnabled || !resolved.WriteEnabled {
		return BackendRef{}, ErrNoDefaultStorage
	}
	return r.backendForResolved(resolved)
}

func (r *Registry) BackendFor(ctx context.Context, configID string, legacyDriver string) (BackendRef, error) {
	if r == nil || r.source == nil {
		return BackendRef{}, ErrStorageUnreadable
	}
	configID, legacyDriver = strings.TrimSpace(configID), strings.TrimSpace(legacyDriver)
	var (
		resolved domainstorageconfig.ResolvedConfig
		err      error
	)
	if configID != "" {
		resolved, err = r.resolveCached(ctx, "id:"+configID, func(ctx context.Context) (domainstorageconfig.ResolvedConfig, error) {
			return r.source.ResolveByID(ctx, configID)
		})
	} else {
		resolved, err = r.resolveCached(ctx, "legacy:"+legacyDriver, func(ctx context.Context) (domainstorageconfig.ResolvedConfig, error) {
			return r.source.ResolveLegacyByDriver(ctx, legacyDriver)
		})
	}
	if err != nil {
		return BackendRef{}, err
	}
	if resolved.Status == domainstorageconfig.StatusDeleted || !resolved.ReadEnabled {
		return BackendRef{}, ErrStorageUnreadable
	}
	return r.backendForResolved(resolved)
}

func (r *Registry) ReadableBackends(ctx context.Context) ([]BackendRef, error) {
	if r == nil || r.source == nil {
		return nil, ErrStorageUnreadable
	}
	ids, err := r.source.ListReadableConfigIDs(ctx)
	if err != nil {
		return nil, err
	}
	refs := make([]BackendRef, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ref, err := r.BackendFor(ctx, id, "")
		if err != nil {
			slog.WarnContext(ctx, "storage backend skipped during readable enumeration", "config_id", id, "error_code", "storage_config_unavailable")
			continue
		}
		refs = append(refs, ref)
	}
	return refs, nil
}

func (r *Registry) resolveCached(ctx context.Context, key string, resolve func(context.Context) (domainstorageconfig.ResolvedConfig, error)) (domainstorageconfig.ResolvedConfig, error) {
	now := r.now()
	r.mu.Lock()
	if cached, ok := r.resolved[key]; ok && cached.expiresAt.After(now) {
		r.mu.Unlock()
		return cached.config, nil
	}
	r.mu.Unlock()

	resolved, err := resolve(ctx)
	if err != nil {
		return domainstorageconfig.ResolvedConfig{}, err
	}
	if strings.TrimSpace(resolved.ID) == "" {
		return domainstorageconfig.ResolvedConfig{}, ErrStorageUnreadable
	}
	r.mu.Lock()
	r.resolved[key] = cachedResolved{config: resolved, expiresAt: now.Add(r.ttl)}
	r.mu.Unlock()
	return resolved, nil
}

func (r *Registry) Invalidate(event StorageInvalidation) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if event.DefaultChanged {
		delete(r.resolved, "default")
	}
	if event.ConfigID == "" {
		return
	}
	delete(r.resolved, "id:"+event.ConfigID)
	for key, cached := range r.resolved {
		if cached.config.ID == event.ConfigID {
			delete(r.resolved, key)
		}
	}
	for key, ref := range r.backends {
		if ref.ConfigID == event.ConfigID {
			delete(r.backends, key)
		}
	}
}

func (r *Registry) Probe(ctx context.Context, resolved domainstorageconfig.ResolvedConfig) domainstorageconfig.ProbeResult {
	start := r.now()
	if ctx == nil {
		ctx = context.Background()
	}
	probeContext, cancelProbe := context.WithTimeout(ctx, r.probeTimeout)
	defer cancelProbe()
	if err := probeContext.Err(); err != nil {
		return probeFailure(r, start, err)
	}
	select {
	case r.probeSlots <- struct{}{}:
	case <-probeContext.Done():
		return probeFailure(r, start, probeContext.Err())
	}
	if err := probeContext.Err(); err != nil {
		<-r.probeSlots
		return probeFailure(r, start, err)
	}

	resultChannel := make(chan domainstorageconfig.ProbeResult, 1)
	go func() {
		defer func() { <-r.probeSlots }()
		result := domainstorageconfig.ProbeResult{}
		func() {
			defer func() {
				if recover() != nil {
					result = probeFailure(r, start, errors.New("storage probe runner panicked"))
				}
			}()
			result = r.runProbeIO(probeContext, resolved, start)
		}()
		resultChannel <- result
	}()

	select {
	case result := <-resultChannel:
		if err := probeContext.Err(); err != nil {
			return probeFailure(r, start, err)
		}
		return result
	case <-probeContext.Done():
		return probeFailure(r, start, probeContext.Err())
	}
}

func (r *Registry) runProbeIO(ctx context.Context, resolved domainstorageconfig.ResolvedConfig, start time.Time) (result domainstorageconfig.ProbeResult) {
	if err := ctx.Err(); err != nil {
		return probeFailure(r, start, err)
	}
	backend, err := r.newBackend(StorageConfigFromResolved(resolved))
	if err != nil {
		return domainstorageconfig.ProbeResult{Status: domainstorageconfig.ProbeStatusFailed, CheckedAt: r.now().UTC(), Message: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return probeFailure(r, start, err)
	}
	key := ".pic-gallery-probe-" + start.UTC().Format("20060102150405.000000000") + ".txt"
	content := []byte("pic-gallery-storage-probe")
	cleanupRequired := true
	defer func() {
		if !cleanupRequired {
			return
		}
		cleanupContext, cancelCleanup := context.WithTimeout(context.Background(), r.probeCleanupTimeout)
		defer cancelCleanup()
		if cleanupErr := backend.Delete(cleanupContext, key); cleanupErr != nil && !errors.Is(cleanupErr, ErrNotFound) {
			result = probeFailure(r, start, errors.New("probe object cleanup failed"))
		}
	}()
	if err := backend.Put(ctx, key, "text/plain", content); err != nil {
		return probeFailure(r, start, err)
	}
	if err := ctx.Err(); err != nil {
		return probeFailure(r, start, err)
	}
	boundedGetter, ok := backend.(BoundedGetter)
	if !ok {
		return probeFailure(r, start, errors.New("storage backend does not support bounded reads"))
	}
	got, err := boundedGetter.GetBounded(ctx, key, int64(len(content)))
	if err != nil {
		return probeFailure(r, start, err)
	}
	if err := ctx.Err(); err != nil {
		return probeFailure(r, start, err)
	}
	if !bytes.Equal(got, content) {
		return probeFailure(r, start, errors.New("probe object content mismatch"))
	}
	if err := backend.Delete(ctx, key); err != nil && !errors.Is(err, ErrNotFound) {
		return probeFailure(r, start, err)
	}
	cleanupRequired = false
	if err := ctx.Err(); err != nil {
		return probeFailure(r, start, err)
	}
	return domainstorageconfig.ProbeResult{Status: domainstorageconfig.ProbeStatusSuccess, CheckedAt: r.now().UTC(), LatencyMS: r.now().Sub(start).Milliseconds(), Message: "put/get/delete probe object succeeded"}
}

func probeFailure(r *Registry, start time.Time, err error) domainstorageconfig.ProbeResult {
	return domainstorageconfig.ProbeResult{Status: domainstorageconfig.ProbeStatusFailed, CheckedAt: r.now().UTC(), LatencyMS: r.now().Sub(start).Milliseconds(), Message: err.Error()}
}

func (r *Registry) backendForResolved(resolved domainstorageconfig.ResolvedConfig) (BackendRef, error) {
	key := strings.Join([]string{resolved.ID, fmt.Sprintf("%d", resolved.Version), resolved.SecretFingerprint}, ":")
	r.mu.Lock()
	if ref, ok := r.backends[key]; ok {
		r.mu.Unlock()
		return ref, nil
	}
	r.mu.Unlock()
	backend, err := NewBackend(StorageConfigFromResolved(resolved))
	if err != nil {
		return BackendRef{}, err
	}
	ref := BackendRef{ConfigID: resolved.ID, Driver: backend.Driver(), Bucket: resolved.Bucket, Namespace: storageNamespaceFingerprint(resolved.ConfigRecord), Version: resolved.Version, Backend: backend}
	r.mu.Lock()
	for cachedKey, cached := range r.backends {
		if cached.ConfigID == resolved.ID {
			delete(r.backends, cachedKey)
		}
	}
	r.backends[key] = ref
	r.mu.Unlock()
	return ref, nil
}

func storageNamespaceFingerprint(record domainstorageconfig.ConfigRecord) string {
	canonical := strings.Join([]string{
		strings.ToLower(strings.TrimSpace(record.Driver)),
		strings.ToLower(strings.TrimSpace(record.Provider)),
		strings.TrimSpace(record.Endpoint),
		strings.TrimSpace(record.Region),
		strings.TrimSpace(record.Bucket),
		strings.Trim(strings.TrimSpace(record.Prefix), "/"),
		fmt.Sprintf("%t", record.ForcePathStyle),
		strings.TrimSpace(record.LocalRoot),
	}, "\x00")
	digest := sha256.Sum256([]byte(canonical))
	return fmt.Sprintf("sha256:%x", digest[:])
}

func StorageConfigFromResolved(resolved domainstorageconfig.ResolvedConfig) config.StorageConfig {
	cfg := config.StorageConfig{
		Driver: resolved.Driver, LocalRoot: resolved.LocalRoot, PublicBaseURL: resolved.PublicBaseURL,
		S3: config.StorageS3Config{
			Endpoint: resolved.Endpoint, Region: resolved.Region, Bucket: resolved.Bucket, Prefix: strings.Trim(resolved.Prefix, "/"),
			ForcePathStyle: resolved.ForcePathStyle, AccessKeyID: strings.TrimSpace(fmt.Sprint(resolved.Secrets["access_key_id"])),
			SecretAccessKey: strings.TrimSpace(fmt.Sprint(resolved.Secrets["secret_access_key"])),
		},
	}
	if strings.TrimSpace(cfg.Driver) == "" {
		cfg.Driver = domainstorageconfig.DriverLocal
	}
	return cfg
}
